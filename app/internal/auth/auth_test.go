package auth_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ldgnu/fitordie/internal/auth"
)

const (
	testUser = "correo@example.com"
	testPass = "Clave-de-prueba-123!"
	secret   = "secreto-de-pruebas-con-mas-de-32-bytes-0123456789"
)

// hashCache genera el hash una sola vez: bcrypt es lento a propósito.
var (
	hashOnce sync.Once
	testHash []byte
)

func hashFor(t *testing.T) []byte {
	t.Helper()
	hashOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte(testPass), bcrypt.DefaultCost)
		if err != nil {
			t.Fatalf("generar hash: %v", err)
		}
		testHash = h
	})
	return testHash
}

// guard construye el guard sobre un mux de prueba con rutas públicas y privadas.
func guard(t *testing.T, mutate func(*auth.Config)) http.Handler {
	t.Helper()
	cfg := auth.Config{
		User:          testUser,
		PasswordHash:  string(hashFor(t)),
		SessionSecret: secret,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	g, err := auth.New(cfg)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hoy")) })
	mux.HandleFunc("GET /rutina", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("rutina")) })
	mux.HandleFunc("GET /progreso", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("progreso")) })
	mux.HandleFunc("GET /api/profile", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"name":"javi"}`)) })
	mux.HandleFunc("POST /api/log", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) })
	mux.HandleFunc("GET /static/css/app.css", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("css")) })
	mux.HandleFunc("GET /manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) })
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	g.Routes(mux)
	return g.Protect(mux)
}

func newServer(t *testing.T) http.Handler { return guard(t, nil) }

// fakeClock permite probar la renovación de la sesión sin dormir el test.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func do(h http.Handler, method, target string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	body := ""
	if form != nil {
		body = form.Encode()
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// cookies devuelve todas las Set-Cookie con ese nombre (gana la última).
func cookies(rec *httptest.ResponseRecorder, name string) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func cookieByName(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	all := cookies(rec, name)
	if len(all) == 0 {
		return nil
	}
	return all[len(all)-1]
}

// formCSRF lee el token oculto del formulario servido.
func formCSRF(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	const marker = `name="csrf" value="`
	body := rec.Body.String()
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("el formulario no trae el campo csrf")
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("csrf mal formado")
	}
	return rest[:j]
}

// sessionExp saca el vencimiento (unix) firmado dentro de la cookie.
func sessionExp(t *testing.T, c *http.Cookie) int64 {
	t.Helper()
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		t.Fatalf("cookie de sesión mal formada: %q", c.Value)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("payload ilegible: %v", err)
	}
	f := strings.Split(string(raw), "|")
	if len(f) != 4 {
		t.Fatalf("payload inesperado: %q", raw)
	}
	exp, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		t.Fatalf("vencimiento inválido: %v", err)
	}
	return exp
}

// loginFlow pide el formulario (cookie + token csrf) y hace el POST.
func loginFlow(t *testing.T, h http.Handler, email, password, next string) (*httptest.ResponseRecorder, *http.Cookie) {
	t.Helper()

	get := do(h, http.MethodGet, "/login", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET /login = %d, quería 200", get.Code)
	}
	csrf := cookieByName(get, "fdo_csrf")
	if csrf == nil {
		t.Fatal("GET /login no emitió cookie csrf")
	}
	form := url.Values{
		"email":    {email},
		"password": {password},
		"csrf":     {formCSRF(t, get)},
		"next":     {next},
	}
	rec := do(h, http.MethodPost, "/login", form, csrf)
	return rec, cookieByName(rec, auth.SessionCookie)
}

func TestPublicPathsNoSession(t *testing.T) {
	h := newServer(t)
	for _, p := range []string{"/api/health", "/static/css/app.css", "/manifest.webmanifest", "/favicon.ico"} {
		rec := do(h, http.MethodGet, p, nil)
		if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
			t.Errorf("GET %s = %d, quería 200 (público)", p, rec.Code)
		}
	}
}

func TestProtectedRedirectsToLogin(t *testing.T) {
	h := newServer(t)
	cases := []struct{ method, path, wantLoc string }{
		{http.MethodGet, "/", "/login"},
		{http.MethodGet, "/rutina", "/login?next=%2Frutina"},
		{http.MethodGet, "/progreso", "/login?next=%2Fprogreso"},
		{http.MethodGet, "/api/profile", "/login?next=%2Fapi%2Fprofile"},
		{http.MethodPost, "/api/log", "/login"},
	}
	for _, c := range cases {
		rec := do(h, c.method, c.path, nil)
		if rec.Code != http.StatusFound {
			t.Errorf("%s %s = %d, quería 302", c.method, c.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Location"); got != c.wantLoc {
			t.Errorf("%s %s Location = %q, quería %q", c.method, c.path, got, c.wantLoc)
		}
	}
}

func TestLoginFormRendersAndSetsCSRF(t *testing.T) {
	h := newServer(t)

	rec := do(h, http.MethodGet, "/login?next=/rutina", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /login = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`name="csrf"`, `name="password"`, `name="email"`, `value="/rutina"`, "/static/css/app.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("el HTML del login no contiene %q", want)
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, quería no-store", rec.Header().Get("Cache-Control"))
	}
	if formCSRF(t, rec) == "" {
		t.Error("no se pudo leer el token csrf del formulario")
	}
	csrf := cookieByName(rec, "fdo_csrf")
	if csrf == nil || !csrf.HttpOnly || !csrf.Secure {
		t.Error("la cookie csrf debe existir, ser HttpOnly y Secure")
	}
}

func TestLoginRejectsBadCSRF(t *testing.T) {
	h := newServer(t)

	form := url.Values{"email": {testUser}, "password": {testPass}, "csrf": {"basura"}}
	rec := do(h, http.MethodPost, "/login", form)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST sin CSRF válido = %d, quería 403", rec.Code)
	}
	if cookieByName(rec, auth.SessionCookie) != nil {
		t.Fatal("no debe emitir sesión con CSRF inválido")
	}
}

func TestLoginWrongCredentialsAreGeneric(t *testing.T) {
	h := newServer(t)

	for _, tc := range []struct{ name, email, pass string }{
		{"clave mala", testUser, "otra-clave"},
		{"usuario inexistente", "otro@example.com", testPass},
		{"vacíos", "", ""},
	} {
		rec, sess := loginFlow(t, h, tc.email, tc.pass, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: código = %d, quería 401", tc.name, rec.Code)
		}
		if sess != nil {
			t.Errorf("%s: no debe emitir cookie de sesión", tc.name)
		}
		if !strings.Contains(rec.Body.String(), "Usuario o clave incorrectos") {
			t.Errorf("%s: falta el mensaje genérico", tc.name)
		}
		if strings.Contains(strings.ToLower(rec.Body.String()), "no existe") {
			t.Errorf("%s: filtra si el usuario existe", tc.name)
		}
	}
}

func TestLoginSuccessSetsHardenedCookie(t *testing.T) {
	h := newServer(t)

	rec, sess := loginFlow(t, h, testUser, testPass, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login ok = %d, quería 303 (%s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, quería /", loc)
	}
	if sess == nil {
		t.Fatal("no se emitió cookie de sesión")
	}
	if !sess.HttpOnly {
		t.Error("la cookie debe ser HttpOnly")
	}
	if !sess.Secure {
		t.Error("la cookie debe ser Secure")
	}
	if sess.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, quería Lax", sess.SameSite)
	}
	if sess.Path != "/" {
		t.Errorf("Path = %q, quería /", sess.Path)
	}
	if sess.MaxAge != 30*24*60*60 {
		t.Errorf("MaxAge = %d, quería %d (30 días)", sess.MaxAge, 30*24*60*60)
	}
}

func TestSessionGrantsAccessAndRenews(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	h := guard(t, func(c *auth.Config) { c.Now = clock.Now })

	_, sess := loginFlow(t, h, testUser, testPass, "")
	if sess == nil {
		t.Fatal("sin cookie de sesión")
	}
	exp0 := sessionExp(t, sess)

	// 26 días después: todavía vale y el request la estira otros 30 días.
	clock.advance(26 * 24 * time.Hour)
	rec := do(h, http.MethodGet, "/rutina", nil, sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rutina con sesión = %d, quería 200", rec.Code)
	}
	renewed := cookieByName(rec, auth.SessionCookie)
	if renewed == nil {
		t.Fatal("la sesión no se renovó en el request")
	}
	want := clock.Now().Add(auth.SessionTTL).Unix()
	if got := sessionExp(t, renewed); got != want || got <= exp0 {
		t.Errorf("vencimiento renovado = %d, quería %d (anterior %d)", got, want, exp0)
	}

	// Y la cookie renovada sirve.
	if rec := do(h, http.MethodGet, "/progreso", nil, renewed); rec.Code != http.StatusOK {
		t.Fatalf("GET /progreso con cookie renovada = %d, quería 200", rec.Code)
	}

	// Pasado el vencimiento vuelve a pedir login.
	clock.advance(auth.SessionTTL + time.Hour)
	if rec := do(h, http.MethodGet, "/", nil, renewed); rec.Code != http.StatusFound {
		t.Fatalf("sesión vencida = %d, quería 302", rec.Code)
	}
}

func TestTamperedSessionCookieIsRejected(t *testing.T) {
	h := newServer(t)

	_, sess := loginFlow(t, h, testUser, testPass, "")
	if sess == nil {
		t.Fatal("sin cookie de sesión")
	}
	parts := strings.Split(sess.Value, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("payload mal codificado: %v", err)
	}
	payload := strings.Replace(string(raw), testUser, "otro@example.com", 1)
	bad := *sess
	bad.Value = base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + parts[1]

	if rec := do(h, http.MethodGet, "/", nil, &bad); rec.Code != http.StatusFound {
		t.Fatalf("cookie adulterada = %d, quería 302 al login", rec.Code)
	}
}

func TestRotatingSecretInvalidatesSessions(t *testing.T) {
	h := newServer(t)
	_, sess := loginFlow(t, h, testUser, testPass, "")
	if sess == nil {
		t.Fatal("sin cookie de sesión")
	}

	rotated := guard(t, func(c *auth.Config) { c.SessionSecret = secret + "-rotado" })
	if rec := do(rotated, http.MethodGet, "/", nil, sess); rec.Code != http.StatusFound {
		t.Fatalf("cookie con secreto viejo = %d, quería 302", rec.Code)
	}
}

func TestNextOnlyAllowsInternalPaths(t *testing.T) {
	h := newServer(t)

	for _, next := range []string{"//evil.example", "https://evil.example/x", "/\\evil", "javascript:alert(1)", "http://evil.example"} {
		rec, sess := loginFlow(t, h, testUser, testPass, next)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("next=%q devolvió %d", next, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("next=%q: Location = %q, quería /", next, loc)
		}
		if sess == nil {
			t.Errorf("next=%q: falta la cookie de sesión", next)
		}
	}

	rec, _ := loginFlow(t, h, testUser, testPass, "/rutina?date=2026-10-01")
	if loc := rec.Header().Get("Location"); loc != "/rutina?date=2026-10-01" {
		t.Errorf("Location = %q, quería /rutina?date=2026-10-01", loc)
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	h := newServer(t)

	_, sess := loginFlow(t, h, testUser, testPass, "")
	if sess == nil {
		t.Fatal("sin cookie de sesión")
	}

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := do(h, method, "/logout", nil, sess)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s /logout = %d, quería 303", method, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/login" {
			t.Errorf("%s /logout Location = %q, quería /login", method, loc)
		}
		// Una sola cookie de sesión en la respuesta: la que borra.
		all := cookies(rec, auth.SessionCookie)
		if len(all) != 1 {
			t.Fatalf("%s /logout emitió %d cookies de sesión, quería 1 (la que borra)", method, len(all))
		}
		if v := all[0]; v.Value != "" || v.MaxAge > 0 || !v.HttpOnly || !v.Secure {
			t.Errorf("%s /logout: cookie no limpia (value=%q maxage=%d)", method, v.Value, v.MaxAge)
		}
		if cookieByName(rec, "fdo_csrf") == nil {
			t.Errorf("%s /logout: no borró la cookie csrf", method)
		}
	}
}

func TestRateLimitPerIP(t *testing.T) {
	h := newServer(t)

	for i := 0; i < 5; i++ {
		rec, _ := loginFlow(t, h, testUser, "mala", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("intento %d = %d, quería 401", i+1, rec.Code)
		}
	}
	rec, sess := loginFlow(t, h, testUser, testPass, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("intento 6 = %d, quería 429", rec.Code)
	}
	if sess != nil {
		t.Fatal("no debe emitir sesión cuando está bloqueado")
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("falta Retry-After en el 429")
	}
}

func TestRateLimitUsesCloudflareIPOnlyFromPrivateOrigin(t *testing.T) {
	h := newServer(t)

	post := func(remote, cf string) *httptest.ResponseRecorder {
		get := httptest.NewRequest(http.MethodGet, "/login", nil)
		get.RemoteAddr = remote
		if cf != "" {
			get.Header.Set("CF-Connecting-IP", cf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, get)

		csrf := cookieByName(w, "fdo_csrf")
		form := url.Values{"email": {testUser}, "password": {"mala"}, "csrf": {formCSRF(t, w)}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = remote
		if cf != "" {
			req.Header.Set("CF-Connecting-IP", cf)
		}
		req.AddCookie(csrf)
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		return out
	}

	// cloudflared en loopback: cada IP real de Cloudflare tiene su propio cupo.
	for i := 0; i < 5; i++ {
		if rec := post("127.0.0.1:5555", "198.51.100.7"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("fallo %d desde CF = %d, quería 401", i+1, rec.Code)
		}
	}
	if rec := post("127.0.0.1:5555", "198.51.100.7"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP de CF agotada = %d, quería 429", rec.Code)
	}
	if rec := post("127.0.0.1:5555", "198.51.100.8"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("otra IP de CF = %d, quería 401 (cupo independiente)", rec.Code)
	}

	// Origen público: el header se ignora, manda el RemoteAddr.
	for i := 0; i < 5; i++ {
		if rec := post("203.0.113.5:4444", fmt.Sprintf("1.2.3.%d", i+1)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("fallo %d desde origen público = %d, quería 401", i+1, rec.Code)
		}
	}
	if rec := post("203.0.113.5:4444", "9.9.9.9"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("header falsificado evadió el límite: %d, quería 429", rec.Code)
	}
}

func TestNewRejectsWeakConfig(t *testing.T) {
	weak, err := bcrypt.GenerateFromPassword([]byte("x"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash débil: %v", err)
	}
	cases := map[string]auth.Config{
		"sin usuario":   {PasswordHash: string(hashFor(t)), SessionSecret: secret},
		"sin hash":      {User: testUser, SessionSecret: secret},
		"hash inválido": {User: testUser, PasswordHash: "no-es-bcrypt", SessionSecret: secret},
		"coste bajo":    {User: testUser, PasswordHash: string(weak), SessionSecret: secret},
		"secreto corto": {User: testUser, PasswordHash: string(hashFor(t)), SessionSecret: "corto"},
	}
	for name, cfg := range cases {
		if _, err := auth.New(cfg); err == nil {
			t.Errorf("%s: auth.New debería fallar", name)
		}
	}
}

// TestCSRFTokenYValidacion cubre la API pública de CSRF que usa el panel web:
// el token se deriva de la cookie, así que sin cookie no hay token válido.
func TestCSRFTokenYValidacion(t *testing.T) {
	g, err := auth.New(auth.Config{
		User:          testUser,
		PasswordHash:  string(hashFor(t)),
		SessionSecret: secret,
	})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	// 1) Emitir el token: tiene que venir con su cookie.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	token := g.CSRFToken(rec, req)
	if token == "" {
		t.Fatal("CSRFToken devolvió vacío")
	}
	var csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "fdo_csrf" {
			csrfCookie = c
		}
	}
	if csrfCookie == nil {
		t.Fatal("CSRFToken no emitió la cookie fdo_csrf")
	}

	// 2) POST con cookie + token: válido.
	form := url.Values{"csrf": {token}}
	post := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(csrfCookie)
	if err := post.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if !g.ValidCSRFToken(post) {
		t.Error("el token correcto debería validar")
	}

	// 3) Token inventado: inválido.
	bad := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{"csrf": {"inventado"}}.Encode()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.AddCookie(csrfCookie)
	if err := bad.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if g.ValidCSRFToken(bad) {
		t.Error("un token inventado no debería validar")
	}

	// 4) Sin cookie: inválido aunque el token exista.
	noCookie := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	noCookie.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := noCookie.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if g.ValidCSRFToken(noCookie) {
		t.Error("sin cookie CSRF no debería validar")
	}

	// 5) Header X-CSRF-Token (clientes que no mandan formulario).
	hdr := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	hdr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hdr.Header.Set("X-CSRF-Token", token)
	hdr.AddCookie(csrfCookie)
	if err := hdr.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if !g.ValidCSRFToken(hdr) {
		t.Error("el token por header debería validar")
	}
}
