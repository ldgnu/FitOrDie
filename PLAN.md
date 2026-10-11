# FitOrDie — Plan de implementación (Fase 2, resumen ejecutable)

## Arquitectura

```
PWA (Next.js) ──HTTPS──► API (Next.js Route Handlers) ──► Postgres
   │                        │                              │
   │                        ├── motor nutricional (lib pura, testeable)
   │                        └── API Telegram (fase 8, autenticada + idempotente)
   └── offline mínimo: cola de registros → sync
```

- Un solo deploy: Next.js full-stack + Postgres en Docker Compose.
- Sin app nativa en el MVP. Sin backend separado (no se justifica todavía).
- Dominios separados en código: `web pública` / `área privada` / `api` /
  `nutrición` / `telegram` / `datos` / `config+secretos`.

## Modelo de datos (MVP)

- `users` — cuenta + credenciales (hash, nunca plaintext).
- `profiles` — nombre visible, unidades, zona horaria, `crudeza_level`,
  objetivos (kcal, proteína, carbos netos), prefs/alergias/disgustos.
- `foods` — alimento + fuente (`verificado|etiqueta|manual|receta`), nutrientes
  por 100 g o por unidad. Sin dato = campo nulo + flag, nunca inventado.
- `recipes` + `recipe_ingredients` — ingredientes con cantidades, porciones,
  snapshot nutricional por porción, etiquetas justificadas por reglas.
- `diary_entries` — usuario, fecha/hora consumo + zona, comida del día,
  alimento/receta, cantidad + unidad, notas. Nutrientes **calculados**, no
  guardados (se recalculan; snapshot solo si la receta cambia).
- `entry_edits` — historial de correcciones (accountability, sin borrado silencioso).
- `weights` (+ `measurements` opcional) — fecha, valor, nota. Privado siempre.
- `favorites`, `telegram_links` (chat id + token de vinculación hasheado),
  `idempotency_keys`, `audit_log` (auth, export, delete, cambios sensibles).

Reglas: todo lo personal con `user_id` + RLS/checks en backend; aislamiento
testeado entre usuarios; UTC en DB, zona del usuario en presentación.

## Motor nutricional (determinista, sin IA)

- Entrada: ingredientes + cantidades + porciones → salida: kcal/P/G/C/fibra/netos.
- Escala lineal por cantidad; por 100 g ↔ por unidad ↔ por porción.
- Keto: `netos = totales − fibra`; umbral diario configurable (default 30 g);
  veredicto en 4 niveles (compatible / con ajustes / difícil / sin datos).
- La IA solo interpreta texto ambiguo ("almorcé pollo con palta"); jamás
  inventa valores: lo no resolvible queda `sin datos`.

## Voz Testigo (modo único)

- Copy reactivo a conducta: bien→motiva, exceso/ausencia→reta, retorno→neutro.
- Disparadores solo por conducta medible; nunca por peso/fotos/antojo aislado.
- Crudeza configurable (suave/picante/sin filtro). Piso de seguridad intacto.

## MVP recortado (orden)

1. Auth real (registro/login/logout/recupero/verificación) + sesiones seguras.
2. Diario: CRUD + búsqueda + favoritos + duplicar + resumen día/semana.
3. Motor nutricional + keto (con tests).
4. Dashboard + voz Testigo + agua + peso/tendencia.
5. PWA (instalable, offline cola de registros).
6. Después: recetario (30 recetas seed), físico completo, Telegram (fase 8).

## Testing mínimo bloqueante

Auth, aislamiento entre usuarios, CRUD diario, cálculos + netos + keto,
entradas malformadas, sin-auth, sesiones vencidas, idempotencia, migraciones
tras reinicio, export/delete. Seed demo ficticio.

## Producción

Docker Compose, HTTPS, headers, rate limits, backups + restore probado,
secretos por entorno, privacy/terms, export/delete de cuenta.
