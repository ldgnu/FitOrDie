# FitOrDie — backend Go + SQLite

App personal de diario alimentario y seguimiento de entrenamiento. Binario único
Go (sin Node, sin Postgres, sin CGO), templates server-rendered y una API REST
mínima. Pensada para correr en Docker con poco más de 20 MB de RAM.

> `web/` (Next.js + Postgres) queda archivado como legacy. Este directorio es la
> app vigente: puerto **3007**, bind `0.0.0.0`.

## Requisitos

- Go **1.26+** (el `go.mod` pide 1.26.0 porque `modernc.org/sqlite` v1.61 lo
  exige; el brief original decía 1.22+). Local: `/home/ubuntu/.local/go/bin/go`.
- Docker + Docker Compose v2 (para el deploy).

## Build y run local

```bash
cd /home/ubuntu/self-hosted/FitOrDie/app
export PATH="$PATH:$HOME/.local/go/bin"     # si go no está en PATH

cp .env.example .env                        # sin secretos versionados
go build -o bin/fitordie ./cmd/fitordie     # binario único (~21 MB)
go test ./...                               # motor nutricional + store

./bin/fitordie seed                         # carga CSV, perfil, plan y rutina
GOMEMLIMIT=64MiB GOGC=50 ./bin/fitordie serve
# http://127.0.0.1:3007
```

Subcomandos: `serve` (default si no pasás nada), `seed`, `health`, `version`.
Flags: `-addr host:puerto`, `-db ruta`.

`seed` es idempotente: se puede correr las veces que haga falta, no duplica
alimentos, plan ni rutina. En Docker el seed corre solo en cada arranque.

## Docker

```bash
cd /home/ubuntu/self-hosted/FitOrDie/app
cp .env.example .env                    # una sola vez
docker compose up -d --build
docker compose logs -f
docker compose ps                        # debe decir (healthy)
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3007/          # 302 -> /login
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3007/api/health  # 200
```

- Imagen multi-stage: `golang:1.27-alpine` → `gcr.io/distroless/static:nonroot`.
- Usuario no-root (65532), `read_only: true`, `tmpfs /tmp`, `no-new-privileges`.
- `mem_limit: 128m`, `GOMEMLIMIT=64MiB`, `GOGC=50` (RSS medido ~20-25 MB).
- Volumen nombrado `fitordie-data` → `/data/fitordie.db`.
- Healthcheck sin shell: `fitordie health` consulta `/api/health` desde dentro.
- El CSV de alimentos va **embebido** en el binario (`data/alimentos.csv`).

Actualizar datos del CSV:

```bash
scripts/sync-data.sh          # copia ../data/alimentos.csv -> app/data/
docker compose up -d --build  # y luego: docker compose exec fitordie ./fitordie seed
```

## Estructura

```
app/
├── cmd/fitordie/main.go        # arranque, flags, subcomandos, señales, memoria
├── internal/
│   ├── config/                 # env + .env (sin dependencias externas)
│   ├── auth/                   # login, sesión HMAC, CSRF, rate limit
│   ├── nutrition/              # MOTOR: funciones puras + tests
│   ├── store/                  # SQLite: schema, queries, seeds
│   │   ├── schema.go           # DDL idempotente
│   │   ├── seed.go             # perfil Javi, Plan Fran, Mesociclo 1
│   │   └── store.go            # acceso a datos
│   ├── api/                    # API REST JSON
│   └── webui/                  # HTML server-rendered + estáticos
├── templates/                  # layout, hoy, comidas, rutina, progreso, perfil
├── static/                     # css, js, manifest.webmanifest, sw.js
├── data/                       # alimentos.csv embebido + fitordie.db (runtime)
└── design/                     # fuentes de diseño de la UI (no se sirve)
```

## Autenticación

Toda la app (páginas y API) exige sesión. Público quedan solo `/login`,
`/logout`, `/static/*`, `/manifest.webmanifest`, `/favicon.ico` y `/api/health`.

Variables en `.env` (nunca versionado). Sin las tres, `serve` no arranca:

| Variable | Qué es |
|---|---|
| `APP_USER` | email del único usuario |
| `APP_PASSWORD_HASH` | hash bcrypt de su clave (coste 12, formato `$2a$12$...`) |
| `SESSION_SECRET` | secreto HMAC de la cookie (`openssl rand -base64 48`) |

La clave en claro **no se guarda en ningún archivo**. Para rotarla se calcula un
hash nuevo y se reinicia el contenedor:

```bash
# hash bcrypt coste 12 (htpasswd emite $2y$; el parser de Go acepta $2a/$2b/$2y)
htpasswd -nbBC 12 usuario 'CLAVE-NUEVA' | cut -d: -f2
```

Diseño:

- **Sesión**: cookie `fdo_session` firmada con HMAC-SHA256 (`SESSION_SECRET`),
  `HttpOnly` + `Secure` + `SameSite=Lax` + `Path=/`, 30 días, **renovada en cada
  request**. Sin tabla de sesiones: el proceso es stateless y el volumen SQLite
  no cambia.
- **CSRF**: doble envío. `GET /login` setea `fdo_csrf` (aleatoria, HttpOnly) y el
  formulario lleva `HMAC(secreto, cookie)`. Un POST sin token válido es 403.
- **Rate limit**: 5 intentos fallidos por minuto y por IP → 429 + `Retry-After`.
  La IP sale de `CF-Connecting-IP` **solo** cuando el request llega desde una red
  privada (cloudflared corre al lado del proceso): un cliente directo no puede
  falsear el header para saltarse el límite.
- **Anti-enumeración**: bcrypt corre siempre (email erróneo o no) y el mensaje es
  siempre "Usuario o clave incorrectos", con 401.
- **`next`**: solo rutas internas (`/x`); `//host`, `http(s)://`, `/\` y
  `javascript:` caen a `/`.
- **Logout**: borra `fdo_session` y `fdo_csrf`, redirige a `/login`. Ojo: la
  sesión es stateless, así que un token ya robado sigue valiendo hasta que
  expire; para invalidar **todo** se rota `SESSION_SECRET` y se reinicia.

Consumir la API con sesión (curl):

```bash
curl -s -c /tmp/fd.jar -o /dev/null http://127.0.0.1:3007/login
CSRF=$(grep -o 'name="csrf" value="[^"]*"' /tmp/fd.jar | sed 's/.*value="//;s/"//')
curl -s -b /tmp/fd.jar -c /tmp/fd.jar -o /dev/null \
  --data-urlencode "email=$APP_USER" --data-urlencode "password=..." \
  --data-urlencode "csrf=$CSRF" http://127.0.0.1:3007/login
curl -s -b /tmp/fd.jar http://127.0.0.1:3007/api/profile
```

## API

Todo lo que sigue **exige sesión** salvo `/api/health`, que es público y no
devuelve datos personales (solo `ok` y la cantidad de alimentos).

| Método | Ruta              | Descripción                                   |
|--------|-------------------|-----------------------------------------------|
| GET    | `/api/health`     | `{"ok":true,"foods":49}` — público            |
| GET    | `/api/profile`    | perfil + objetivos                            |
| PATCH  | `/api/profile`    | edita objetivos (también acepta `POST`)        |
| GET    | `/api/day`        | día completo: comidas, entradas, totales, %   |
| POST   | `/api/log`        | carga comida (food_id o per100) → macros calc |
| DELETE | `/api/log/{id}`   | borra una entrada                             |
| GET    | `/api/foods?q=`   | busca alimentos por nombre                    |
| GET    | `/api/weight`     | historial de peso `{day,kg,note}` (`from`/`to`)|
| POST   | `/api/weight`     | registra peso del día (upsert + `note`)       |
| GET    | `/api/routine`    | Mesociclo 1 A/B/C                             |
| GET    | `/api/workouts`   | entrenamientos `from`/`to` + volumen          |
| POST   | `/api/workout`    | registra sesión + series                      |
| POST   | `/api/estimate`   | texto libre → items (no loguea)               |
| GET    | `/api/plan`       | Plan Fran (opciones A/B/C y reglas)           |

Quizá los nombres exactos de campos cambien cuando la UI definitiva los fije;
la fuente de verdad es `internal/api/api.go`.

Ejemplo:

```bash
curl -s -X POST localhost:3007/api/log -H 'Content-Type: application/json' \
  -d '{"date":"2026-10-22","meal":"almuerzo","food_id":36,"grams":250}'
# 250 g de pollo -> 412,5 kcal / 77,5 g proteína (calculado desde el CSV)
```

Pesos (nota + regla de fecha máxima):

```bash
# pesa del día, con nota
curl -s -X POST localhost:3007/api/weight -H 'Content-Type: application/json' \
  -d '{"date":"2026-10-23","kg":87.1,"note":"pesa real"}'
# -> {"date":"2026-10-23","kg":87.1,"note":"pesa real","delta_kg":-0.4,
#     "previous_kg":87.5,"profile_updated":true}

# una pesa vieja entra al historial pero NO pisa profile.weight_kg
curl -s -X POST localhost:3007/api/weight -H 'Content-Type: application/json' \
  -d '{"date":"2026-10-04","kg":96.0,"note":"pesa previa; fecha estimada, confirmar"}'

curl -s 'localhost:3007/api/weight?from=2026-10-01&to=2026-10-31'
```

Perfil editable (patch parcial, valida 20-400 kg y min<=max):

```bash
curl -s -X PATCH localhost:3007/api/profile -H 'Content-Type: application/json' \
  -d '{"target_weight_min":82,"target_weight_max":83.5,"kcal_min":2100,"kcal_max":2350}'
curl -s -X POST localhost:3007/api/profile -H 'Content-Type: application/json' \
  -d '{"height_cm":185,"protein_min":160,"protein_max":175}'   # POST = alias
```

Workouts del rango con volumen por serie y por ejercicio:

```bash
curl -s 'localhost:3007/api/workouts?from=2026-10-20&to=2026-10-26'
# -> workouts[].sets[] {exercise_id, exercise_name, set_number, weight_kg, reps, rir, volume_kg}
#    workouts[].by_exercise[] {sets, reps, volume_kg, top_weight_kg}
#    workouts[].volume_kg, total_reps + volume_kg/total_sets del rango
```

Comida por texto libre (NO loguea: el cliente confirma con `/api/log`):

```bash
curl -s -X POST localhost:3007/api/estimate -H 'Content-Type: application/json' \
  -d '{"date":"2026-10-22","meal":"almuerzo","text":"comí 250 g de pollo con arroz y ensalada"}'
# -> {"source":"local","items":[{"food_id":36,"food_name":"Pollo pechuga","grams":250,
#     "macros":{...},"matched":true,"confidence":0.9}, {arroz, porción 150 g}],
#     "unmatched":[{"text":"ensalada","reason":"sin coincidencia en foods"}],
#     "totals":{"kcal":608,...}}
```

## Verificación (lo que se corrió)

```bash
# 1. tests, vet y build
export PATH="$HOME/.local/go/bin:$PATH"
go build ./... && go vet ./... && go test ./...

# 2. seed y conteos
./bin/fitordie seed
sqlite3 data/fitordie.db "select 'foods', count(*) from foods
  union all select 'rutina', count(*) from routine_exercises
  union all select 'plan', count(*) from meal_plan"

# 3. servidor
GOMEMLIMIT=64MiB GOGC=50 ./bin/fitordie serve &
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3007/    # 302 -> /login
curl -s http://127.0.0.1:3007/api/health                          # {"foods":49,"ok":true}

# 4. autenticación (Go: internal/auth/auth_test.go, 15 casos)
#    - GET / , /rutina, /progreso, /api/profile sin sesión -> 302 /login?next=...
#    - GET /login -> 200 con csrf; POST sin csrf -> 403
#    - POST clave mala -> 401 "Usuario o clave incorrectos"
#    - POST clave buena -> 303 + fdo_session HttpOnly/Secure/SameSite=Lax/30 días
#    - 6º intento fallido -> 429 + Retry-After
#    - next=//evil , https://evil , javascript: -> /
#    - /logout -> 303 /login y cookies borradas (Max-Age=0)

# 5. memoria
ps -o rss= -p $(pgrep -x fitordie)     # KB; ~6 MB idle, ~33 MB tras 54 requests
```

Resultados medidos: 49 alimentos importados, 13 ejercicios, 22 opciones de plan,
9 reglas, `curl /` = 200 (antes del login) / 302 (con auth), RSS 6.152 KB idle y
32.936 KB después del smoke test de la API (54 checks, 0 fallos).

Smoke con auth en producción (`https://fit.solisjavier.com.ar`): `/` 302,
`/login` 200, clave mala 401, clave buena 303 → `/progreso`, `/` 200,
`/rutina` 200, `/api/profile` 200, `/logout` 303 y `/` vuelve a 302.
Datos intactos tras el rebuild: 49 alimentos, 4 pesas, 7 entradas del día.

## Reglas de datos

- **La app calcula desde datos, nunca inventa.** Si un valor falta, se muestra
  "sin datos" en la UI; no se rellena con 0 en el motor.
- Macros se escalan por gramos en `internal/nutrition` (funciones puras,
  cubiertas por tests: escalado, suma, redondeo, estados vs objetivo).
- Las entradas del diario **congelan** los macros al momento de cargar: si el
  CSV cambia después, el histórico no se reescribe.
- `data/alimentos.csv` es la tabla editable por humanos (valores por 100 g).

## Seeds incluidos

- Perfil Javi: 38 años, 1,85 m, 89,65 kg, objetivo 84-85 kg,
  2150-2400 kcal, proteína 155-170 g.
- Plan Fran: 5 comidas + picoteo con opciones A/B/C y reglas (agua, sal,
  pesaje en crudo, ajustes por velocidad de descenso).
- Rutina Mesociclo 1: día A (5 ejercicios), B (4) y C (4), con series, reps,
  RIR, descanso y nota técnica.

## Notas de implementación

- CSV legacy: `data/alimentos.csv` tenía comillas duplicadas (`""`), lo que
  rompía el parseo (8 columnas en vez de 10). El archivo ya está normalizado y,
  igual, el importador tolera las dos variantes (`normalizeFoodRecord`): si
  vuelve a aparecer el defecto, recupera `sugar100`/`sodium_mg100` del campo
  `source_detail` en vez de perder los valores.
- Sin CGO (`CGO_ENABLED=0`), driver `modernc.org/sqlite`, `journal_mode=WAL`,
  `busy_timeout=5000`, una sola conexión (menos RAM, menos locks).
- `tzdata` embebido: no depende del zoneinfo del host.
