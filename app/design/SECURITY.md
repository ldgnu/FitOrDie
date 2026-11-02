# FitOrDie — Notas de seguridad

Alcance: app personal, un solo usuario (Javi), servida en el VPS por Tailscale.
No hay registro público, no hay multiusuario, no hay datos de terceros. Eso baja
mucho el riesgo, pero no lo elimina: la app guarda peso, comida y rutina, y el
teléfono puede quedar abierto en la mesa del gimnasio.

Documento vivo: escribir acá cada decisión y cada hallazgo. Lo que no está
escrito, no existe.

---

## 1. Modelo de amenaza (corto)

| Activo | Riesgo realista | Mitigación |
|---|---|---|
| Historial de comidas/peso | alguien con acceso al teléfono desbloqueado | PIN + sesión corta (defensa en profundidad) |
| Credenciales del LLM (`LLM_API_KEY`) | filtrarse en repo, imagen Docker o logs | `.env` fuera de git, env vars en runtime, nunca al cliente |
| DB SQLite | corrupción o borrado accidental | volumen Docker + backup diario |
| Servicio en :3007 | exposición fuera de la tailnet | bind local + `tailscale serve`, sin Funnel |
| Repo | publicar algo con secretos o datos personales | gate pre-publicación (§7) |

Fuera de alcance: ataques de red dirigidos, compromiso del VPS, cadena de
suministro de Go. Igual se aplican las mitigaciones baratas de §5.

---

## 2. Secretos: regla dura

- **Nunca** en el repo: `.env`, tokens, claves, PIN, dumps de DB, fotos.
  `.gitignore` ya cubre `.env*`, `*.pem`, `*.key`, `secrets/`, `uploads/`.
- Variables (runtime, no build): `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`.
  Se leen con `os.Getenv` y **no** se loguean. Si falta `LLM_API_KEY`, la
  estimación por IA se desactiva y la app sigue funcionando con el parser local.
- **No** metas secretos en la imagen Docker: `ENV LLM_API_KEY=...` queda en las
  capas. Se pasan por `env_file`/`environment` del compose.
- El front no recibe secretos nunca. Ni el manifest, ni `sw.js`, ni el HTML, ni
  un endpoint `/config`. Todo lo que llega al navegador es público.
- Antes de cada push:

```bash
git grep -nEi '(api[_-]?key|secret|token|password|passwd|bearer|sk-[a-z0-9]{10,})' -- ':!*.md' ':!package-lock.json'
git log -p --all | grep -nEi 'sk-[a-z0-9]{10,}|-----BEGIN' | head
```

- Si un secreto se commiteó: **rotarlo primero**, después reescribir historia.
  Asumir que ya está comprometido; borrar el commit no alcanza.

---

## 3. Validación de entradas (server-side, sin excepciones)

El prototipo en `app/design` **no valida nada**: es una maqueta. En Go, cada
handler valida antes de tocar SQLite. Reglas mínimas:

| Campo | Regla |
|---|---|
| `meal_id` | enum: `desayuno, media, almuerzo, merienda, cena, picoteo`. Rechazar el resto. |
| `food_id` | debe existir en `foods` (query por id; si no está → 400). |
| `grams` | número, `0 < g <= 5000`, máx. 2 decimales. |
| `qty` | número, `0 < q <= 100`, si `unit != "g"`. |
| `unit` | `g` o una medida registrada para ese alimento (`measures.unit`). |
| `date` | `YYYY-MM-DD`, no futura (+1 día de tolerancia por zona horaria), no anterior a 2 años. |
| `text` (parser/IA) | string, ≤ 500 caracteres, sin control chars. Se pasa al parser como dato, nunca como instrucción. |
| `photo` | ≤ 5 MB, MIME real por magic bytes (`image/jpeg|png|webp`), se re-encodea antes de guardar. |
| `weight_kg` | `30 <= kg <= 250`, 2 decimales, un registro por día (upsert). |
| `set.kg` / `set.reps` | `0 <= kg <= 500`, `0 <= reps <= 100`, enteros. |
| `theme` | `dark|light|system`; cualquier otra cosa → default. |

Además:

- `http.MaxBytesReader(w, r.Body, 1<<20)` en todos los POST (5 MB en el de fotos).
- Decodificar con `json.Decoder` + `DisallowUnknownFields()`.
- Timeouts del server: `ReadHeaderTimeout 5s`, `ReadTimeout 15s`,
  `WriteTimeout 30s`, `IdleTimeout 60s`.
- Rechazar `Content-Type` inesperado y métodos no declarados (el routing 1.22
  con `GET /x` ya devuelve 405).

El parser de texto libre es **local y determinista**; el LLM es opcional. El
texto del usuario va al prompt como **dato delimitado**, no como instrucción:
el modelo devuelve solo `{name, grams}` y los macros se resuelven después
contra la tabla. Si el modelo devuelve un alimento que no está, va a
`unmatched`: **no se inventan macros**. Esa regla es de producto y también de
seguridad (evita que un texto raro termine en datos falsos de salud).

---

## 4. Base de datos y procesos

- SQLite con `modernc.org/sqlite`, **siempre** prepared statements
  (`db.QueryContext(ctx, "… WHERE id = ?", id)`). Cero concatenación de strings.
- `PRAGMA journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`.
- Un solo writer: `db.SetMaxOpenConns(1)` evita `database is locked`.
- Endpoint de export (`/api/export.csv`) con `Content-Disposition: attachment`
  y escapado de fórmulas (`=`, `+`, `-`, `@` al inicio de celda → prefijar `'`)
  para que Excel no ejecute nada.
- Logs: sin cuerpos de request, sin tokens, sin datos de comida. Errores al
  cliente genéricos; el detalle va al log del server.

### Hallazgo de datos (no es seguridad, pero rompe el import)

`data/alimentos.csv` tiene comillas mal balanceadas en `source_detail`, y las
columnas `sugar100` / `sodium_mg100` quedan como `""0"`, `""2"`. Un `encoding/csv`
estricto va a fallar o a guardar strings con comillas. El generador del
prototipo lo tolera a mano (`tools/build-pages.py`), pero conviene arreglar el
CSV antes de importarlo: 49 filas, un solo `sed`/regeneración y listo. Si no,
el import tiene que limpiar `"` de esos campos explícitamente y validar que
sean numéricos o vacíos (**vacío = sin dato**, no cero).

---

## 5. Exposición: Tailscale, no internet

La app escucha en `:3007`, pero **solo debe ser alcanzable desde la tailnet**
(`100.100.210.126`). Dos formas, en orden de preferencia:

**A. Bind local + `tailscale serve` (recomendado)**

```bash
# el binario escucha solo en loopback
HTTP_ADDR=127.0.0.1:3007
# Tailscale publica HTTPS dentro de la tailnet, con certificado válido
tailscale serve --bg https / http://127.0.0.1:3007
tailscale serve status
```

Ventajas: el puerto no queda escuchando en la interfaz pública del VPS, el
tráfico va cifrado dentro de WireGuard, y el certificado HTTPS hace que la PWA
y el service worker funcionen en iOS (Safari **no** habilita SW en `http://`,
solo en `https://` o `localhost`).

**B. Bind `0.0.0.0:3007` (lo que pide el brief)**

Funciona y es más simple, pero el puerto queda escuchando en la interfaz
pública; si algún día el firewall de OCI se abre por error, la app queda en
internet. Si se elige esta opción:

```bash
ss -lntp | grep 3007                 # debe escuchar, no necesariamente en 0.0.0.0
sudo iptables -L INPUT -n | head     # confirmar que OCI bloquea el ingreso
tailscale status                     # ver quién está en la tailnet
```

En ambos casos:

- **No** `tailscale funnel` (eso publica a internet). Verificar:
  `tailscale serve status` no debe mostrar `Funnel on`.
- ACLs de Tailscale: solo los dispositivos de Javi acceden al tag/host de la app.
- `X-Forwarded-For` no se confía si no hay proxy propio delante.
- El contenedor corre como usuario no-root, `read_only: true` en el rootfs,
  volumen writable solo para `fitordie.db` y `data/`, `no-new-privileges:true`,
  `mem_limit: 128m`.
- Healthcheck sin datos personales (`GET /healthz` → `ok`).

**Exposición por Cloudflare: no.** Este servicio no se publica en
`solisjavier.com.ar` ni en ningún túnel público. Si algún día hace falta,
primero pasa por revisión de este documento.

---

## 6. Auth simple: PIN (defensa en profundidad)

Tailscale autentica el dispositivo, no la persona. Un teléfono desbloqueado con
la PWA abierta entra sin fricción. El PIN cubre ese caso y nada más: no
reemplaza a Tailscale.

Diseño sugerido (simple y suficiente para un usuario):

1. **PIN de 6 dígitos** (no 4: 10⁶ combinaciones con rate limit alcanza).
2. Se guarda **hasheado**: `argon2id` (parámetros: 64 MB, 3 iteraciones) o
   `bcrypt` cost 12. Nunca en claro, nunca reversible, nunca del lado del cliente.
3. **Sesión**: al validar el PIN se emite un token aleatorio de 32 bytes
   (`crypto/rand`), se guarda su **hash** en SQLite con `expires_at` (30 días) y
   en el navegador va una cookie `HttpOnly; Secure; SameSite=Strict; Path=/`.
   Rotar el token en cada login; permitir "cerrar sesión" que borra la fila.
4. **Rate limit**: 5 intentos fallidos por IP y por dispositivo cada 15 minutos,
   con backoff exponencial y bloqueo temporal. Contador en memoria (single
   process) o en SQLite; no en el cliente.
5. **Sin bypass**: nada de `?pin=`, ni headers mágicos, ni "modo dev" en prod.
   El middleware de sesión cubre **todo** salvo `/login`, `/healthz`, `/css/`,
   `/js/`, `/icons/`, `/manifest.webmanifest`, `/sw.js`.
6. **Recuperación**: si se olvida el PIN, se borra la tabla de sesiones y se
   re-setea el PIN por CLI (`docker exec -it fitordie /fitordie pin-reset`). No
   hay endpoint público de recuperación.
7. **Primer arranque**: el PIN inicial se genera aleatorio y se imprime una sola
   vez en los logs del contenedor (o se pide por stdin en `pin-reset`). Nunca
   hardcodeado en el repo.
8. Al cerrar sesión: `Clear-Site-Data: "cache", "cookies", "storage"` (o al
   menos borrar las cachés del SW desde JS) para que no quede el día cacheado en
   el teléfono.

Nada de esto se implementa en el prototipo: `#pin-switch` en `perfil.html` es un
mock que avisa exactamente eso.

---

## 7. Gate pre-publicación (checklist del portero)

Antes de hacer público el repo, un release, una demo o abrir un puerto:

1. **Secretos**: `grep` de §2 sobre el árbol y sobre `git log -p --all`. Sin
   resultados.
2. **`.gitignore`**: `.env*`, `*.key`, `*.pem`, `secrets/`, `uploads/`,
   `*.db`, `*.db-wal`, `*.db-shm`, `screenshots` si tienen datos reales.
3. **Puertos**: `ss -lntp` + firewall; nada nuevo escuchando en `0.0.0.0` sin
   justificación. `tailscale serve status` sin Funnel.
4. **Dependencias**: `go mod tidy && govulncheck ./...`; sin CDNs en el front
   (este prototipo no carga nada externo: verificado, 0 requests de terceros).
5. **Datos personales**: nombre, edad, peso, altura, objetivos, rutina y fotos
   **no** van a un repo público ni a un manifest. Si el repo va a ser público,
   el perfil se carga por seed local y se saca de la doc.
6. **HTML/JS**: nada de `innerHTML` con datos que vengan del usuario (ver §8).
7. **Headers**: CSP, `X-Content-Type-Options: nosniff`, `Referrer-Policy:
   no-referrer`, `X-Frame-Options: DENY` (o `frame-ancestors 'none'`).

CSP sugerida (todo local, sin inline salvo el script de tema — mejor moverlo a
un archivo o usar `nonce`):

```
Content-Security-Policy:
  default-src 'self';
  img-src 'self' data:;
  style-src 'self' 'unsafe-inline';   /* el prototipo usa style="" para --v y --pct */
  script-src 'self';
  connect-src 'self';
  frame-ancestors 'none';
  base-uri 'none';
  form-action 'self'
```

Si se elimina el uso de `style` inline (pasando los porcentajes a clases o a
`data-*`), se puede sacar `'unsafe-inline'` de `style-src`. Es la deuda técnica
más grande del CSS actual.

---

## 8. Hallazgos sobre este prototipo (para no arrastrarlos)

| # | Hallazgo | Impacto | Qué hacer en Go |
|---|---|---|---|
| 1 | `js/demo.js` arma HTML con `innerHTML` usando nombres de alimentos y texto del usuario | XSS si se reusa tal cual | Renderizar del servidor con `html/template`; si queda JS, usar `textContent` y `createElement`, nunca `innerHTML` con datos |
| 2 | `localStorage` guarda comidas y pesos (demo) | datos personales en claro en el dispositivo | En producción solo `theme` en localStorage; los datos van a SQLite |
| 3 | El prototipo no tiene auth | obvio: es estático | Middleware de sesión cubriendo todo salvo estáticos y `/login` |
| 4 | `sw.js` cachea HTML | el día cacheado queda en el teléfono | Ya usa red-primero para navegación y no cachea `/api/*`; sumar borrado de caché al cerrar sesión |
| 5 | El manifest y los iconos son públicos | nada sensible | OK, pero no poner datos personales en `name`/`description` |
| 6 | `style="--v:73%"` inline en el HTML | obliga a `'unsafe-inline'` en CSP | Aceptable a corto plazo; migrar a clases cuando se toque el CSS |
| 7 | CSV con comillas rotas | import incorrecto, valores con `"` | Arreglar el CSV o limpiar en el importador (ver §4) |
| 8 | `data-mock` y toasts de demo en Perfil | confusión si se publica así | Detrás de un flag `DEMO=1` o directamente fuera del template de producción |

Verificado en el prototipo: **0 requests a dominios externos** (no hay CDN, ni
fuentes, ni analytics), **0 secretos** en HTML/CSS/JS/manifest, y el único
enlace saliente es la búsqueda de YouTube de cada ejercicio (abre en pestaña
nueva con `rel="noopener noreferrer"`).
