# Publicación pública: https://fit.solisjavier.com.ar

Publicado por el agente SRE (Qwerty) el 2026-10-22. Estado: **funcionando, verificado end-to-end**.

## Resultado

| Ítem | Valor |
|---|---|
| URL | https://fit.solisjavier.com.ar |
| `GET /` | 200 (`x-served-by: fit.solisjavier.com.ar`) — si el login está activo devuelve 302 a `/login`, es correcto |
| `GET /api/health` | 200 `{"foods":49,"ok":true}` — **público**, sin auth (lo usa el healthcheck) |
| TLS | Edge Cloudflare, Google Trust Services WE1, `CN=solisjavier.com.ar` + SAN `*.solisjavier.com.ar`, válido hasta 2026-12-05, `ssl_verify_result=0` |
| HTTP → HTTPS | 301 a `https://fit.solisjavier.com.ar/` (setting zonal *Always Use HTTPS*, ya activo en el zone) |
| Mecanismo | **Cloudflare Tunnel (ingress) → NPM (nginx-proxy) → container `fitordie:3007`** |
| Puerto 3007 | Ya NO se publica en `0.0.0.0`. Solo `127.0.0.1:3007` y `100.100.210.126:3007` (tailnet). Verificado: `132.145.142.77:3007` no conecta |

## Arquitectura (cómo está armado el homelab)

```
Internet ──TLS (edge CF)──> Cloudflare ──tunnel──> cloudflared (red proxy)
                                                     │ ingress: hostname → http://nginx-proxy
                                                     ▼
                                              nginx-proxy (NPM, :80)
                                                     │ proxy_host: domain → forward_host:port
                                                     ▼
                                              fitordie:3007 (red proxy, DNS interno docker)
```

- **TLS se termina en el edge de Cloudflare.** NPM no tiene certificados (tabla `certificate` vacía) y ningún
  proxy host escucha en 443: los 46 subdominios existentes funcionan así. Se copió ese mismo mecanismo, no se
  inventó uno nuevo.
- cloudflared es token-based: el ingress NO está en un YAML local, vive en la config remota del tunnel
  (`70a6ad37-baa8-400b-8f72-ebd027e1022a` = "Superserver"), editable por dashboard o por API.
- NPM resuelve el backend por **nombre de contenedor** dentro de la red `proxy`, por eso `fitordie` se
  conectó a esa red (antes estaba sólo en `fitordie_default`).

## Cambios aplicados (4 piezas)

### 1. DNS — CNAME `fit` → tunnel

Zone `7d13c68faaf8d7caa56fa97bce767d48` (solisjavier.com.ar), registro **nuevo** (no existía, ni wildcard):

```
CNAME  fit.solisjavier.com.ar  →  70a6ad37-baa8-400b-8f72-ebd027e1022a.cfargotunnel.com  (proxied=true)
```

### 2. Tunnel ingress — 1 regla nueva

`PUT /accounts/<acct>/cfd_tunnel/70a6ad37-.../configurations` agregando, antes del catch-all `http_status:404`:

```json
{ "hostname": "fit.solisjavier.com.ar", "service": "http://nginx-proxy" }
```

Backup del config previo (48 reglas, version 458): `~/.hermes/profiles/sre-agent/cache/scratch/tunnel_config_before.json`.
Verificado post-PUT: 49 reglas, version 459, resto intacto.

### 3. NPM — proxy host id 27

Fila en `proxy_host` de `/data/database.sqlite` (id 27) + `/data/nginx/proxy_host/27.conf` generado a partir de
la plantilla real de `ssa` (validado con `nginx -t` y recargado con `nginx -s reload`, **sin downtime**).

```
domain_names   = ["fit.solisjavier.com.ar"]
forward_host   = fitordie
forward_port   = 3007
forward_scheme = http
block_exploits = 1
ssl_forced=0, certificate_id=0   # TLS en el edge, igual que el resto
advanced_config= HSTS + X-Frame-Options + nosniff + XSS + Referrer-Policy + CSP (idéntico a los otros hosts)
```

Acceso log: `/data/logs/proxy-host-27_access.log`.

### 4. Contenedor — red `proxy` + puerto acotado

`docker-compose.yml`:

```yaml
    ports:
      - "127.0.0.1:3007:3007"
      - "100.100.210.126:3007:3007"   # Tailscale (solo tailnet)
    networks:
      - default
      - proxy
...
networks:
  proxy:
    external: true
```

Aplicado con `docker compose up -d --no-build --force-recreate`. Contenedor: `healthy`, IP en `proxy` `172.18.0.17`.

## Verificación (repetible)

```bash
curl -sI https://fit.solisjavier.com.ar/                    # 200 (o 302 /login)
curl -s  https://fit.solisjavier.com.ar/api/health          # {"foods":49,"ok":true}

# Si el resolver del host tiene NXDOMAIN cacheado (recién creado el DNS):
curl -s --resolve fit.solisjavier.com.ar:443:104.21.16.160 https://fit.solisjavier.com.ar/api/health

# cert del edge
echo | openssl s_client -connect 104.21.16.160:443 -servername fit.solisjavier.com.ar 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates

# redirect 80 -> 443 (a nivel edge)
curl -sI --resolve fit.solisjavier.com.ar:80:104.21.16.160 http://fit.solisjavier.com.ar/   # 301

# 3007 NO expuesto
curl -s -m 5 -o /dev/null -w '%{http_code}\n' http://132.145.142.77:3007/api/health   # 000
ss -lntp | grep 3007                                                                  # solo 127.0.0.1 y 100.100.210.126
```

## Cómo revertir (en este orden)

```bash
# 1) NPM: borrar proxy host 27 (UI :81 o DB) y su conf
sudo docker exec nginx-proxy rm -f /data/nginx/proxy_host/27.conf
sudo docker exec nginx-proxy nginx -t && sudo docker exec nginx-proxy nginx -s reload
#    (si además querés sacar la fila de la DB: NPM admin → Proxy Hosts → delete,
#     o sqlite: DELETE FROM proxy_host WHERE id=27;  y copiar la DB al contenedor)

# 2) Tunnel: quitar la regla de fit y re-PUTear el config
#    GET  /accounts/<acct>/cfd_tunnel/70a6ad37-.../configurations
#    quitar el objeto {"hostname":"fit.solisjavier.com.ar", ...} y PUT de nuevo
#    (o dashboard Zero Trust → Networks → Tunnels → Superserver → Public Hostnames → delete)

# 3) DNS: borrar el CNAME fit
#    dashboard → DNS → fit → delete   (id: cc3aacbde4be5060d822348a6835318f)

# 4) Contenedor: volver al compose anterior (puerto 0.0.0.0:3007 + sin red proxy)
cd /home/ubuntu/self-hosted/FitOrDie/app && git diff docker-compose.yml   # ver el cambio
#    editar ports/networks a como estaban y: docker compose up -d --no-build --force-recreate
```

Nada acá toca los otros 46 hostnames: no se reinició `nginx-proxy` (solo `reload`), no se reinició
`cloudflared`, y el PUT del tunnel preservó todas las reglas existentes.

## Notas / deuda

- **OCI Security List**: el puerto 3007 ya no tiene listener público (`000` desde la IP pública), así que no
  hay superficie aunque la regla de ingress exista. Si querés cerrarla formalmente hace falta la consola/OCI
  CLI con credenciales (no hay `~/.oci` en el VPS).
- **NPM no regenera `proxy_host/*.conf` al bootear** (esta versión sólo escribe el .conf cuando el host se
  crea/edita por API/UI). Por eso el estado durable son *las dos cosas*: la fila en la DB **y** el .conf.
  Si tocás el host desde la UI, NPM lo re-renderiza desde la DB y queda consistente.
- `/api/health` queda sin auth a propósito (healthcheck interno + verificación externa). Todo lo demás puede
  exigir login sin afectar el monitoreo.
- **RESUELTO 2026-10-22**: `music.solisjavier.com.ar` (502, no existía `navidrome`) y
  `home.solisjavier.com.ar` (401, auth de Homepage) **ya no están expuestos**: se borraron DNS, ingress del
  tunnel y proxy host de NPM. Detalle, backups y rollback en la sección siguiente.

---

# Baja de exposición: `music` y `home` (2026-10-22)

Pedido de Javi: sacar dos subdominios que quedaron rotos/inactivos. **No se borró ningún contenedor ni dato**:
`homepage` sigue `Up 4 days (healthy)` con sus volúmenes; sólo se le quitó la puerta de entrada pública.

## Estado previo (medido, no supuesto)

| Host | Antes | Causa real |
|---|---|---|
| `music.solisjavier.com.ar` | `502` | El ingress del tunnel apuntaba **directo** a `http://navidrome:4533` y el contenedor `navidrome` no existe (`cloudflared`: `lookup navidrome on 127.0.0.11:53: server misbehaving`). Nunca tuvo proxy host en NPM. |
| `home.solisjavier.com.ar` | `401` | Homepage (`homepage:3000`) con su auth propio, vía ingress `http://nginx-proxy` + proxy host NPM **id 23**. |
| `fit.solisjavier.com.ar` | `302` → `/login` | Referencia de control: no se tocó. |

## Backups (antes de tocar nada)

Directorio: `/home/ubuntu/backup-scripts/limpieza-subdominios-20261022-223625/`

| Archivo | Contenido |
|---|---|
| `tunnel_config_before.json` | Config remota **completa** del tunnel `70a6ad37-…022a`: version **459**, 49 reglas |
| `tunnel_config_after.json` | PUT aplicado: version **460**, 47 reglas |
| `npm-database.sqlite` | `/data/database.sqlite` de `nginx-proxy` pre-cambio (22 filas en `proxy_host`) |
| `npm-proxy_host.bak/` | Todo `/data/nginx/proxy_host/` pre-cambio (incluye `23.conf`) |
| `23.conf.bak` + `npm_host_23_row.json` | El vhost y la fila completa del host 23 |
| `dns_deleted.json` | Registro DNS completo (id/tipo/contenido/proxied/ttl) + respuesta del DELETE |
| `SHA256SUMS.txt` | Hashes de los backups |
| `RESTORE.md` / `revert.sh` | Runbook de rollback + script idempotente (`bash revert.sh`) |

## Qué se borró (3 capas)

### 1. DNS — Cloudflare, zone `7d13c68faaf8d7caa56fa97bce767d48`

| Host | Record id | Tipo | Apuntaba a |
|---|---|---|---|
| `music.solisjavier.com.ar` | `45c09045d097ad2dbe25bd0219cdf866` | CNAME (proxied) | `70a6ad37-…022a.cfargotunnel.com` |
| `home.solisjavier.com.ar` | `c8166817b4639637b5985d06e54814a5` | CNAME (proxied) | `70a6ad37-…022a.cfargotunnel.com` |

`DELETE /zones/<zone>/dns_records/<id>` → `success: true`. Registros en la zona: 40 → **38**. No hay
wildcard en la zona (verificado), así que no quedó ningún comodín sirviendo estos nombres.

### 2. Tunnel ingress — `PUT /accounts/<acct>/cfd_tunnel/70a6ad37-…022a/configurations`

Se quitaron **2 reglas** y se re-PUTeó el config completo (el resto quedó intacto):

```diff
- { "hostname": "home.solisjavier.com.ar",  "service": "http://nginx-proxy" }
- { "hostname": "music.solisjavier.com.ar", "service": "http://navidrome:4533" }
```

Reglas: 49 → **47**, version **459 → 460**, catch-all `{"service":"http_status:404"}` sigue al final,
`warp-routing` intacto. No hizo falta reiniciar `cloudflared` (toma la config remota por polling, ~8 s).

### 3. NPM — `nginx-proxy`

| Acción | Detalle |
|---|---|
| Fila `proxy_host` id **23** | `home.solisjavier.com.ar` → `homepage:3000`. Borrada. Filas: 22 → **21** |
| `/data/nginx/proxy_host/23.conf` | Borrado |
| `music` | No tenía proxy host (nunca lo tuvo) — no hubo nada que borrar en NPM |

Se hizo con la DB en caliente (copia afuera → `DELETE` → copia adentro) y luego
`nginx -t` (OK) + `nginx -s reload` — **sin reiniciar** el contenedor, cero downtime para el resto.
La API de NPM no se usó porque la credencial de `.sre-secrets.env` no es la del panel (`Invalid email or password`).

## Verificación (real, post-cambio)

```bash
# autoritativo Cloudflare
dig music.solisjavier.com.ar A @andronicus.ns.cloudflare.com   # NXDOMAIN
dig home.solisjavier.com.ar  A @andronicus.ns.cloudflare.com   # NXDOMAIN

# cloudflared: se acabaron los errores de navidrome (último 01:36Z, cambio 01:37Z)

# si el resolver cachea (acá el de OCI, 169.254.169.254, con TTL 300), el request igual NO llega al tunnel:
curl https://home.solisjavier.com.ar/   # 530 error code 1016  (= mismo resultado que un host inexistente)
curl https://music.solisjavier.com.ar/  # 000 / no resuelve
```

`530 / 1016` es lo que devuelve Cloudflare para un hostname que ya no existe en la zona: se comprobó
comparando contra un nombre inventado (`zzzztest.solisjavier.com.ar`) — misma respuesta. **Ya no hay 502
ni 401**, y cloudflared no registra más intentos contra `navidrome`.

Control de regresión (todos siguen igual):

```
notas 200 · grafana 302 · ssa 200 · boveda 200 · draw 200 · rss 302 · guardar 200
speedtest 200 · htop 401 (pre-existente) · lidarr 302 · llm 200 · doom 302 · root 200
fit / → 302 https://fit.solisjavier.com.ar/login · fit /api/health → 200 {"foods":49,"ok":true}
n8n 502 (pre-existente, contenedor n8n no está corriendo)
```

Verificación interna (dentro de la red `proxy`, `Host: home.solisjavier.com.ar` → `http://nginx-proxy`)
devuelve la página "Default Site" de NPM: no hay vhost que matchee, el dominio ya no está ruteado.

## Rollback

`bash /home/ubuntu/backup-scripts/limpieza-subdominios-20261022-223625/revert.sh` repone las 3 capas
(DNS → ingress 459 → fila 23 + `23.conf` + reload) y verifica. Detalle manual en `RESTORE.md`.
Único aviso: reponer `music` deja el 502 de vuelta hasta que exista el contenedor `navidrome`.

## Pendientes / observaciones (RESUELTOS el 2026-10-23 01:45Z — ver sección final "Limpieza: pendientes cerrados")

- `~/self-hosted/observability/prometheus/prometheus.yml` (líneas 59, 62, 93, 96) sigue con targets
  blackbox a `music` y `home` → van a quedar en rojo en Prometheus/Grafana (no hay regla de alerta sobre
  `probe_success`, así que no spamea Telegram). Limpiar targets + `POST /-/reload` cuando Javi diga.
- `~/self-hosted/uptime-kuma-import.json` (líneas 248, 338) tiene monitores viejos para esos hosts, pero
  `uptime-kuma` **no está corriendo** (el proxy host 19 apunta a un contenedor inexistente → `monitor`
  también da 502; otro pendiente aparte).
- `~/self-hosted/homepage/docker-compose.yml` mantiene `HOMEPAGE_ALLOWED_HOSTS=home.solisjavier.com.ar`
  (config del contenedor, no exposición). Si algún día se republica con otro nombre, actualizarlo.
- `homepage` quedó accesible sólo por red interna: `http://homepage:3000` desde la red `proxy`.

---

# Limpieza de subdominios: pendientes cerrados (2026-10-23 01:45Z / 22:45 -03)

Cierra los 3 pendientes de la sección anterior. **No se tocó `fit.solisjavier.com.ar` ni ningún otro host activo**
(control de regresión al final). Todo idempotente y con rollback probado.

## 1. Prometheus — targets blackbox de `music`, `home` (y `monitor`) fuera

Dato importante: el archivo que **realmente** lee Prometheus es
`/home/ubuntu/stacks/monitoring/prometheus/prometheus.yml` (bind mount ro → `/etc/prometheus`).
`~/self-hosted/observability/prometheus/prometheus.yml` es una **copia del repo que no está en uso** (difieren:
la copia no tiene `alerting`/`rule_files`, tiene `node-exporter` y `cadvisor:8080`). Se corrigieron **los dos**
para que no vuelva el target por copiar el viejo.

Se quitaron **6 targets** (no 4): `music` y `home` en `blackbox-http` y `blackbox-tls`, más `monitor` en ambos
jobs — `monitor` se dió de baja en el punto 3, dejarlo habría generado rojo nuevo.

| Paso | Resultado real |
|---|---|
| `promtool check config` (contenedor, archivo vivo) | `SUCCESS: 1 rule files found` + `SUCCESS: ... valid prometheus config file syntax` (8 rules) |
| `promtool check config` (copia del repo, `docker run prom/prometheus`) | `SUCCESS: ... valid` |
| Recarga | `POST http://172.22.0.2:9090/-/reload` → **HTTP 200** (`--web.enable-lifecycle` ya estaba). Prometheus no publica 9090 al host: se llega por la red `monitoring_internal` (172.22.0.2) |
| Targets activos | 49 → **43** (16 `blackbox-http` + 16 `blackbox-tls`) |
| Referencias restantes a `music.`/`home.`/`monitor.` | **0** |
| `probe_success == 0` | quedan `voice`, `n8n`, `portainer` — **pre-existentes, fuera de alcance** (n8n y portainer no tienen contenedor corriendo) |

Nota de método: en blackbox el target figura `up` aunque el sitio esté caído; el rojo real es `probe_success == 0`.
Por eso la verificación se hizo por métrica, no por estado del target.

## 2. uptime-kuma — monitores viejos podados (servicio detenido)

`uptime-kuma` **no existe como contenedor** (`docker ps -a` no lo lista; el compose quedó intacto, no se borró nada
de la app). Como la DB estaba cerrada limpia (sin `-wal`/`-shm`), se podó directo:

| Ítem | Antes | Después |
|---|---|---|
| `kuma.db` → tabla `monitor` | 17 | **15** (borrados 8 `navidrome`/music y 11 `homepage`/home) |
| heartbeats de esos monitores | 81.799 | 0 (borrados en cascada + 2 filas `monitor_tls_info`) |
| `pragma integrity_check` | — | **ok** |
| `uptime-kuma-import.json` | 19 monitores | **16** (quitados uptime-kuma/monitor, navidrome/music, homepage/home) |

Los monitores de **otros** hosts quedaron intactos: n8n, portainer, lidarr, linkwarden, freellm (llm/models/auth),
beszel, speedtest, trilium-notas, excalidraw, freshrss, vaultwarden, saa-quiz, grafana.

## 3. `monitor.solisjavier.com.ar` — exposición eliminada (no se levantó el contenedor)

**Decisión: dar de baja la exposición, no resucitar uptime-kuma.** Por qué: el monitoreo ya corre con
`grafana + prometheus + alertmanager + alert-telegram` (`grafana.solisjavier.com.ar` 302, activo), uptime-kuma está
de baja desde ~10/10 (última escritura de la DB) y levantarlo habría sumado un servicio sin mantener, con doble
monitoreo y alertas duplicadas. Baja = mismo patrón que `music`/`home`, cero riesgo para el resto.

| Capa | Acción | Verificación |
|---|---|---|
| DNS (CF zone `7d13c68f…`) | CNAME `monitor` → `70a6ad37-….cfargotunnel.com` **borrado** | consulta a CF: sin registros con ese nombre (38 registros en la zona) |
| Tunnel `Superserver` (`70a6ad37-…`) | `PUT /cfd_tunnel/<id>/configurations` sin la regla de `monitor` | v460 (47 reglas) → **v463 (46 reglas)**; diff: solo desapareció `monitor`, 0 reglas agregadas |
| NPM | fila `proxy_host` id **19** (`uptime-kuma:3001`) borrada + `19.conf` removido del vhost dir | `proxy_host` = 20 filas, 0 con `monitor`; `nginx -t` ok + `nginx -s reload` (sin downtime) |

Resultado: **502 → `530 / 1016`** (hostname sin ingress en el tunnel) y después NXDOMAIN cuando expire el cache del
resolver. Es exactamente lo mismo que devuelven hoy `music`/`home`: exposición eliminada, no "servicio caído".

## Control de regresión (hosts activos, después de todo)

```
fit /api/health 200 · grafana 302 · notas 200 · boveda 200 · htop 401 (pre-existente) · draw 200 · ssa 200
music NXDOMAIN · home NXDOMAIN · monitor 530/000 (sin DNS/ingress)
Probes: 43 targets, 0 referencias a music/home/monitor, 0 fallos nuevos
```

## Backup y rollback

Directorio: `/home/ubuntu/backups/subdominios-20261022/` (101 MB) con `SHA256SUMS.txt`:

```
prometheus.live.yml.bak              prometheus.observability.yml.bak
npm-database.sqlite.bak              npm_host_19_row.json           npm-proxy_host-19.conf.bak
tunnel_config_before.json (v460/47)  dns_records_before.json        dns_deleted_monitor.json
kuma.db.bak (105 MB)                 uptime-kuma-import.json.bak
```

**Rollback (5 capas, probado de punta a punta):**

```bash
cd /home/ubuntu/backups/subdominios-20261022
./revert.sh          # verifica SHA256, repone Prometheus+NPM+tunnel+DNS+kuma y recarga
```

- Es idempotente y **se ejecutó completo como test**: repuso configs (promtool OK, `/reload` 200), fila 19 + `19.conf`,
  ingress v462/47 reglas, el CNAME de `monitor` y `kuma.db` (17 monitores, integrity ok) — con `monitor` volviendo a
  dar **502**, que es el estado previo. Después se re-aplicó la limpieza con `./apply.sh` (idempotente, mismas
  verificaciones).
- Si se repone `monitor` sin levantar `uptime-kuma`, el 502 vuelve (esperado).
- `SKIP_KUMA=1 ./revert.sh` para no tocar la DB de kuma.

## Observaciones (no tocado)

- `n8n` y `portainer` figuran con `probe_success = 0`: los contenedores no están corriendo. Fuera de alcance.
- `voice.solisjavier.com.ar` también en 0 (su CNAME apunta al apex). Fuera de alcance.
- `homepage` sigue corriendo (`Up`, red interna) con `HOMEPAGE_ALLOWED_HOSTS=home.solisjavier.com.ar`; no se toca.
