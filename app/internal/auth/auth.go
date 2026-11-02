// Package auth protege FitOrDie con un único usuario y sesión firmada.
//
// Modelo de seguridad:
//   - Un solo usuario (APP_USER) con la clave guardada como hash bcrypt
//     (APP_PASSWORD_HASH, coste >= 10). Nunca se guarda la clave en claro.
//   - Sesión en cookie firmada con HMAC-SHA256 (SESSION_SECRET): no hay tabla
//     de sesiones, así que el proceso es stateless y el volumen SQLite no
//     cambia. La cookie se re-emite en cada request (sesión deslizante).
//   - Cookie HttpOnly + Secure + SameSite=Lax + Path=/, 30 días.
//   - CSRF por doble envío: cookie aleatoria + token = HMAC(secreto, cookie)
//     dentro del formulario. Sin el secreto el token no se puede fabricar.
//   - Rate limit de 5 intentos fallidos por minuto y por IP; la IP sale de
//     CF-Connecting-IP solo si el request llega desde una red privada (el
//     cloudflared corre al lado del proceso y Cloudflare pisa ese header).
//   - Mensajes genéricos: no se distingue usuario inexistente de clave mala.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ldgnu/fitordie/templates"
)

// Nombres de cookie y parámetros de la sesión.
const (
	SessionCookie = "fdo_session"
	csrfCookie    = "fdo_csrf"
	loginPath     = "/login"

	// SessionTTL es la vida de la sesión (30 días) y de la cookie.
	SessionTTL = 30 * 24 * time.Hour

	// maxFormBytes acota el cuerpo del POST /login (email + clave + csrf).
	maxFormBytes = 4 << 10

	// rateLimit / rateWindow: intentos fallidos permitidos por IP.
	rateLimit  = 5
	rateWindow = time.Minute

	// minCost es el coste bcrypt mínimo aceptado; el hash de producción usa 12.
	minCost = 10

	// maxNextLen evita que un `next` gigante se meta en la URL de redirect.
	maxNextLen = 512
)

// publicPaths son las únicas rutas que no exigen sesión. Todo lo demás
// (páginas y API) pasa por el middleware.
var publicPaths = map[string]bool{
	loginPath:               true,
	"/api/health":           true, // liveness: sin datos personales
	"/manifest.webmanifest": true, // PWA
	"/favicon.ico":          true, // pedido directo del navegador
}

// Config es lo que necesita el guard para arrancar.
type Config struct {
	User          string // email del único usuario
	PasswordHash  string // hash bcrypt ($2a$12$...)
	SessionSecret string // secreto HMAC (>= 32 bytes de entropía)

	// Now inyecta el reloj (tests). nil = time.Now.
	Now func() time.Time
}

// Guard valida credenciales, emite/renueva la cookie y protege el mux.
type Guard struct {
	user    string // tal como se configura
	userKey []byte // minúsculas, para comparar en tiempo constante
	hash    []byte
	secret  []byte
	login   *template.Template
	limits  *attempts
	now     func() time.Time
	ttl     time.Duration
}

// New valida la configuración y parsea el template del login.
// Falla cerrado: sin usuario, sin hash válido o con un secreto corto no arranca.
func New(cfg Config) (*Guard, error) {
	user := strings.TrimSpace(cfg.User)
	if user == "" {
		return nil, errors.New("APP_USER vacío")
	}
	hash := []byte(strings.TrimSpace(cfg.PasswordHash))
	if len(hash) == 0 {
		return nil, errors.New("APP_PASSWORD_HASH vacío")
	}
	cost, err := bcrypt.Cost(hash)
	if err != nil {
		return nil, fmt.Errorf("APP_PASSWORD_HASH no es un hash bcrypt válido: %w", err)
	}
	if cost < minCost {
		return nil, fmt.Errorf("APP_PASSWORD_HASH usa coste %d: mínimo %d", cost, minCost)
	}
	secret := []byte(strings.TrimSpace(cfg.SessionSecret))
	if len(secret) < 32 {
		return nil, fmt.Errorf("SESSION_SECRET demasiado corto (%d bytes): mínimo 32", len(secret))
	}
	t, err := template.ParseFS(templates.FS, "login.html")
	if err != nil {
		return nil, fmt.Errorf("parsear templates/login.html: %w", err)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Guard{
		user:    user,
		userKey: []byte(strings.ToLower(user)),
		hash:    hash,
		secret:  secret,
		login:   t,
		limits:  newAttempts(rateLimit, rateWindow),
		now:     now,
		ttl:     SessionTTL,
	}, nil
}

// Routes registra /login (GET y POST) y /logout (GET y POST).
func (g *Guard) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+loginPath, g.form)
	mux.HandleFunc("POST "+loginPath, g.submit)
	mux.HandleFunc("GET /logout", g.logout)
	mux.HandleFunc("POST /logout", g.logout)
}

// Protect envuelve el mux: todo lo que no esté en publicPaths exige sesión.
// Si no hay cookie válida redirige a /login?next=<ruta interna>.
func (g *Guard) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cabeceras defensivas para todas las respuestas.
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")

		if isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		sess, ok := g.readSession(r)
		if !ok {
			g.redirectToLogin(w, r)
			return
		}
		// Sesión deslizante: cada request renueva los 30 días. /logout es la
		// excepción: ahí el handler borra la cookie y no hay que reemitirla.
		if r.URL.Path != "/logout" {
			g.writeSession(w, sess.sid)
		}
		next.ServeHTTP(w, r)
	})
}

// isPublic indica si la ruta se sirve sin sesión.
func isPublic(path string) bool {
	if publicPaths[path] {
		return true
	}
	return strings.HasPrefix(path, "/static/")
}

// ---------------------------------------------------------------- handlers

func (g *Guard) form(w http.ResponseWriter, r *http.Request) {
	// Ya logueado: no tiene sentido mostrar el formulario.
	if _, ok := g.readSession(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	g.render(w, r, http.StatusOK, loginView{
		Next: safeNext(r.URL.Query().Get("next")),
		CSRF: g.ensureCSRF(w, r),
	})
}

func (g *Guard) submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		g.render(w, r, http.StatusBadRequest, loginView{
			Next:  "/",
			CSRF:  g.ensureCSRF(w, r),
			Error: "No pudimos leer el formulario. Probá de nuevo.",
		})
		return
	}

	next := safeNext(r.PostFormValue("next"))

	// 1) CSRF antes que nada: un POST sin token no consume rate limit.
	if !g.validCSRF(r, r.PostFormValue("csrf")) {
		g.render(w, r, http.StatusForbidden, loginView{
			Next:  next,
			CSRF:  g.ensureCSRF(w, r),
			Error: "La página venció. Recargá y volvé a intentar.",
		})
		return
	}

	// 2) Rate limit por IP.
	ip := clientIP(r)
	now := g.now()
	if blocked, retry := g.limits.blocked(ip, now); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds()+0.5)))
		slog.Warn("login bloqueado por rate limit", "ip", ip, "retry_s", int(retry.Seconds()))
		g.render(w, r, http.StatusTooManyRequests, loginView{
			Next:  next,
			CSRF:  g.ensureCSRF(w, r),
			Error: "Demasiados intentos. Esperá un minuto y probá otra vez.",
		})
		return
	}

	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	// 3) Credenciales. bcrypt corre siempre (aunque el email no coincida)
	// para no filtrar por tiempo si el usuario existe.
	if !g.check(email, password) {
		g.limits.fail(ip, now)
		slog.Warn("login fallido", "ip", ip)
		g.render(w, r, http.StatusUnauthorized, loginView{
			Next:  next,
			CSRF:  g.ensureCSRF(w, r),
			Email: strings.TrimSpace(email),
			Error: "Usuario o clave incorrectos.",
		})
		return
	}

	// 4) Éxito: sesión nueva y redirect al destino pedido.
	g.limits.forgive(ip)
	sid, err := randomToken(32)
	if err != nil {
		slog.Error("generar id de sesión", "err", err)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	g.writeSession(w, sid)
	slog.Info("login ok", "ip", ip)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (g *Guard) logout(w http.ResponseWriter, r *http.Request) {
	g.clear(w, SessionCookie)
	g.clear(w, csrfCookie)
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}

// ---------------------------------------------------------------- login view

type loginView struct {
	Title string
	Email string
	Next  string
	CSRF  string
	Error string
}

func (g *Guard) render(w http.ResponseWriter, r *http.Request, status int, v loginView) {
	v.Title = "Entrar"
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
	// La página de login no necesita nada externo: CSP estricta.
	h.Set("Content-Security-Policy",
		"default-src 'none'; style-src 'self'; img-src 'self'; font-src 'self'; "+
			"form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	if err := g.login.ExecuteTemplate(w, "login.html", v); err != nil {
		slog.Error("render login", "err", err)
	}
}

// ---------------------------------------------------------------- sesión

// session es el contenido firmado de la cookie.
type session struct {
	sid string
	exp time.Time
}

// writeSession emite (o renueva) la cookie firmada con vencimiento a 30 días.
func (g *Guard) writeSession(w http.ResponseWriter, sid string) {
	exp := g.now().Add(g.ttl)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    g.sign(sid, exp),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  exp,
		MaxAge:   int(g.ttl.Seconds()),
	})
}

// readSession valida firma, usuario y vencimiento de la cookie.
func (g *Guard) readSession(r *http.Request) (session, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return session{}, false
	}
	b64, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return session{}, false
	}
	if !hmac.Equal([]byte(sig), []byte(g.mac("fdo.session."+b64))) {
		return session{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return session{}, false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[0] != "v1" {
		return session{}, false
	}
	if subtle.ConstantTimeCompare([]byte(parts[1]), g.userKey) != 1 {
		return session{}, false
	}
	expUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return session{}, false
	}
	exp := time.Unix(expUnix, 0)
	if !g.now().Before(exp) {
		return session{}, false
	}
	if parts[3] == "" {
		return session{}, false
	}
	return session{sid: parts[3], exp: exp}, true
}

// sign arma el valor de la cookie: payload firmado con usuario y vencimiento.
func (g *Guard) sign(sid string, exp time.Time) string {
	payload := "v1|" + string(g.userKey) + "|" + strconv.FormatInt(exp.Unix(), 10) + "|" + sid
	b64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return b64 + "." + g.mac("fdo.session."+b64)
}

// mac devuelve el HMAC-SHA256 en base64url sin padding.
func (g *Guard) mac(msg string) string {
	m := hmac.New(sha256.New, g.secret)
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// redirectToLogin manda al formulario recordando el destino (solo interno).
func (g *Guard) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := loginPath
	if r.Method == http.MethodGet {
		next := r.URL.RequestURI()
		if safe := safeNext(next); safe != "/" {
			target += "?next=" + url.QueryEscape(safe)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

// clear borra una cookie (logout).
func (g *Guard) clear(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// ---------------------------------------------------------------- CSRF

// ensureCSRF devuelve el token del formulario, creando la cookie si falta.
func (g *Guard) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	seed := ""
	if c, err := r.Cookie(csrfCookie); err == nil {
		seed = c.Value
	}
	if len(seed) < 32 {
		var err error
		if seed, err = randomToken(32); err != nil {
			return ""
		}
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookie,
			Value:    seed,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			Expires:  g.now().Add(g.ttl),
			MaxAge:   int(g.ttl.Seconds()),
		})
	}
	return g.mac("fdo.csrf." + seed)
}

// validCSRF compara el token del form con el derivado de la cookie.
func (g *Guard) validCSRF(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	c, err := r.Cookie(csrfCookie)
	if err != nil || len(c.Value) < 32 {
		return false
	}
	return hmac.Equal([]byte(token), []byte(g.mac("fdo.csrf."+c.Value)))
}

// ---------------------------------------------------------------- CSRF público

// CSRFToken devuelve el token para un formulario, emitiendo la cookie si falta.
// Es la puerta que usa el panel web para los POST server-rendered.
func (g *Guard) CSRFToken(w http.ResponseWriter, r *http.Request) string {
	return g.ensureCSRF(w, r)
}

// ValidCSRFToken valida el token que vino en el campo `csrf` de un POST.
func (g *Guard) ValidCSRFToken(r *http.Request) bool {
	token := r.PostFormValue("csrf")
	if token == "" {
		// Los clientes que mandan JSON lo pueden poner en el header.
		token = r.Header.Get("X-CSRF-Token")
	}
	return g.validCSRF(r, token)
}

// ---------------------------------------------------------------- credenciales

// check compara email en tiempo constante y siempre ejecuta bcrypt.
func (g *Guard) check(email, password string) bool {
	emailOK := subtle.ConstantTimeCompare([]byte(strings.ToLower(strings.TrimSpace(email))), g.userKey) == 1
	err := bcrypt.CompareHashAndPassword(g.hash, []byte(password))
	return emailOK && err == nil
}

// ---------------------------------------------------------------- helpers

// randomToken devuelve n bytes aleatorios en base64url (sin padding).
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// safeNext solo acepta rutas internas: "/x", nunca "//host" ni "http://...".
// Devuelve "/" cuando no confía en el valor.
func safeNext(raw string) string {
	if raw == "" || len(raw) > maxNextLen {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.ContainsAny(raw, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return "/"
	}
	if u.Path == loginPath || u.Path == "/logout" {
		return "/"
	}
	return u.RequestURI()
}

// clientIP devuelve la IP del cliente. CF-Connecting-IP se usa solo si el
// request llega desde una red privada (cloudflared en el host o en docker):
// así un cliente directo no puede falsear la IP y saltarse el rate limit.
func clientIP(r *http.Request) string {
	ip := remoteIP(r.RemoteAddr)
	if isPrivateIP(ip) {
		if v := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); v != "" {
			if net.ParseIP(v) != nil {
				return v
			}
		}
	}
	if ip == nil {
		return "desconocida"
	}
	return ip.String()
}

func remoteIP(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func isPrivateIP(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// ---------------------------------------------------------------- rate limit

// attempts es una ventana deslizante de intentos fallidos por clave (IP).
type attempts struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newAttempts(limit int, window time.Duration) *attempts {
	return &attempts{hits: make(map[string][]time.Time), limit: limit, window: window}
}

// blocked informa si la clave agotó sus intentos y cuánto falta para reintentar.
func (a *attempts) blocked(key string, now time.Time) (bool, time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.pruneLocked(key, now)
	h := a.hits[key]
	if len(h) < a.limit {
		return false, 0
	}
	return true, a.window - now.Sub(h[0])
}

// fail registra un intento fallido.
func (a *attempts) fail(key string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.hits[key] = append(a.hits[key], now)
	a.pruneLocked(key, now)
	if len(a.hits) > 4096 {
		a.sweepLocked(now)
	}
}

// forgive limpia el historial tras un login correcto.
func (a *attempts) forgive(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.hits, key)
}

func (a *attempts) pruneLocked(key string, now time.Time) {
	h, ok := a.hits[key]
	if !ok {
		return
	}
	cut := now.Add(-a.window)
	i := 0
	for i < len(h) && !h[i].After(cut) {
		i++
	}
	if i == len(h) {
		delete(a.hits, key)
		return
	}
	if i > 0 {
		a.hits[key] = append(h[:0], h[i:]...)
	}
}

// sweepLocked libera memoria cuando hay muchas IPs dando vueltas.
func (a *attempts) sweepLocked(now time.Time) {
	cut := now.Add(-a.window)
	for k, h := range a.hits {
		if len(h) == 0 || !h[len(h)-1].After(cut) {
			delete(a.hits, k)
		}
	}
}
