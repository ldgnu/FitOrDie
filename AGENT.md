# AGENT.md — FitOrDie (guía para agentes)

## Qué es
FitOrDie: diario alimentario + seguimiento de cambio físico. Webapp mobile-first
(PWA) con Next.js. Regla de oro: **la app calcula desde datos, nunca inventa**.
Si falta un dato se muestra "sin datos", no se estima en silencio.

## Estructura
- `web/` — app Next.js (App Router, TypeScript, Tailwind 4, Auth.js v4, Drizzle + Postgres)
  - `src/app/` — páginas y API (`src/app/api/...`)
  - `src/lib/nutrition.ts` — motor nutricional (funciones puras, deterministas)
  - `src/lib/parse-food.ts` — parser de texto libre a comida (local, sin IA)
  - `src/lib/day-summary.ts` — resumen del día (usado por API y dashboard)
  - `src/db/schema.ts` — schema de base de datos (Drizzle)
  - `scripts/` — `seed*.mts` (datos base), `load-csv.mts` (carga el CSV),
    `itest.mts` (integración E2E, 45 checks), `check-*.mts` (unitarios)
- `data/alimentos.csv` — **tabla nutricional editable por humanos**
- `docker-compose.yml` — app + Postgres 16
- `BRIEF.md`, `PLAN.md`, `FASE1.md` — concepto, plan y decisiones (leer antes de opinar)

## Tarea típica: revisar o corregir datos nutricionales
1. Editar **solo** `data/alimentos.csv`. Columnas:
   `name,kcal100,protein100,fat100,carbs100,fiber100,source,source_detail`
2. Valores por 100 g. Celda vacía = sin dato (válido; no poner 0 inventado).
3. `source`: `verificado` (fuente oficial) | `etiqueta` (envase) | `manual` (estimación).
   `source_detail`: qué fuente + fecha.
4. Validar: `npx tsx scripts/load-csv.mts` (requiere `DATABASE_URL`) y
   `npx tsx scripts/check-nutrition.mts` desde `web/`.
5. Abrir PR a `main` describiendo qué cambió y por qué. No pushear directo a `main`.

## Reglas duras
- No inventar valores nutricionales. Lo dudoso se marca, no se acomoda en silencio.
- No commitear `.env`, secretos ni datos personales de usuarios
  (emails, pesos, objetivos, horarios). Los loaders `load-*.mts` viven solo en
  local y están gitignoreados.
- No tocar autenticación (`src/auth.ts`, `src/app/api/auth/*`), proxy ni
  infraestructura sin coordinar con el dueño del repo.
- Commits cortos, en español. Un cambio = un commit.
- Antes de dar algo por terminado: `npx tsc --noEmit` en `web/` y
  `npx tsx scripts/itest.mts` contra la app corriendo.

## Correr en local
1. `cp env.example .env` y completar (ver el archivo).
2. `docker compose up -d db` (Postgres en `localhost:5432`).
3. En `web/`: crear `.env` con `DATABASE_URL`, `NEXTAUTH_SECRET`
   (`openssl rand -base64 32`) y `NEXTAUTH_URL=http://localhost:3000`.
4. `npm install`, `npm run dev`, abrir `http://localhost:3000`.
5. Seeds: `npx tsx scripts/seed.mts` y `npx tsx scripts/seed-recetas.mts`
   (requieren `DATABASE_URL` en el entorno).
6. Migraciones: `npx drizzle-kit generate` + aplicar el SQL en `web/drizzle/`.
