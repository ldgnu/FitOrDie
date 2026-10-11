# FitOrDie — Fase 1: Diagnóstico (2026-10-11)

## Qué existe hoy

**Perfil `cambio-fisico` en Hermes (VPS propio), gateway activo.**
No es un bot independiente: corre sobre el bot principal de Telegram,
atiende en un topic privado.

- **Rol actual**: coach conversacional (rutinas, dieta orientativa, hábitos,
  seguimiento por chat). Tono motivador, cero juzgamiento explícito.
- **Almacenamiento**: memoria del perfil (`state.db`, sessions, vault Obsidian).
  Sin base de datos estructurada de comidas ni nutrientes.
- **Capacidades**: charla, crons (`jobs.json`), skills genéricas. Sin API,
  sin webhooks propios, sin idempotencia, sin cálculos deterministas.
- **Local** (`~/Proyectos/`): no hay código del bot. Solo BRIEF + README.

## Implicancias para FitOrDie

1. **La integración Telegram se construye del lado web.** El perfil no tiene
   API que consumir: FitOrDie expone API autenticada e idempotente, y el
   perfil la usa como cliente (o vía un bridge mínimo en el VPS, fase 8).
2. **FitOrDie backend = fuente de verdad.** Hoy los datos viven en charlas;
   el diario estructurado (Postgres) pasa a ser el registro canónico.
3. **No tocar el gateway en producción.** Es frágil (multiplex, bug same-token
   ya parcheado a mano) y el restart está bloqueado desde dentro. Solo lectura
   hasta fase 8, y cualquier cambio con backup + ventana acordada.
4. **Conflicto de tono a resolver**: el perfil actual es motivador puro
   ("si una semana falló, se retoma, no se castiga"). FitOrDie v2.1 define la
   voz Testigo (reta conducta). Recomendación: no cambiar el perfil; la web
   tiene voz propia. Desacoplado por diseño.

## Riesgos señalados

- Estimaciones nutricionales "a ojo" del perfil no son datos: al migrar a
  FitOrDie, solo entra lo re-confirmado por el usuario, nada automático.
- El token del bot vive en el VPS: la web jamás lo toca ni lo replica.
- Vinculación Telegram↔web requiere flujo seguro (fase 4/8), no IDs confiados.

## Plan (resumen)

1. Git init + GitHub.
2. Stack: Next.js + TS + Postgres en Docker, PWA mobile-first.
3. MVP: auth + diario + motor nutricional + dashboard/Testigo.
4. Después: recetario, físico, y al final Telegram (fase 8).

## Estado

Fase 1 completa (solo lectura, sin cambios en producción).
