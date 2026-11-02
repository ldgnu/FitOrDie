// Comando fitordie: servidor HTTP único (API + UI) con SQLite embebido.
//
// Un solo binario, sin CGO, sin Docker: el proceso escucha en 0.0.0.0:3007,
// sirve la UI server-rendered y la API REST, y guarda todo en data/fitordie.db.
//
//	fitordie serve    # corre el servidor (default)
//	fitordie seed     # re-siembra alimentos, plan y rutina, y sale
//	fitordie health   # GET /api/health local (para HEALTHCHECK de Docker)
//	fitordie version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	// tzdata embebido: el VPS puede no tener /usr/share/zoneinfo completo.
	_ "time/tzdata"

	"github.com/ldgnu/fitordie/internal/api"
	"github.com/ldgnu/fitordie/internal/auth"
	"github.com/ldgnu/fitordie/internal/config"
	"github.com/ldgnu/fitordie/internal/estimate"
	"github.com/ldgnu/fitordie/internal/store"
	"github.com/ldgnu/fitordie/internal/webui"
)

// version se puede fijar en build: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	cfg := config.Load()

	// Límites de memoria: GOMEMLIMIT=64MiB y GOGC=50 por defecto. Se aplican
	// también por código para que valgan aunque el proceso se lance sin env.
	debug.SetGCPercent(cfg.GCPercent)
	debug.SetMemoryLimit(cfg.MemLimitBytes)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLevel(cfg.LogLevel),
	})))

	cmd, args := splitCommand(os.Args[1:])

	fs := flag.NewFlagSet("fitordie "+cmd, flag.ExitOnError)
	addr := fs.String("addr", cfg.Addr, "dirección de escucha (host:puerto)")
	dbPath := fs.String("db", cfg.DBPath, "ruta del archivo SQLite")
	_ = fs.Parse(args)

	switch cmd {
	case "version":
		fmt.Printf("fitordie %s\n", version)
	case "health":
		os.Exit(health(*addr))
	case "seed":
		if err := runSeed(cfg, *dbPath); err != nil {
			fatal("seed", err)
		}
	case "serve":
		if err := runServer(cfg, *addr, *dbPath); err != nil {
			fatal("servidor", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "uso: fitordie [serve|seed|health|version] [-addr host:puerto] [-db ruta]\n")
		os.Exit(2)
	}
}

// splitCommand separa el subcomando de los flags ("serve -db x" / "-db x").
func splitCommand(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "serve", args
}

// runSeed re-siembra la base (alimentos, plan y rutina) sin tocar diario ni pesos.
func runSeed(cfg config.Config, dbPath string) error {
	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	csvData, err := readCSV(cfg.CSVPath)
	if err != nil {
		return err
	}
	if err := st.SeedFrom(context.Background(), true, csvData); err != nil {
		return err
	}
	n, err := st.CountFoods(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("seed OK: %d alimentos, plan y rutina cargados en %s\n", n, dbPath)
	return nil
}

// runServer levanta el servidor HTTP y espera señal de corte.
func runServer(cfg config.Config, addr, dbPath string) error {
	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx := context.Background()
	csvData, err := readCSV(cfg.CSVPath)
	if err != nil {
		return err
	}
	if err := st.SeedFrom(ctx, false, csvData); err != nil {
		return err
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		slog.Warn("zona horaria no disponible, se usa UTC-3 fijo", "tz", cfg.Timezone, "err", err)
		loc = time.FixedZone("ART", -3*60*60)
	}

	// Autenticación obligatoria: sin usuario/hash/secreto no se sirve nada.
	guard, err := auth.New(auth.Config{
		User:          cfg.AppUser,
		PasswordHash:  cfg.AppPasswordHash,
		SessionSecret: cfg.SessionSecret,
	})
	if err != nil {
		return fmt.Errorf("configurar autenticación (APP_USER / APP_PASSWORD_HASH / SESSION_SECRET): %w", err)
	}

	// Analizador de comidas: el mismo para la API y para el panel web. Sin LLM
	// configurado corre solo el parser local determinista.
	est := estimate.New(st, estimate.Options{
		LLMBaseURL: cfg.LLMBaseURL,
		LLMAPIKey:  cfg.LLMAPIKey,
		LLMModel:   cfg.LLMModel,
	})

	// El panel escribe por formularios POST: exige sesión (guard.Protect) y
	// CSRF (el guard emite y valida el token).
	ui, err := webui.New(st, loc, webui.WithCSRF(guard), webui.WithEstimator(est))
	if err != nil {
		return fmt.Errorf("parsear templates: %w", err)
	}

	mux := http.NewServeMux()
	api.NewWithOptions(st, loc, api.Options{
		LLMBaseURL: cfg.LLMBaseURL,
		LLMAPIKey:  cfg.LLMAPIKey,
		LLMModel:   cfg.LLMModel,
	}).Routes(mux)
	ui.Routes(mux)
	guard.Routes(mux)

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(guard.Protect(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	n, _ := st.CountFoods(ctx)
	slog.Info("fitordie arriba",
		"version", version,
		"addr", addr,
		"db", dbPath,
		"foods", n,
		"auth_user", cfg.AppUser,
		"memlimit_mib", cfg.MemLimitBytes>>20,
		"gogc", cfg.GCPercent,
	)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-shutdownCtx.Done():
		slog.Info("apagando")
		ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctxTimeout); err != nil {
			slog.Error("shutdown", "err", err)
		}
		return nil
	}
}

func openStore(dbPath string) (*store.Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("crear directorio de datos: %w", err)
	}
	return store.Open(dbPath)
}

// readCSV devuelve nil si no hay CSV_PATH (se usa el embebido).
func readCSV(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leer CSV_PATH %q: %w", path, err)
	}
	return b, nil
}

// health consulta /api/health en el propio proceso (0 = sano). No usa curl ni
// shell para poder correr dentro de distroless.
func health(addr string) int {
	host := addr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		h := host[:i]
		if h == "" || h == "0.0.0.0" || h == "[::]" || h == "::" {
			h = "127.0.0.1"
		}
		host = h + host[i:]
	}
	if host == "" {
		host = "127.0.0.1:3007"
	}

	c := &http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://" + host + "/api/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "health FAIL: %v\n", err)
		return 1
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "health FAIL: status %d\n", res.StatusCode)
		return 1
	}
	fmt.Printf("health OK (%s)\n", host)
	return 0
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

// logRequests loguea método, ruta, status y duración de cada request.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
