# FitOrDie — Front-end (diseño + prototipos)

Esta carpeta es **el sistema de diseño y los prototipos estáticos** de la app.
No tiene build, no tiene Node, no tiene dependencias: HTML + CSS + JS vanilla.
El backend Go (SRE) consume estos archivos; los prototipos son la referencia
visual y funcional de las 5 pantallas.

Estado: **prototipo funcional con datos mock**. La única lógica real es el
cálculo de macros por gramos y el parser local de texto libre — el resto
(guardar, borrar, sesión) lo implementa Go.

---

## 1. Cómo verlo

```bash
cd app/design
python3 -m http.server 8099
# abrir http://127.0.0.1:8099/
```

También abre con doble clic (`file://`), con una excepción: el service worker
no corre en `file://` (no hay offline ni instalación, el resto anda igual).

| Archivo | Pantalla | Ruta sugerida en Go |
|---|---|---|
| `index.html` | Hoy (dashboard) | `/` |
| `comidas.html` | Comidas (carga + IA) | `/comidas` |
| `rutina.html` | Rutina A/B/C | `/rutina` |
| `progreso.html` | Progreso (peso, tendencia, adherencia) | `/progreso` |
| `perfil.html` | Perfil y objetivos | `/perfil` |

Las capturas de referencia están en `screenshots/` (mobile 390×844 y desktop
1280×800, tema oscuro).

---

## 2. Estructura

```
app/design/
├── index.html · comidas.html · rutina.html · progreso.html · perfil.html
├── css/
│   ├── app.css          ← único archivo a enlazar (importa los otros 3)
│   ├── tokens.css       ← variables: color, tipografía, espaciado, radios, sombras
│   ├── base.css         ← reset, tipografía, layout, utilidades, responsive
│   └── components.css   ← botones, inputs, cards, nav, sheets, gráficos, etc.
├── js/demo.js           ← SOLO prototipo (ver §6). No es el JS de producción.
├── manifest.webmanifest
├── sw.js
├── icons/               ← 192, 512, maskable 512, apple 180, favicon 32, svg
├── screenshots/
└── tools/
    ├── build-pages.py   ← genera los 5 HTML (shell compartido + datos del CSV)
    └── make-icons.py    ← genera los PNG de iconos (Pillow)
```

**Los `.html` son generados.** El shell (head, sprite de iconos, bottom nav,
rail de desktop, toast) vive una sola vez en `tools/build-pages.py`. Si tocás
el HTML a mano y después regenerás, perdés el cambio. Editá el generador y
corré:

```bash
python3 tools/build-pages.py     # HTML + manifest
python3 tools/make-icons.py      # iconos PNG (requiere Pillow)
```

El generador lee `data/alimentos.csv` para los valores nutricionales y calcula
los totales del día demo. No hay números hardcodeados a mano en las pantallas.

---

## 3. Sistema de diseño

### 3.1 Color

Dos capas: paleta cruda (`--p-*`) y tokens semánticos (`--bg`, `--surface`,
`--accent`…). **Usá los semánticos**; la paleta solo se toca en `tokens.css`.

Tema: `<html data-theme="dark|light">`. Sin atributo, manda el sistema
(`prefers-color-scheme`). El script inline del `<head>` aplica el tema guardado
antes del primer paint, así no hay flash.

| Token | Uso |
|---|---|
| `--bg`, `--bg-2` | fondo de app / fondo elevado (sheets, nav) |
| `--surface`, `--surface-2`, `--surface-3` | cards, inputs, tracks de barras |
| `--line`, `--line-strong` | bordes 1px |
| `--text`, `--text-dim`, `--text-mute` | jerarquía de texto (3 niveles, no más) |
| `--accent`, `--accent-strong`, `--accent-text` | verde de marca. `--accent` = relleno (botón), `--accent-text` = texto sobre fondo teñido |
| `--protein`, `--carbs`, `--fat` | macros: azul / ámbar / violeta |
| `--ok`, `--warn`, `--danger` | estados |

Contraste verificado (WCAG AA, texto normal 4.5:1, grande 3:1) en los dos
temas, todas las pantallas: **0 fallas**. Si cambiás un color, volvé a medir.

### 3.2 Tipografía

Stack del sistema (sin webfonts: cero requests, cero FOUT, anda offline).

`--fs-xs` 12 · `--fs-sm` 13 · `--fs-base` 15 · `--fs-md` 16 (inputs) ·
`--fs-lg` 18 · `--fs-xl` 22 · `--fs-2xl` 28 · `--fs-3xl` 34 · `--fs-display` 42.

Reglas: inputs siempre 16px (evita el auto-zoom de iOS); números con
`font-variant-numeric: tabular-nums` (clase `.num` o `.tnum`); títulos con
`letter-spacing: -0.02em`; nada de texto por debajo de 11px.

### 3.3 Espaciado y formas

Escala de 4px: `--sp-1` 4 · `--sp-2` 8 · `--sp-3` 12 · `--sp-4` 16 · `--sp-5` 20 ·
`--sp-6` 24 · `--sp-7` 32 · `--sp-8` 40 · `--sp-9` 48 · `--sp-10` 64.

Radios: `--r-xs` 8 · `--r-sm` 10 · `--r-md` 14 · `--r-lg` 18 · `--r-xl` 22 ·
`--r-2xl` 28 · `--r-pill`. Cards usan `--r-xl`; inputs y botones internos `--r-md`;
botones principales y chips, pill.

### 3.4 Layout

Mobile-first, una columna, gutter de 16px (20px en desktop).

- `< 900px`: **bottom nav** fija de 5 ítems (64px + safe area iOS).
- `≥ 900px`: la bottom nav se oculta y aparece un **rail lateral** de 240px;
  el contenido pasa a grilla de 2 columnas (`.grid-page`).
- `≥ 1000px`: la rutina muestra los ejercicios en 2 columnas.
- Ancho máximo de contenido: 58rem. Breakpoint chico extra en 380px (ring y
  títulos más compactos).

### 3.5 Touch, foco y movimiento

- Todo control interactivo mide **≥ 44×44 px** (medido con `getBoundingClientRect`
  en 320/390/768/1280: 0 excepciones). Los switches de 28px de alto van dentro
  de una fila `<label>` de 56px, así que el target real es la fila entera.
- Foco visible con `:focus-visible` (anillo de 2px con offset).
- `prefers-reduced-motion: reduce` desactiva transiciones y scroll suave.
- Áreas seguras iOS: `env(safe-area-inset-bottom)` en nav, sheets y toasts.

### 3.6 Componentes disponibles

Botones (`.btn` + `--primary/--secondary/--outline/--ghost/--soft/--danger`,
`--lg/--sm/--block/--icon`), `.iconbtn`, `.chip`, `.badge` (+`--accent/--ok/--warn/--danger`),
`.tag`, `.card` (+`--hero/--tight/--flat/--pad0`), `.bar`, `.ring`, `.macro`,
`.stats/.stat`, `.meal`, `.food`, `.entry`, `.hist`, `.ex` (tarjeta de ejercicio),
`.series`, `.daytab`, `.segmented`, `.switch`, `.stepper`, `.input/.field/.label/.hint`,
`.sheet`, `.toast`, `.callout` (+`--accent/--warn/--danger`), `.empty`, `.kv`,
`.setting`, `.daybar`, `.bottomnav`, `.railnav`, `.pagehead`, `.chips`, `.legend`, `.chart`.

Iconos: sprite SVG inline (`<svg class="i"><use href="#i-plus"/></svg>`). Nombres
en `tools/build-pages.py` (`ICONS`). Sin icon fonts, sin librerías.

### 3.7 Voz y microcopy

Español rioplatense, voseo, frases cortas, sin corporate: «Cargá», «Elegí»,
«Anotá», «Sumá», «Registrá series». Nunca culpar al usuario («Te pasaste» →
mostrar el número y listo). Si falta un dato: **«sin datos»**, nunca un cero
inventado.

---

## 4. Datos que espera cada pantalla

Propuesta de contrato para alinear con el backend (`snake_case` en JSON,
nombres de template en `camelCase`). Si SRE ya definió otra forma, se adapta el
template: lo que **no** se negocia es que el cálculo salga del servidor y que
el "sin datos" sea explícito.

### 4.1 Comunes

```
Date            string  "2026-10-22"            (zona America/Argentina/Buenos_Aires)
Targets         {kcal_min 2150, kcal_max 2400, protein_min 155, protein_max 170,
                 carbs_g 190, fat_g 155}
Macros          {kcal float, protein_g, carbs_g, fat_g, fiber_g}
```

### 4.2 Hoy

```
PageHoy {
  date            string
  streak_days     int
  consumed        Macros
  targets         Targets
  remaining_kcal  int            // targets.kcal_min - consumed.kcal (puede ser negativo)
  meals           []MealSlot     // 6: desayuno, media, almuerzo, merienda, cena, picoteo
  weight          {current_kg, delta_7d_kg, goal_min_kg, goal_max_kg,
                   series []{date, kg}}   // serie para el sparkline (últimos 14)
  suggestion      {title, detail, kcal, protein_g} | null   // cena sugerida del plan
  quick_foods     []FoodRef      // chips de carga rápida
}
MealSlot {id, name, time, entries []Entry, totals Macros, planned_note string|null}
```

Mapeo visual: `consumed.kcal` → anillo (`--pct = consumed/targets.kcal_min`),
`remaining_kcal` → número grande, `consumed.protein_g` → barra azul, `meals[i].totals`
→ columna derecha de cada fila, `weight.series` → sparkline.

### 4.3 Comidas

```
PageComidas {
  date      string
  targets   Targets
  meals     []MealSlot        // para los chips y los totales por comida
  foods     []Food            // tabla completa (búsqueda client-side o /api/foods?q=)
  entries   []Entry           // lo cargado hoy
  totals    Macros
}
Food  {id, name, kcal100, protein100, carbs100, fat100, fiber100,
       sugar100 *float, sodium_mg100 *float, source string,
       measures []Measure}          // medidas caseras: [{unit:"unidad", grams:50}]
Entry {id, meal_id, food_id *string, name, qty, unit, grams,
       kcal, protein_g, carbs_g, fat_g, fiber_g, estimated bool}
```

Reglas de cálculo (las mismas del prototipo, tienen que coincidir con Go):

```
factor = grams / 100
kcal = round(food.kcal100 * factor)      // se guarda el valor ya calculado
protein_g = food.protein100 * factor     // 1 decimal
```

El **entry guarda los macros calculados**, no solo los gramos: si mañana se
corrige el CSV, lo ya cargado no cambia solo. (Decisión para charlar con SRE;
el prototipo asume esto.)

Estimación por texto/foto:

```
POST /api/parse {text, meal_id, photo?} -> {
  source: "local" | "llm",
  items:   []{name, food_id *string, grams, qty, unit, kcal, protein_g, carbs_g, fat_g, confidence},
  unmatched: []string        // lo que no está en la tabla: NO se inventa
}
```

`unmatched` no vacío → la UI muestra el callout ámbar («No están en la tabla: …»).
Si `items` viene vacío, el botón «Sumar todo» no debe hacer nada.

### 4.4 Rutina

```
PageRutina {
  mesocycle  {id, title, days_per_week}
  days       []RoutineDay
  cardio     []string
  medical    []{label, status}     // ej. eco: pending
}
RoutineDay {key "A"|"B"|"C", title, focus, exercises []Exercise}
Exercise   {id, name, sets int, reps_min, reps_max, rir, rest_seconds int,
            notes, media_url *string, video_url, log []SetLog, done bool}
SetLog     {set int, kg *float, reps *int}
```

`video_url` en el prototipo es una **búsqueda de YouTube** armada con el nombre
(`https://www.youtube.com/results?search_query=<nombre>+tecnica+correcta`), sin
API key. Cuando haya GIF/imagen propia, se usa `media_url` y el botón «Ver
video» queda como respaldo.

Progresión (texto en la UI, la decide el plan): subir reps 10→12 y después
+5 % de peso. RIR 3-4, nunca al fallo.

### 4.5 Progreso

```
PageProgreso {
  current_kg, start_kg, goal_min_kg, goal_max_kg
  delta_7d_kg, avg_7d_kg, rate_kg_week
  series     []{date, kg}                    // 30 días para el gráfico
  adherence  {days_logged, days_total, pct}
  streak_days int
  history    []{date, kg, delta_kg}
}
```

Gráfico: SVG server-side (el prototipo lo genera con `line_chart()` en
`tools/build-pages.py`). Sin librerías de charting.

### 4.6 Perfil

```
PagePerfil {
  profile {name, age, height_cm, weight_kg, timezone, activity,
           goal_min_kg, goal_max_kg, kcal_min, kcal_max,
           protein_min, protein_max, tmb_kcal}
  prefs   {theme "dark"|"light"|"system", pin_enabled bool, offline bool}
}
```

---

## 5. Integración con Go

### 5.1 Servir los estáticos

```go
//go:embed design/css design/js design/icons design/manifest.webmanifest design/sw.js
var staticFS embed.FS
```

```go
mux.Handle("GET /css/", http.FileServer(http.FS(cssFS)))
mux.Handle("GET /js/",  http.FileServer(http.FS(jsFS)))
mux.Handle("GET /icons/", http.FileServer(http.FS(iconsFS)))
mux.HandleFunc("GET /manifest.webmanifest", manifestHandler)
mux.HandleFunc("GET /sw.js", swHandler)
```

`sw.js` **tiene que** servirse desde la raíz (`/sw.js`), no desde `/js/sw.js`:
el scope del service worker es su propia carpeta, y desde `/js/` no controlaría
las páginas. Lo mismo el `manifest` (`start_url` y `scope` relativos al
manifiesto).

Headers recomendados:

```
Cache-Control: public, max-age=31536000, immutable   → /icons/* (no cambian de nombre: usar max-age=86400)
Cache-Control: no-cache                              → /sw.js (si no, no se actualiza nunca)
Cache-Control: no-store                              → HTML y /api/*
```

Si cambiás CSS/JS, subí `VERSION` en `sw.js` (borra cachés viejas en `activate`).

### 5.2 Templates

`html/template` con un layout y un bloque por pantalla. El shell del prototipo
(head, sprite, navs, toast) es el partial a extraer:

```
app/internal/web/templates/
├── layout.html        ← {{define "layout"}} … {{template "content" .}} …{{end}}
├── partials/nav.html  ← bottom nav + rail (activo según .Page)
├── partials/icons.html← sprite SVG (copiar de tools/build-pages.py)
├── partials/macros.html
├── hoy.html           ← {{define "content"}}
├── comidas.html
├── rutina.html
├── progreso.html
└── perfil.html
```

Ejemplo mínimo:

```go
// partials/macros.html
{{define "macro"}}
<div class="macro">
  <div class="macro__head">
    <span class="macro__name"><i class="dot" style="--c:var(--{{.Key}})"></i>{{.Label}}</span>
    <span class="macro__val">{{fmt1 .Value}} / <span class="macro__target">{{fmt0 .Target}} g</span></span>
  </div>
  <div class="bar" style="--v:{{pct .Value .Target}}%"><i style="--bar:var(--{{.Key}})"></i></div>
</div>
{{end}}
```

Helpers a registrar en el `FuncMap` (formato argentino, sin depender del locale
del server): `fmt0` → `1.530`, `fmt1` → `89,7`, `fmt2` → `89,65`, `pct` →
`76.2` (número, sin `%`), `dateShort` → `22/10`, `dateLong` → `jueves 22 de octubre`.

Ojo con `style="--v:{{...}}%"`: `html/template` escapa CSS dentro de `style`
y puede romper el `calc`; si pasa, calculá el string en Go y pasalo con
`template.CSS` (con valores numéricos propios, nunca con input del usuario).

### 5.3 Interacción: qué reemplaza a `demo.js`

| Prototipo (demo.js) | Producción |
|---|---|
| Búsqueda y cálculo en vivo | HTMX: `hx-get="/api/foods?q="` `hx-trigger="input changed delay:250ms"`; o JS vanilla contra el mismo endpoint |
| Sumar entrada | `hx-post="/api/entries"` → devuelve el bloque de la comida + totales (`hx-target`, `hx-swap="outerHTML"`) |
| Borrar entrada | `hx-delete="/api/entries/{id}"` con confirmación |
| Estimar con IA | `hx-post="/api/parse"`, muestra items + `unmatched` |
| Marcar ejercicio / series | `hx-post="/api/routine/{id}/log"` |
| Guardar peso | `hx-post="/api/weights"` |
| Cambiar tema | `localStorage` + `data-theme` (queda client-side, es cosmético) |

Regla: **toda mutación vuelve del servidor con los totales recalculados**. El
cliente no suma macros por su cuenta en producción (el prototipo sí, para
demostrar el flujo).

### 5.4 PWA

- `manifest.webmanifest`: nombre, `start_url "./index.html"` (en Go, `"/"`),
  `display: standalone`, `theme_color #0a0f0e`, 4 iconos (incluye maskable),
  2 shortcuts (Comidas, Rutina).
- `sw.js`: navegación **red primero** (para no servir el día de ayer como si
  fuera hoy), estáticos red primero con caché de respaldo, `/api/*` **sin caché**
  y error 503 explícito si no hay red. `install` precachea el shell.
- registrar el SW solo por HTTPS o localhost (Tailscale con `https` o
  `http://100.x` no habilita SW en iOS; ver `SECURITY.md` §6).

---

## 6. Qué NO usar del prototipo

`js/demo.js` existe para que el prototipo se pueda tocar y para documentar el
flujo. **No copiarlo a producción**: usa `localStorage`, re-render con
`innerHTML` y no valida nada. En Go:

- validar todo en el servidor (gramos, fechas, ids, rangos),
- escapar datos con `html/template` (nunca `template.HTML` con input de usuario),
- no confiar en los totales que mande el cliente.

Detalle completo en [`SECURITY.md`](./SECURITY.md).

---

## 7. Verificación hecha sobre el prototipo

Medido en Chromium con viewports 320×568, 390×844, 768×1024, 1280×800:

- **Sin scroll horizontal** en las 5 pantallas (todos los anchos).
- **Touch targets**: 0 controles por debajo de 44×44 (switches dentro de fila
  `<label>` de 56px).
- **Contraste AA**: 0 fallas en claro y oscuro (≈80–105 nodos de texto por
  pantalla, midiendo color efectivo contra el fondo efectivo).
- **Nada tapado** por la bottom nav ni por la barra de totales al llegar al
  final del scroll (23–46px de aire).
- **PWA**: SW se registra y activa, 8 entradas en caché; con red apagada las
  pantallas cacheadas siguen abriendo.
- **Flujos probados**: buscar → elegir → cambiar gramos → cambiar medida
  (conversión g↔unidad) → sumar → totales y barra del día; estimar por texto
  (con items y con «sin datos»); marcar ejercicio y ver progreso; guardar peso
  (y rechazo de valor inválido); cambio de tema.
- HTML sin errores de anidado (parser estricto sobre los 5 archivos).

Pendientes conocidos: imagen/GIF real de ejercicios, gráficos con datos reales,
foto→IA (requiere `LLM_BASE_URL`), y el PIN (se implementa en Go).

---

## 8. Checklist de integración (para cuando SRE cierre)

- [ ] `/css/app.css` responde con los 4 archivos accesibles (o concatenados en 1).
- [ ] `/sw.js` en la raíz, `manifest` con los iconos en `/icons/`.
- [ ] Los 5 templates renderizan con datos reales y helpers `fmt0/fmt1/pct`.
- [ ] Totales del servidor coinciden con el cálculo del prototipo (±1 kcal por redondeo).
- [ ] `unmatched` y «sin datos» visibles cuando corresponde (no ceros inventados).
- [ ] `/api/*` responde 401 cuando el PIN no está validado.
- [ ] Medir de nuevo contraste y touch targets con datos reales (nombres largos
      de alimentos son el caso borde: el prototipo usa `.truncate`).
