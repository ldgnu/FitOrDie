// Package config carga la configuración de FitOrDie desde variables de entorno
// y, opcionalmente, desde un archivo .env local (nunca versionado).
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config es la configuración efectiva del proceso.
type Config struct {
	Addr     string // dirección de escucha, por defecto 0.0.0.0:3007
	DBPath   string // archivo SQLite
	CSVPath  string // CSV de alimentos (vacío = autodetectar)
	Timezone string // zona horaria para "hoy"
	LogLevel string

	// LLM es opcional: si falta, la estimación por texto/foto queda deshabilitada.
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	// Memoria: límites aplicados por main con runtime/debug.
	MemLimitBytes int64
	GCPercent     int

	// Autenticación. Sin estos tres valores `serve` no arranca: la app no se
	// sirve sin login. El hash bcrypt vive en .env (gitignored) o en el entorno.
	AppUser         string
	AppPasswordHash string
	SessionSecret   string
}

// Load lee .env (si existe en el directorio actual) y luego el entorno real.
// Las variables ya presentes en el entorno tienen prioridad sobre el archivo.
func Load() Config {
	loadDotEnv(".env")

	// ADDR manda; si no está, se arma con PORT (compatibilidad docker-compose).
	addr := env("ADDR", "")
	if addr == "" {
		addr = "0.0.0.0:" + env("PORT", "3007")
	}

	return Config{
		Addr:          addr,
		DBPath:        env("DB_PATH", "data/fitordie.db"),
		CSVPath:       env("CSV_PATH", ""),
		Timezone:      env("TZ_NAME", env("TIMEZONE", "America/Argentina/Cordoba")),
		LogLevel:      env("LOG_LEVEL", "info"),
		LLMBaseURL:    env("LLM_BASE_URL", ""),
		LLMAPIKey:     env("LLM_API_KEY", ""),
		LLMModel:      env("LLM_MODEL", ""),
		MemLimitBytes: envInt64("MEM_LIMIT_BYTES", 64<<20), // 64 MiB
		GCPercent:     envInt("GC_PERCENT", 50),

		AppUser:         env("APP_USER", ""),
		AppPasswordHash: env("APP_PASSWORD_HASH", ""),
		SessionSecret:   env("SESSION_SECRET", ""),
	}
}

// LLMEnabled indica si hay un endpoint OpenAI-compatible configurado.
func (c Config) LLMEnabled() bool { return c.LLMBaseURL != "" && c.LLMModel != "" }

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func envInt64(key string, def int64) int64 {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return def
	}
	return n
}

// loadDotEnv parsea un .env mínimo (KEY=VALUE, comentarios con #, comillas
// opcionales). No pisa variables ya definidas en el entorno del proceso.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
