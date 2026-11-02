#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Genera los prototipos estáticos de FitOrDie (app/design/*.html).

Por qué un generador y no 5 archivos a mano: el shell (head, sprite de iconos,
bottom nav, rail de desktop, toast) es idéntico en las 5 pantallas. Acá vive una
sola vez. El HTML que sale es estático, sin dependencias: ábranlo con doble clic
o sirvan la carpeta con `python3 -m http.server`.

Los números de las pantallas (totales del día, macros, tendencia de peso) se
calculan desde data/alimentos.csv y desde los mismos datos seed que usa el
prototipo. Nada está inventado a mano: si cambia el CSV, se regenera.

Uso:  python3 tools/build-pages.py
"""

import datetime
import json
import math
import os
import re
import unicodedata
from urllib.parse import quote_plus

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REPO = os.path.dirname(os.path.dirname(ROOT))  # /home/ubuntu/self-hosted/FitOrDie
CSV_PATH = os.path.join(REPO, "data", "alimentos.csv")
OUT = ROOT

TARGET = {"kcal": 2150, "kcal_max": 2400, "p": 155, "p_max": 170, "c": 190, "f": 155}
GOAL = (84.0, 85.0)
START_WEIGHT = 96.0

# ---------------------------------------------------------------- utilidades


def esc(s):
    return (
        str(s)
        .replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


def fold(s):
    s = unicodedata.normalize("NFD", str(s).lower())
    return "".join(c for c in s if unicodedata.category(c) != "Mn").strip()


def slug(s):
    s = fold(s)
    s = re.sub(r"[^a-z0-9]+", "-", s).strip("-")
    return s


def nf(n, d=0):
    """Formato es-AR: 1.530 / 89,65"""
    if n is None:
        return "—"
    s = f"{n:,.{d}f}"
    return s.replace(",", "§").replace(".", ",").replace("§", ".")


def r0(n):
    return int(round(n))


def r1(n):
    return round(n + 0.0, 1)


# ---------------------------------------------------------------- datos CSV


def load_foods():
    foods = []
    with open(CSV_PATH, encoding="utf-8") as fh:
        rows = fh.read().splitlines()
    for line in rows[1:]:
        if not line.strip():
            continue
        # El CSV tiene comillas mal balanceadas en source_detail: separamos por
        # el patrón exacto y limpiamos comillas sobrantes de cada campo.
        parts = line.split('","')
        parts = [p.strip().strip('"') for p in parts]
        if len(parts) < 6:
            continue
        name = parts[0]
        try:
            kcal = float(parts[1] or 0)
            p = float(parts[2] or 0)
            fat = float(parts[3] or 0)
            c = float(parts[4] or 0)
            fiber = float(parts[5] or 0)
        except ValueError:
            continue
        foods.append(
            {
                "id": slug(name),
                "name": name,
                "kcal": r0(kcal) if kcal == int(kcal) else r1(kcal),
                "p": p,
                "c": c,
                "fat": fat,
                "fiber": fiber,
            }
        )
    return foods


FOODS = load_foods()
BY_NAME = {f["name"]: f for f in FOODS}


def macros(name, grams):
    f = BY_NAME[name]
    k = grams / 100.0
    return {
        "name": name,
        "foodId": f["id"],
        "grams": r1(grams),
        "kcal": r1(f["kcal"] * k),
        "p": r1(f["p"] * k),
        "c": r1(f["c"] * k),
        "f": r1(f["fat"] * k),
        "fiber": r1(f["fiber"] * k),
    }


# Día de demo: 4 comidas cargadas, cena pendiente (así se ve el CTA)
SEED_RAW = [
    ("desayuno", "Huevo", 150, "unidad", 3),
    ("desayuno", "Queso cremoso", 40, "g", 40),
    ("desayuno", "Café negro", 200, "taza", 1),
    ("media", "Yogur griego sin azúcar", 170, "pote", 1),
    ("media", "Nueces", 15, "g", 15),
    ("almuerzo", "Pollo pechuga", 200, "g", 200),
    ("almuerzo", "Arroz blanco cocido", 190, "g", 190),
    ("almuerzo", "Lechuga", 80, "g", 80),
    ("almuerzo", "Aceite de oliva", 10, "cucharada", 1),
    ("merienda", "Banana", 120, "unidad", 1),
    ("merienda", "Yogur griego sin azúcar", 170, "pote", 1),
]

SEED = []
for i, (slot, name, grams, unit, qty) in enumerate(SEED_RAW):
    m = macros(name, grams)
    m.update({"id": f"seed{i}", "slot": slot, "unit": unit, "qty": qty})
    SEED.append(m)

TOT = {"kcal": 0, "p": 0, "c": 0, "f": 0, "fiber": 0}
for e in SEED:
    for k in TOT:
        TOT[k] += e[k]

SLOTS = [
    ("desayuno", "Desayuno", "7:30"),
    ("media", "Media mañana", "11:00"),
    ("almuerzo", "Almuerzo", "13:30"),
    ("merienda", "Merienda", "16:30"),
    ("cena", "Cena", "21:00"),
    ("picoteo", "Picoteo", "—"),
]
SLOT_NAME = {s[0]: s[1] for s in SLOTS}


def slot_totals(slot):
    t = {"kcal": 0, "p": 0}
    for e in SEED:
        if e["slot"] == slot:
            t["kcal"] += e["kcal"]
            t["p"] += e["p"]
    return t


# Cena sugerida (Plan Fran C) calculada con la tabla, no a mano
CENA_C = {"kcal": 0, "p": 0}
for name, g in [("Huevo", 150), ("Zapallitos", 200), ("Cebolla", 50), ("Aceite de oliva", 10)]:
    m = macros(name, g)
    CENA_C["kcal"] += m["kcal"]
    CENA_C["p"] += m["p"]

# ---------------------------------------------------------------- progreso

CURRENT_KG = 89.65
PREV7_KG = 89.90

# 30 días de historial (mock determinista, tendencia a la baja con mesetas)
WEIGHT_OFFSETS = [
    1.55, 1.50, 1.38, 1.42, 1.30, 1.22, 1.26, 1.14, 1.05, 0.98,
    1.02, 0.88, 0.80, 0.72, 0.76, 0.64, 0.55, 0.48, 0.52, 0.40,
    0.32, 0.24, 0.28, 0.16, 0.10, 0.04, 0.08, -0.02, 0.01, 0.0,
]
TODAY = datetime.date(2026, 10, 22)
WEIGHTS = []
for i, off in enumerate(WEIGHT_OFFSETS):
    day = TODAY - datetime.timedelta(days=len(WEIGHT_OFFSETS) - 1 - i)
    WEIGHTS.append({"date": day.isoformat(), "kg": round(CURRENT_KG + off, 2)})
WEIGHTS[-1]["kg"] = CURRENT_KG

WEEK_AGO = [w for w in WEIGHTS if w["date"] == (TODAY - datetime.timedelta(days=7)).isoformat()][0]


SEED_JSON = json.dumps(
    {"entries": SEED, "weights": [{"date": w["date"], "kg": w["kg"]} for w in WEIGHTS]},
    ensure_ascii=False,
    separators=(",", ":"),
)


def short_date(iso, with_year=False):
    y, m, d = iso.split("-")
    return f"{d}/{m}" if not with_year else f"{d}/{m}/{y}"


def human_date(iso):
    names = ["lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo"]
    months = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
              "agosto", "septiembre", "octubre", "noviembre", "diciembre"]
    dt = datetime.date.fromisoformat(iso)
    return f"{names[dt.weekday()]} {dt.day} de {months[dt.month - 1]}"


# ---------------------------------------------------------------- gráficos SVG


def line_chart(points, gid, w=340, h=150, padl=34, padr=10, padt=12, padb=20, dots=True):
    """points: [(iso_date, kg)] -> SVG (viewBox w x h) con área + línea."""
    ys = [p[1] for p in points]
    ymin = math.floor((min(ys) - 0.45) * 2) / 2
    ymax = math.ceil((max(ys) + 0.35) * 2) / 2
    if ymax - ymin < 1:
        ymax = ymin + 1
    n = len(points)
    inner_w = w - padl - padr
    inner_h = h - padt - padb

    def px(i):
        return padl + inner_w * (i / (n - 1)) if n > 1 else padl + inner_w / 2

    def py(v):
        return padt + (ymax - v) / (ymax - ymin) * inner_h

    coords = [(px(i), py(p[1])) for i, p in enumerate(points)]

    grid = ""
    ticks = 3
    for t in range(ticks):
        v = ymax - (ymax - ymin) * t / (ticks - 1)
        y = py(v)
        grid += f'<line class="chart__grid" x1="{padl}" y1="{y:.1f}" x2="{w - padr}" y2="{y:.1f}" />'
        grid += (
            f'<text class="chart__label" x="{padl - 6}" y="{y + 3:.1f}" '
            f'text-anchor="end">{nf(v, 1)}</text>'
        )

    path = "M " + " L ".join(f"{x:.1f} {y:.1f}" for x, y in coords)
    area = (
        f'{path} L {coords[-1][0]:.1f} {h - padb:.1f} L {coords[0][0]:.1f} {h - padb:.1f} Z'
    )

    dot_html = ""
    if dots:
        for i, (x, y) in enumerate(coords):
            if i % 4 == 0 or i == n - 1:
                rad = 3.2 if i == n - 1 else 2.1
                dot_html += (
                    f'<circle class="chart__dot" cx="{x:.1f}" cy="{y:.1f}" r="{rad}" />'
                )

    # etiquetas X: primera, del medio, última
    xlabels = ""
    for i in (0, n // 2, n - 1):
        anchor = "start" if i == 0 else ("end" if i == n - 1 else "middle")
        cls = "chart__label chart__label--strong" if i == n - 1 else "chart__label"
        xlabels += (
            f'<text class="{cls}" x="{coords[i][0]:.1f}" y="{h - 5}" '
            f'text-anchor="{anchor}">{short_date(points[i][0])}</text>'
        )

    last = coords[-1]
    return f"""<svg class="chart" viewBox="0 0 {w} {h}" role="img"
        aria-label="Evolución del peso: de {nf(points[0][1], 1)} kg el {short_date(points[0][0])} a {nf(points[-1][1], 2)} kg hoy">
  <defs>
    <linearGradient id="{gid}" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stop-color="var(--accent)" stop-opacity="0.28" />
      <stop offset="100%" stop-color="var(--accent)" stop-opacity="0" />
    </linearGradient>
  </defs>
  {grid}
  <path d="{area}" fill="url(#{gid})" />
  <path class="chart__line" d="{path}" />
  {dot_html}
  {xlabels}
  <text class="chart__label chart__label--strong" x="{last[0] - 4:.1f}" y="{last[1] - 9:.1f}"
        text-anchor="end">{nf(points[-1][1], 2)}</text>
</svg>"""


def sparkline(values, w=120, h=40):
    lo, hi = min(values), max(values)
    if hi - lo < 0.2:
        hi = lo + 0.2
    n = len(values)
    pts = []
    for i, v in enumerate(values):
        x = 3 + (w - 8) * i / (n - 1)
        y = 4 + (h - 10) * (hi - v) / (hi - lo)
        pts.append((x, y))
    path = "M " + " L ".join(f"{x:.1f} {y:.1f}" for x, y in pts)
    area = f'{path} L {pts[-1][0]:.1f} {h} L {pts[0][0]:.1f} {h} Z'
    return f"""<svg viewBox="0 0 {w} {h}" class="chart" aria-hidden="true">
  <defs><linearGradient id="sparkFill" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0%" stop-color="var(--accent)" stop-opacity="0.3" />
    <stop offset="100%" stop-color="var(--accent)" stop-opacity="0" />
  </linearGradient></defs>
  <path d="{area}" fill="url(#sparkFill)" />
  <path class="chart__line" d="{path}" style="stroke-width:2" />
  <circle class="chart__dot" cx="{pts[-1][0]:.1f}" cy="{pts[-1][1]:.1f}" r="2.8" />
</svg>"""


# ---------------------------------------------------------------- sprite SVG

ICONS = {
    "home": '<path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z"/>',
    "utensils": '<path d="M6 3v6a2.5 2.5 0 0 0 5 0V3M8.5 11.5V21M17 3v18M17 3c2.2 1.2 3.2 3.2 3.2 5.6 0 2.3-1 4.1-3.2 4.4"/>',
    "dumbbell": '<path d="M6.5 6.5v11M17.5 6.5v11M3.5 9.5v5M20.5 9.5v5M6.5 12h11"/>',
    "chart": '<path d="M4 4v16h16M8 16l3.5-4 3 2.5L20 8"/>',
    "user": '<path d="M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4.5 21c1.2-3.6 4-5.5 7.5-5.5s6.3 1.9 7.5 5.5"/>',
    "plus": '<path d="M12 5v14M5 12h14"/>',
    "search": '<circle cx="11" cy="11" r="7"/><path d="M16.2 16.2 21 21"/>',
    "sun": '<path d="M12 4V2M12 22v-2M4 12H2M22 12h-2M5.6 5.6 4.2 4.2M19.8 19.8l-1.4-1.4M18.4 5.6l1.4-1.4M4.2 19.8l1.4-1.4"/><circle cx="12" cy="12" r="4.5"/>',
    "moon": '<path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"/>',
    "chevron": '<path d="M9 6l6 6-6 6"/>',
    "chevron-down": '<path d="M6 9l6 6 6-6"/>',
    "play": '<path d="M8 5.5v13l11-6.5z" fill="currentColor" stroke="none"/>',
    "check": '<path d="M5 13l4.5 4.5L19 7"/>',
    "trash": '<path d="M4 7h16M9.5 7V4h5v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/>',
    "sparkles": '<path d="M12 3l1.8 4.2L18 9l-4.2 1.8L12 15l-1.8-4.2L6 9l4.2-1.8zM18 15.5l.8 1.9 1.9.8-1.9.8-.8 1.9-.8-1.9-1.9-.8 1.9-.8z"/>',
    "alert": '<path d="M12 4.5 21 20H3zM12 10v4.5M12 17.4h.01"/>',
    "info": '<circle cx="12" cy="12" r="9"/><path d="M12 11v5.5M12 8h.01"/>',
    "camera": '<path d="M4 8h3l1.5-2h7L17 8h3v11H4z"/><circle cx="12" cy="13.5" r="3.4"/>',
    "scale": '<path d="M4 7h16M12 3v4M7.5 7 4 15h7zM16.5 7 13 15h7z"/>',
    "flame": '<path d="M12 3c3 4 5 5.5 5 9a5 5 0 0 1-10 0c0-2 .8-3.4 2-5 .4 1.5 1 2 2 2 .5-2 .5-4 1-6z"/>',
    "lock": '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M9 11V8a3 3 0 0 1 6 0v3"/>',
    "download": '<path d="M12 4v10M8 11l4 4 4-4M5 20h14"/>',
    "image": '<rect x="3.5" y="5" width="17" height="14" rx="2.5"/><path d="M3.5 15.5l4.2-4 3.1 3 3-2.2 6.2 6.2"/><circle cx="9" cy="9" r="1.3"/>',
    "clock": '<circle cx="12" cy="12" r="9"/><path d="M12 7.5V12l3 1.8"/>',
    "x": '<path d="M6 6l12 12M18 6 6 18"/>',
    "target": '<circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="4.5"/><circle cx="12" cy="12" r="1" fill="currentColor"/>',
    "youtube": '<rect x="2.5" y="5.5" width="19" height="13" rx="3.5"/><path d="M10.5 9.5l5 2.5-5 2.5z" fill="currentColor" stroke="none"/>',
    "list": '<path d="M4 6.5h16M4 12h16M4 17.5h10"/>',
    "trend": '<path d="M4 7l5.5 5.5L13 9l7 7M20 12.5V16h-3.5"/>',
    "camera-off": '<path d="M4 8h3l1.5-2h7L17 8h3v11H4z"/><path d="M4 20 20 4"/>',
    "edit": '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M14.5 5.5 18.5 9.5"/>',
    "logout": '<path d="M9 5H5v14h4M15 8l4 4-4 4M19 12H9"/>',
    "phone": '<rect x="6" y="2.5" width="12" height="19" rx="2.5"/><path d="M11 18.5h2"/>',
    "shield": '<path d="M12 3l7.5 3v6c0 4.5-3 7.6-7.5 9-4.5-1.4-7.5-4.5-7.5-9V6z"/>',
    "wifi": '<path d="M2.5 8.5a15 15 0 0 1 19 0M5.5 12a10.5 10.5 0 0 1 13 0M8.5 15.4a6 6 0 0 1 7 0M12 19h.01"/>',
    "bolt": '<path d="M13 2 5 13h5l-1 9 8-11h-5z"/>',
}


def icon(name, cls="i"):
    return f'<svg class="{cls}" aria-hidden="true"><use href="#i-{name}"/></svg>'


def sprite():
    parts = ['<svg xmlns="http://www.w3.org/2000/svg" style="display:none" aria-hidden="true">']
    for name, body in ICONS.items():
        parts.append(
            f'<symbol id="i-{name}" viewBox="0 0 24 24" fill="none" stroke="currentColor" '
            f'stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">{body}</symbol>'
        )
    parts.append("</svg>")
    return "".join(parts)


# ---------------------------------------------------------------- shell

NAV = [
    ("index.html", "home", "Hoy", "hoy"),
    ("comidas.html", "utensils", "Comidas", "comidas"),
    ("rutina.html", "dumbbell", "Rutina", "rutina"),
    ("progreso.html", "chart", "Progreso", "progreso"),
    ("perfil.html", "user", "Perfil", "perfil"),
]

THEME_SCRIPT = """<script>
    (function () {
      try {
        var raw = localStorage.getItem("fitordie:demo:v1");
        if (raw) {
          var st = JSON.parse(raw);
          if (st && st.theme) document.documentElement.setAttribute("data-theme", st.theme);
        }
      } catch (e) {}
    })();
  </script>"""


def nav_bottom(active):
    items = ""
    for href, ico, label, key in NAV:
        cur = ' aria-current="page"' if key == active else ""
        items += (
            f'<a class="bottomnav__item" href="{href}"{cur}>'
            f'<span class="bottomnav__pill">{icon(ico)}</span>'
            f"<span>{label}</span></a>"
        )
    return f'<nav class="bottomnav" aria-label="Navegación principal">{items}</nav>'


def nav_rail(active):
    items = ""
    for href, ico, label, key in NAV:
        cur = ' aria-current="page"' if key == active else ""
        items += f'<a class="railnav__item" href="{href}"{cur}>{icon(ico)}<span>{label}</span></a>'
    return f"""<nav class="railnav" aria-label="Navegación principal">
  <div class="railnav__brand"><span class="avatar">FD</span><span>FitOrDie</span></div>
  {items}
  <div class="railnav__foot">Plan Fran · Mesociclo 1<br>v0.1 · prototipo UI</div>
</nav>"""


def theme_button():
    return (
        '<button class="iconbtn" type="button" data-theme-toggle '
        'aria-label="Cambiar entre tema claro y oscuro">'
        f'<svg class="i theme-sun" aria-hidden="true"><use href="#i-sun"/></svg>'
        f'<svg class="i theme-moon" aria-hidden="true"><use href="#i-moon"/></svg>'
        "</button>"
    )


def shell(page, title, active, body, head="", seed=False):
    seed_tag = (
        f'<script type="application/json" id="seed-data">{SEED_JSON}</script>'
        if seed
        else ""
    )
    return f"""<!DOCTYPE html>
<html lang="es-AR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <title>{esc(title)} · FitOrDie</title>
  <meta name="description" content="FitOrDie — diario de comidas, macros y rutina. App personal.">
  <meta name="color-scheme" content="dark light">
  <meta name="theme-color" content="#0a0f0e">
  <link rel="manifest" href="manifest.webmanifest">
  <link rel="icon" href="icons/icon.svg" type="image/svg+xml">
  <link rel="icon" href="icons/favicon-32.png" sizes="32x32" type="image/png">
  <link rel="apple-touch-icon" href="icons/apple-touch-icon-180.png">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="apple-mobile-web-app-title" content="FitOrDie">
  <link rel="stylesheet" href="css/app.css">
  {THEME_SCRIPT}
  {head}
</head>
<body data-page="{page}">
  {sprite()}
  <a class="skip-link" href="#main">Saltar al contenido</a>
  {nav_rail(active)}
  <div class="app-shell">
    <main class="app-main" id="main">
{body}
    </main>
  </div>
  {nav_bottom(active)}
  <div class="toast" id="toast" role="status" aria-live="polite"></div>
  {seed_tag}
  <script src="js/demo.js" defer></script>
</body>
</html>
"""


def pagehead(kicker, title, sub=None, actions="", extra=""):
    subhtml = f'<p class="pagehead__sub">{sub}</p>' if sub else ""
    return f"""<header class="pagehead">
  <div>
    <p class="kicker">{kicker}</p>
    <h1 class="pagehead__title">{title}</h1>
    {subhtml}
  </div>
  <div class="pagehead__actions">{actions or theme_button()}</div>
</header>
{extra}"""


def macro_row(key, label, value, target, unit="g", color=None):
    color = color or f"var(--{key})"
    p = min(100, (value / target * 100) if target else 0)
    return f"""<div class="macro">
  <div class="macro__head">
    <span class="macro__name"><i class="dot" style="--c:{color}"></i>{label}</span>
    <span class="macro__val" id="val-{key}">{nf(value)} / {nf(target)} {unit}</span>
  </div>
  <div class="bar" id="bar-{key}" style="--v:{p:.1f}%"><i style="--bar:{color}"></i></div>
</div>"""


# ================================================================ PANTALLA: HOY


def page_hoy():
    kcal_left = TARGET["kcal"] - TOT["kcal"]
    ring_pct = min(100, TOT["kcal"] / TARGET["kcal"] * 100)

    meals = ""
    for sid, sname, stime in SLOTS:
        items = [e for e in SEED if e["slot"] == sid]
        st = slot_totals(sid)
        if items:
            names = " · ".join(e["name"] for e in items)
            meals += f"""<li class="meal">
  <div class="meal__time">{stime}</div>
  <div style="min-width:0">
    <div class="meal__name">{sname}</div>
    <div class="meal__items">{names}</div>
  </div>
  <div style="display:flex;align-items:center;gap:8px">
    <div class="meal__cal">{nf(r0(st["kcal"]))}<small>P {nf(r0(st["p"]))} g</small></div>
    <a class="iconbtn iconbtn--plain" href="comidas.html?slot={sid}" aria-label="Cargar en {sname}">{icon("plus")}</a>
  </div>
</li>"""
        else:
            meals += f"""<li class="meal meal__empty">
  <div class="meal__time">{stime}</div>
  <div style="min-width:0">
    <div class="meal__name">{sname} <span class="badge">sin cargar</span></div>
    <div class="meal__items">Tocá + para anotar</div>
  </div>
  <div style="display:flex;align-items:center;gap:8px">
    <div class="meal__cal">—<small>kcal</small></div>
    <a class="iconbtn iconbtn--plain" href="comidas.html?slot={sid}" aria-label="Cargar en {sname}">{icon("plus")}</a>
  </div>
</li>"""

    quick = ["Huevo", "Pollo pechuga", "Yogur griego sin azúcar", "Banana", "Arroz blanco cocido", "Queso cremoso"]
    chips = "".join(
        f'<a class="chip" href="comidas.html?q={esc(fold(q))}">{esc(q)}</a>' for q in quick
    )

    lost = START_WEIGHT - CURRENT_KG
    journey = START_WEIGHT - 84.5
    prog = min(100, lost / journey * 100)

    body = f"""{pagehead(human_date(TODAY.isoformat()), "Hoy",
        f"Plan Fran · cena pendiente",
        actions=f'<span class="badge badge--accent">{icon("flame", "i i--sm")} 12 días</span>{theme_button()}')}

<div class="grid-page">
<section class="card card--hero span-2">
  <div class="hero">
    <div class="ring" id="ring" style="--pct:{ring_pct:.1f}">
      <div>
        <div class="ring__num">{nf(r0(TOT["kcal"]))}</div>
        <div class="ring__label">de {nf(TARGET["kcal"])} kcal</div>
      </div>
    </div>
    <div class="hero__side">
      <div class="hero__kcal" id="hero-kcal">{nf(r0(kcal_left))} <small>kcal disponibles</small></div>
      <p class="hero__state" id="hero-state">Llevás {nf(r0(TOT["kcal"]))} kcal · proteína {nf(r0(TOT["p"]))} de {TARGET["p_max"]} g</p>
      <div class="hero__chips">
        <span class="badge badge--accent">faltan {nf(max(0, TARGET["p"] - TOT["p"]))} g de proteína</span>
        <span class="badge">2.150–2.400 kcal</span>
      </div>
    </div>
  </div>

  <div class="mt-5 stack-sm" style="gap:0">
    {macro_row("p", "Proteína", TOT["p"], TARGET["p_max"], color="var(--protein)")}
    {macro_row("c", "Carbos", TOT["c"], TARGET["c"], color="var(--carbs)")}
    {macro_row("f", "Grasas", TOT["f"], TARGET["f"], color="var(--fat)")}
  </div>
</section>

<section class="section">
  <div class="section-head">
    <h2>Comidas del día</h2>
    <a class="card__link" href="comidas.html">Cargar {icon("chevron", "i i--sm")}</a>
  </div>
  <div class="card card--pad0" style="padding:0 1rem">
    <ul class="list" id="meal-list">{meals}</ul>
  </div>
  <div class="callout callout--accent mt-3">
    {icon("sparkles")}
    <div><strong>Para cerrar el día:</strong> cena C del plan — tortilla de 3 huevos + verduras
    (~{nf(r0(CENA_C["kcal"]))} kcal · P {nf(r0(CENA_C["p"]))} g). Te deja en rango sin pasarte.</div>
  </div>
</section>

<section class="section">
  <div class="section-head"><h2>Cargar rápido</h2></div>
  <div class="card">
    <a class="input-wrap w-full" href="comidas.html" style="text-decoration:none">
      {icon("search")}
      <span class="input" style="display:flex;align-items:center;color:var(--text-dim);padding-left:2.625rem">Buscar alimento…</span>
    </a>
    <div class="chips chips--wrap mt-3">{chips}</div>
    <div class="btn-row btn-row--2 mt-4">
      <a class="btn btn--primary" href="comidas.html">{icon("plus")}Cargar comida</a>
      <a class="btn btn--secondary" href="comidas.html#ia">{icon("sparkles")}Estimar con IA</a>
    </div>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="weigh">
      <div>
        <p class="kicker">Peso</p>
        <p class="weigh__v">{nf(CURRENT_KG, 2)} <small>kg</small></p>
        <p class="text-sm muted mt-2"><span class="badge badge--ok">{nf(CURRENT_KG - WEEK_AGO["kg"], 2)} kg</span> última semana</p>
      </div>
      <div class="weigh__spark">{sparkline([w["kg"] for w in WEIGHTS[-14:]])}</div>
    </div>
    <hr class="divider">
    <div class="between">
      <span class="text-sm muted">Objetivo 84–85 kg · bajaste {nf(lost, 2)} kg</span>
      <a class="card__link" href="progreso.html">Ver progreso {icon("chevron", "i i--sm")}</a>
    </div>
    <div class="bar bar--thick mt-2" style="--v:{prog:.1f}%"><i></i></div>
  </div>
</section>

<footer class="empty span-2" style="padding-top:2rem">
  {icon("bolt", "i")}
  Agua 2,5–3 L por día · pollo y carne se pesan crudos.
</footer>
</div>"""
    return shell("hoy", "Hoy", "hoy", body, seed=True)


# ============================================================ PANTALLA: COMIDAS


def food_rows(names):
    rows = ""
    for name in names:
        f = BY_NAME[name]
        rows += f"""<button class="food" data-food="{esc(name)}">
  <span style="min-width:0;flex:1">
    <span class="food__name">{esc(name)}</span>
    <span class="food__meta">{nf(f["kcal"])} kcal · P {nf(f["p"], 1)} · C {nf(f["c"], 1)} · G {nf(f["fat"], 1)} /100 g</span>
  </span>
  <span class="food__cal">{nf(f["kcal"])}<br><span class="text-xs mute">kcal</span></span>
  <span class="iconbtn iconbtn--plain" aria-hidden="true">{icon("plus")}</span>
</button>"""

    return rows


def page_comidas():
    default_names = ["Huevo", "Pollo pechuga", "Yogur griego sin azúcar", "Arroz blanco cocido",
                     "Banana", "Queso cremoso", "Aceite de oliva", "Avena"]

    chips = ""
    active_slot = "almuerzo"
    for sid, sname, stime in SLOTS:
        chips += (
            f'<button class="chip" type="button" data-slot="{sid}" '
            f'aria-pressed="{str(sid == active_slot).lower()}">{sname}</button>'
        )

    entries_html = ""
    for sid, sname, _ in SLOTS:
        items = [e for e in SEED if e["slot"] == sid]
        if not items:
            continue
        st = slot_totals(sid)
        entries_html += (
            f'<div class="hist__week" style="padding-top:16px">{sname}'
            f'<span class="badge">{nf(r0(st["kcal"]))} kcal</span></div>'
        )
        for e in items:
            entries_html += f"""<div class="entry" data-entry="{e["id"]}">
  <div style="min-width:0">
    <div class="entry__name truncate">{esc(e["name"])}</div>
    <div class="entry__qty">{nf(e["qty"])} {e["unit"]} · {nf(e["grams"])} g</div>
  </div>
  <div class="entry__macros">{nf(r0(e["kcal"]))} kcal<br>P {nf(r0(e["p"]))} · C {nf(r0(e["c"]))} · G {nf(r0(e["f"]))}</div>
  <button class="iconbtn iconbtn--plain" data-del="{e["id"]}" aria-label="Borrar {esc(e["name"])}">{icon("trash", "i i--sm")}</button>
</div>"""

    foods_json = json.dumps({"foods": FOODS}, ensure_ascii=False, separators=(",", ":"))
    body = f"""{pagehead(human_date(TODAY.isoformat()), "Comidas",
        f"Hoy: {nf(r0(TOT['kcal']))} de {nf(TARGET['kcal'])} kcal",
        actions=f'<button class="iconbtn iconbtn--accent" type="button" id="open-ai" aria-label="Estimar con IA">{icon("sparkles")}</button>{theme_button()}')}

<div class="mealbar">
  <div class="chips" role="group" aria-label="¿En qué comida lo cargás?">{chips}</div>
</div>

<div class="grid-page">
<section class="card">
  <div class="between mb-3">
    <div>
      <p class="card__title">Buscar alimento</p>
      <p class="hint">Tabla local · valores por 100 g</p>
    </div>
    <button class="btn btn--soft btn--sm" type="button" data-mock="Cámara: pendiente en esta fase (la IA de foto usa LLM_BASE_URL)">
      {icon("camera", "i i--sm")}Foto
    </button>
  </div>
  <div class="input-wrap">
    {icon("search")}
    <input class="input" id="search" type="search" inputmode="search" autocomplete="off"
           enterkeyhint="search" placeholder="Pollo, arroz, yogur…" aria-label="Buscar alimento en la tabla">
  </div>
  <p class="hint mt-2">Cargando en <strong class="accent" id="target-slot">Almuerzo</strong> · elegí otra comida arriba</p>
  <div class="results" id="results" role="listbox" aria-label="Resultados">
    {food_rows(default_names)}
  </div>
</section>

<section class="section">
  <div class="section-head">
    <h2>Cargado hoy</h2>
    <span class="badge">{nf(len(SEED))} ítems</span>
  </div>
  <div class="card card--pad0" style="padding:0 1rem 0.5rem">
    <div id="day-list">{entries_html}</div>
  </div>
</section>

<section class="section span-2">
  <div class="callout callout--warn">
    {icon("alert")}
    <div><strong>Sin datos = sin datos.</strong> Si un alimento no está en la tabla, no se estima a ojo:
    se marca como faltante. La IA interpreta texto, los macros salen siempre de la tabla.</div>
  </div>
</section>

</div>

<div class="daybar">
  <div style="min-width:0">
    <div class="daybar__num" id="daybar-kcal">{nf(r0(TOT["kcal"]))} <small>/ {nf(TARGET["kcal"])} kcal</small></div>
    <div class="bar mt-2" id="daybar-bar" style="--v:{min(100, TOT['kcal'] / TARGET['kcal'] * 100):.1f}%;width:8rem"><i></i></div>
  </div>
  <div class="daybar__macros">
    <span style="color:var(--protein)" id="daybar-p">P {nf(r0(TOT["p"]))} g</span>
    <span style="color:var(--carbs)" id="daybar-c">C {nf(r0(TOT["c"]))} g</span>
    <span style="color:var(--fat)" id="daybar-f">G {nf(r0(TOT["f"]))} g</span>
  </div>
</div>

<script type="application/json" id="foods-data">{foods_json}</script>

<div class="sheet" id="sheet-food" role="dialog" aria-modal="true" aria-labelledby="ef-name">
  <div class="sheet__panel">
    <div class="sheet__grab"></div>
    <div class="sheet__head">
      <div style="min-width:0">
        <p class="sheet__title" id="ef-name">Alimento</p>
        <p class="sheet__sub" id="ef-meta">—</p>
      </div>
      <button class="iconbtn iconbtn--plain" type="button" data-close aria-label="Cerrar">{icon("x")}</button>
    </div>

    <div class="field">
      <label class="label" for="ef-qty">Cantidad <span class="hint" id="ef-grams"></span></label>
      <div class="stepper">
        <button type="button" data-step="-10" aria-label="Restar 10">{icon("x", "i i--sm")}</button>
        <input id="ef-qty" type="number" min="0" step="1" inputmode="decimal" value="100" aria-describedby="ef-hint">
        <button type="button" data-step="10" aria-label="Sumar 10">{icon("plus", "i i--sm")}</button>
      </div>
    </div>

    <div class="field">
      <label class="label" for="ef-unit">Medida</label>
      <select class="input" id="ef-unit">
        <option value="g">gramos (g)</option>
      </select>
    </div>

    <p class="hint mt-3" id="ef-hint">Pesá crudo, como indica el plan.</p>

    <div class="calc" aria-live="polite">
      <div class="calc__cell calc__cell--kcal"><div class="calc__v" id="ef-kcal">0</div><div class="calc__l">kcal</div></div>
      <div class="calc__cell calc__cell--p"><div class="calc__v" id="ef-p">0</div><div class="calc__l">prot</div></div>
      <div class="calc__cell calc__cell--c"><div class="calc__v" id="ef-c">0</div><div class="calc__l">carb</div></div>
      <div class="calc__cell calc__cell--g"><div class="calc__v" id="ef-g">0</div><div class="calc__l">gras</div></div>
    </div>

    <div class="sheet__actions btn-row btn-row--2">
      <button class="btn btn--secondary" type="button" data-close>Cancelar</button>
      <button class="btn btn--primary" type="button" id="ef-add">
        {icon("plus")}Sumar a <span id="ef-slot">Almuerzo</span>
      </button>
    </div>
  </div>
</div>

<div class="sheet" id="sheet-ai" role="dialog" aria-modal="true" aria-labelledby="ai-title">
  <div class="sheet__panel">
    <div class="sheet__grab"></div>
    <div class="sheet__head">
      <div>
        <p class="sheet__title" id="ai-title">Estimar con IA</p>
        <p class="sheet__sub">Escribí lo que comiste, en tus palabras</p>
      </div>
      <button class="iconbtn iconbtn--plain" type="button" data-close aria-label="Cerrar">{icon("x")}</button>
    </div>

    <div class="field">
      <label class="label" for="ai-text">Tu texto</label>
      <textarea class="input" id="ai-text" rows="3"
        placeholder="Ej: 250 g de pollo con arroz y ensalada"></textarea>
      <p class="hint" id="ai-note">El parser local busca los alimentos en la tabla. La IA solo interpreta:
      si el alimento no está, se avisa y no se inventan macros.</p>
    </div>

    <div class="mt-4" id="ai-results"></div>

    <div class="sheet__actions btn-row btn-row--2">
      <button class="btn btn--secondary" type="button" id="ai-estimate">{icon("sparkles")}Estimar</button>
      <button class="btn btn--primary" type="button" id="ai-add-all">{icon("plus")}Sumar todo</button>
    </div>
  </div>
</div>"""
    return shell("comidas", "Comidas", "comidas", body, seed=True)


# ============================================================= PANTALLA: RUTINA

ROUTINE = {
    "A": {
        "title": "Empuje + piernas",
        "sub": "Prensa, pecho, remo, hombro y bíceps",
        "items": [
            ("prensa-de-piernas", "Prensa de piernas", "3 × 10-12", "RIR 3-4", "2 min",
             "Bajada controlada, sin trabar las rodillas al final.", 3),
            ("press-pecho-maquina", "Press de pecho (máquina o mancuernas)", "3 × 10-12", "RIR 3-4", "2 min",
             "Bajada de 2 segundos, sin rebotar en el pecho.", 3),
            ("remo-maquina", "Remo (máquina o mancuernas)", "3 × 10-12", "RIR 3-4", "2 min",
             "Codos cerca del torso, sin balancear la espalda.", 3),
            ("elevaciones-laterales", "Elevaciones laterales", "2 × 12-15", "RIR 2-3", "60 s",
             "Hombros abajo: subí el codo, no el trapecio.", 2),
            ("curl-biceps", "Curl de bíceps", "2 × 10-12", "RIR 2-3", "60 s",
             "Codos pegados al torso, bajada de 2 segundos.", 2),
        ],
    },
    "B": {
        "title": "Femoral + hombro + tirón",
        "sub": "Cadena posterior, hombro y espalda",
        "items": [
            ("curl-femoral", "Curl femoral en máquina", "3 × 12-15", "RIR 3-4", "90 s",
             "Movimiento lento, sentí el femoral en la subida.", 3),
            ("press-militar", "Press militar sentado", "3 × 10-12", "RIR 3-4", "2 min",
             "Espalda apoyada, no arquees la lumbar.", 3),
            ("jalon-al-pecho", "Jalón al pecho", "3 × 10-12", "RIR 3-4", "90 s",
             "Llevá la barra al pecho, no atrás de la nuca.", 3),
            ("extension-triceps", "Extensión de tríceps en polea", "2 × 10-12", "RIR 2-3", "60 s",
             "Codos fijos al costado, extendé completo.", 2),
        ],
    },
    "C": {
        "title": "Piernas + espalda",
        "sub": "Prensa, press inclinado y remos",
        "items": [
            ("prensa-c", "Prensa de piernas", "3 × 10-12", "RIR 3-4", "2 min",
             "Mismo rango que el día A: buscá sentir la pierna.", 3),
            ("press-inclinado", "Press inclinado con mancuernas", "3 × 10-12", "RIR 3-4", "2 min",
             "Banco a 30°, bajada controlada.", 3),
            ("remo-unilateral", "Remo unilateral con mancuerna", "3 × 10-12", "RIR 3-4", "90 s",
             "Apoyá la mano, tirá con el codo pegado.", 3),
            ("remo-polea-baja", "Remo en polea baja sentado", "3 × 10-12", "RIR 3-4", "90 s",
             "Pecho arriba, sin encorvar la espalda.", 3),
        ],
    },
}

YT = "https://www.youtube.com/results?search_query="


def yt_url(name):
    return YT + quote_plus(f"{name} tecnica correcta")


def exercise_card(day, idx, item):
    slug_, name, sets, rir, rest, note, nsets = item
    eid = f"{day}{idx}"
    rows = ""
    for s in range(1, nsets + 1):
        rows += f"""<div class="series__grid">
  <span class="series__label">S{s}</span>
  <input class="input" type="number" inputmode="decimal" min="0" step="0.5" placeholder="kg"
         aria-label="{esc(name)} serie {s}: peso en kilos">
  <input class="input" type="number" inputmode="numeric" min="0" step="1" placeholder="reps"
         aria-label="{esc(name)} serie {s}: repeticiones">
</div>"""
    return f"""<article class="ex" data-ex="{eid}">
  <div class="ex__media">
    {icon("image", "i i--xl")}
    <span>Imagen o GIF pendiente</span>
    <a class="btn btn--secondary btn--sm ex__play" href="{yt_url(name)}" target="_blank" rel="noopener noreferrer">
      {icon("play", "i i--sm")}Ver video
    </a>
  </div>
  <div class="ex__body">
    <div class="ex__top">
      <div style="min-width:0">
        <h3 class="ex__name">{esc(name)}</h3>
        <p class="ex__note">{esc(note)}</p>
      </div>
      <button class="checkbtn" type="button" data-check="{eid}" aria-pressed="false"
              aria-label="Marcar {esc(name)} como completado">{icon("check")}</button>
    </div>
    <div class="ex__tags">
      <span class="tag">{sets}</span>
      <span class="tag">{rir}</span>
      <span class="tag">descanso {rest}</span>
    </div>
    <details class="series">
      <summary class="btn btn--secondary btn--sm w-full">
        <span>Registrar series</span>{icon("chevron-down", "i i--sm chev")}
      </summary>
      <div class="mt-3">
        <div class="series__grid">
          <span class="series__label">Serie</span>
          <span class="series__label">kg</span>
          <span class="series__label">reps</span>
        </div>
        {rows}
        <p class="hint mt-3">Progresión: subí reps de 10 a 12 y recién ahí +5 % de peso.</p>
      </div>
    </details>
  </div>
</article>"""


def page_rutina():
    tabs = ""
    for key in ("A", "B", "C"):
        tabs += (
            f'<button class="daytab" type="button" data-daytab="{key}" '
            f'aria-pressed="{str(key == "A").lower()}">'
            f'<span class="daytab__k">Día {key}</span>'
            f'<span class="daytab__n">{ROUTINE[key]["title"]}</span></button>'
        )

    sections = ""
    for key in ("A", "B", "C"):
        data = ROUTINE[key]
        cards = "".join(exercise_card(key, i + 1, it) for i, it in enumerate(data["items"]))
        sections += f"""<section data-day="{key}"{" hidden" if key != "A" else ""} class="section">
  <div class="card card--tight">
    <div class="between">
      <div>
        <p class="kicker">Día {key} · Mesociclo 1</p>
        <p class="strong mt-1">{data["title"]}</p>
      </div>
      <span class="badge" data-progress-label>0 de {len(data["items"])} hechos</span>
    </div>
    <div class="bar bar--thick mt-3" data-progress style="--v:0%"><i></i></div>
    <p class="hint mt-2">{data["sub"]}</p>
  </div>
  <div class="stack mt-3">{cards}</div>
  <div class="callout mt-3">
    {icon("info")}
    <div>RIR 3-4 siempre: nunca al fallo. Exhalá en el esfuerzo, levantate y sentate lento.</div>
  </div>
</section>"""

    body = f"""{pagehead("Mesociclo 1 · 3 días por semana", "Rutina",
        "Fuerza A/B/C · sentate entre series",
        actions=theme_button())}

<div class="daytabs mb-2" role="group" aria-label="Elegí el día">{tabs}</div>
{sections}

<section class="section">
  <div class="card">
    <p class="card__title">Cardio y pendientes</p>
    <div class="kv mt-2"><span class="kv__k">Caminata</span><span class="kv__v">20–30 min · martes y jueves</span></div>
    <div class="kv"><span class="kv__k">Eco de control</span><span class="kv__v"><span class="badge badge--warn">pendiente</span></span></div>
    <div class="kv"><span class="kv__k">Frecuencia</span><span class="kv__v">3 días/semana</span></div>
  </div>
</section>

<footer class="empty">{icon("dumbbell", "i")}Si algo duele, paramos y consultamos. El plan no es una orden médica.</footer>"""
    return shell("rutina", "Rutina", "rutina", body)


# =========================================================== PANTALLA: PROGRESO


def page_progreso():
    lost = START_WEIGHT - CURRENT_KG
    remaining = CURRENT_KG - 84.5
    prog = min(100, lost / (START_WEIGHT - 84.5) * 100)

    week = [w for w in WEIGHTS if w["date"] >= (TODAY - datetime.timedelta(days=6)).isoformat()]
    avg_week = sum(w["kg"] for w in week) / len(week)

    hist = ""
    rows = list(reversed(WEIGHTS))[:10]
    for i, w in enumerate(rows):
        prev = rows[i + 1]["kg"] if i + 1 < len(rows) else None
        delta = w["kg"] - prev if prev is not None else 0
        if i == 0:
            badge = '<span class="badge badge--accent">hoy</span>'
        elif abs(delta) < 0.005:
            badge = '<span class="badge">igual</span>'
        else:
            cls = "badge--ok" if delta < 0 else "badge--warn"
            sign = "−" if delta < 0 else "+"
            badge = f'<span class="badge {cls}">{sign}{nf(abs(delta), 2)}</span>'
        hist += f"""<div class="hist__row">
  <div class="hist__date">{short_date(w["date"])}{" · hoy" if i == 0 else ""}</div>
  {badge}
  <div class="hist__kg">{nf(w["kg"], 2)} kg</div>
</div>"""

    body = f"""{pagehead(f"Últimos 30 días · desde {nf(START_WEIGHT, 1)} kg", "Progreso",
        f"Bajaste {nf(lost, 2)} kg desde el arranque",
        actions=f'<span class="badge badge--ok">{icon("trend", "i i--sm")} −{nf(lost, 2)} kg</span>{theme_button()}')}

<div class="grid-page">
<section class="card card--hero span-2">
  <div class="between" style="align-items:flex-start">
    <div>
      <p class="kicker">Peso actual</p>
      <p class="bigv mt-2" id="weight-now">{nf(CURRENT_KG, 2)} <small>kg</small></p>
      <p class="text-sm muted mt-2">
        <span class="badge badge--ok">{nf(CURRENT_KG - WEEK_AGO["kg"], 2)} kg</span>
        en 7 días · promedio semanal {nf(avg_week, 2)} kg
      </p>
    </div>
    <div class="ring" style="--pct:{prog:.1f};--ring-size:5.5rem" role="img"
         aria-label="{prog:.0f}% del camino al objetivo">
      <div class="center">
        <div class="ring__num" style="font-size:1.125rem">{prog:.0f}%</div>
        <div class="ring__label" style="font-size:0.6875rem">al goal</div>
      </div>
    </div>
  </div>
  <div class="bar bar--thick mt-4" style="--v:{prog:.1f}%"><i></i></div>
  <div class="between mt-2 text-xs mute">
    <span>96,00 kg</span><span>objetivo 84–85 kg</span>
  </div>
  <p class="hint mt-3">Te faltan {nf(remaining, 2)} kg. A −0,35 kg/semana son ~{remaining / 0.35:.0f} semanas.</p>
</section>

<section class="section">
  <div class="card">
    <div class="card__head">
      <p class="card__title">Tendencia</p>
      <div class="legend"><span class="legend__item"><i class="legend__line"></i>peso</span></div>
    </div>
    {line_chart([(w["date"], w["kg"]) for w in WEIGHTS], "chartFillBig")}
  </div>
</section>

<section class="section">
  <div class="stats">
    <div class="stat"><div class="stat__v">{nf(avg_week, 2)} <small>kg</small></div><div class="stat__l">promedio 7 días</div></div>
    <div class="stat"><div class="stat__v">86<small>%</small></div><div class="stat__l">adherencia · 24 de 28 días</div></div>
    <div class="stat"><div class="stat__v">{icon("flame", "i")} 12</div><div class="stat__l">racha de días cargados</div></div>
    <div class="stat"><div class="stat__v">−0,35 <small>kg/sem</small></div><div class="stat__l">ritmo actual (objetivo)</div></div>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="card__head"><p class="card__title">Registrar peso de hoy</p></div>
    <form id="weight-form" novalidate>
      <div class="field">
        <label class="label" for="weight-input">Peso (kg)</label>
        <div class="row-4" style="gap:0.5rem">
          <input class="input" id="weight-input" type="number" inputmode="decimal" step="0.05"
                 min="30" max="250" placeholder="89,65" aria-describedby="weight-hint" style="flex:1">
          <button class="btn btn--primary" type="submit">{icon("scale", "i i--sm")}Guardar</button>
        </div>
        <p class="hint" id="weight-hint">Mismo momento del día, idealmente en ayunas. Un valor por día.</p>
      </div>
    </form>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="card__head">
      <p class="card__title">Historial</p>
      <span class="badge">últimos 10</span>
    </div>
    <div id="hist-list">{hist}</div>
  </div>
</section>

<section class="section span-2">
  <div class="callout callout--accent">
    {icon("alert")}
    <div><strong>Regla del plan:</strong> si bajás más de 0,5 kg por semana, sumá 200 kcal.
    Si no bajás nada en 2 semanas, revisá primero las porciones antes de recortar más.</div>
  </div>
</section>
</div>"""
    return shell("progreso", "Progreso", "progreso", body)


# ============================================================= PANTALLA: PERFIL


def page_perfil():
    body = f"""{pagehead("Plan Fran · Mesociclo 1", "Perfil", actions=theme_button())}

<div class="grid-page">
<section class="card span-2">
  <div class="identity">
    <span class="avatar" aria-hidden="true">JS</span>
    <div style="min-width:0">
      <p class="text-lg strong">Javier Solis</p>
      <p class="text-sm muted">38 años · 1,85 m · Córdoba</p>
    </div>
    <span class="badge badge--accent" style="margin-left:auto">Keto → Plan Fran</span>
  </div>
  <div class="stats mt-4">
    <div class="stat card--flat"><div class="stat__v">{nf(CURRENT_KG, 2)} <small>kg</small></div><div class="stat__l">peso actual</div></div>
    <div class="stat card--flat"><div class="stat__v">84–85 <small>kg</small></div><div class="stat__l">objetivo</div></div>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="card__head">
      <p class="card__title">Objetivos diarios</p>
      <button class="card__link" type="button" data-mock="Edición de objetivos: la maneja el backend (PATCH /api/perfil)">{icon("edit", "i i--sm")}Editar</button>
    </div>
    <div class="kv"><span class="kv__k">Calorías</span><span class="kv__v">2.150–2.400 kcal</span></div>
    <div class="kv"><span class="kv__k">Proteína</span><span class="kv__v">155–170 g</span></div>
    <div class="kv"><span class="kv__k">Peso objetivo</span><span class="kv__v">84–85 kg</span></div>
    <div class="kv"><span class="kv__k">TMB estimada</span><span class="kv__v">1.868 kcal</span></div>
    <div class="kv"><span class="kv__k">Ritmo buscado</span><span class="kv__v">−0,35 kg/semana</span></div>
    <div class="kv"><span class="kv__k">Altura</span><span class="kv__v">1,85 m (el plan asume 1,87)</span></div>
    <p class="hint mt-3">Rango, no número fijo: el plan tolera 2.150 a 2.400 kcal según hambre y entrenamiento.</p>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="card__head"><p class="card__title">Datos del perfil</p></div>
    <div class="kv"><span class="kv__k">Edad</span><span class="kv__v">38</span></div>
    <div class="kv"><span class="kv__k">Actividad</span><span class="kv__v">Fuerza 3×/semana + caminata</span></div>
    <div class="kv"><span class="kv__k">Comidas</span><span class="kv__v">5 + picoteo</span></div>
    <div class="kv"><span class="kv__k">Zona horaria</span><span class="kv__v">America/Argentina/Buenos_Aires</span></div>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="card__head"><p class="card__title">Preferencias</p></div>
    <div class="field">
      <span class="label">Tema</span>
      <div class="segmented" role="group" aria-label="Tema de la interfaz">
        <button type="button" data-theme-choice="light" aria-pressed="false">{icon("sun", "i i--sm")}Claro</button>
        <button type="button" data-theme-choice="dark" aria-pressed="false">{icon("moon", "i i--sm")}Oscuro</button>
        <button type="button" data-theme-choice="system" aria-pressed="true">Auto</button>
      </div>
    </div>
    <div class="mt-4">
      <label class="switch-row" for="pin-switch">
        <span><span class="setting__label">Bloqueo con PIN</span><span class="setting__sub">Pide un PIN al abrir la app</span></span>
        <input class="switch" type="checkbox" id="pin-switch" role="switch">
      </label>
      <label class="switch-row" for="notif-switch">
        <span><span class="setting__label">Recordatorios de comida</span><span class="setting__sub">7:30 · 11:00 · 13:30 · 16:30 · 21:00</span></span>
        <input class="switch" type="checkbox" id="notif-switch" role="switch" checked>
      </label>
      <label class="switch-row" for="offline-switch">
        <span><span class="setting__label">Modo offline</span><span class="setting__sub">Guarda la última vista para leer sin red</span></span>
        <input class="switch" type="checkbox" id="offline-switch" role="switch" checked>
      </label>
    </div>
  </div>
</section>

<section class="section">
  <div class="card">
    <div class="card__head"><p class="card__title">Acceso y privacidad</p></div>
    <div class="setting">
      <div><div class="setting__label">Red</div><div class="setting__sub">Solo por Tailscale, sin exponer a internet</div></div>
      <span class="badge badge--ok">{icon("lock", "i i--sm")}100.100.210.126:3007</span>
    </div>
    <div class="setting">
      <div><div class="setting__label">Datos</div><div class="setting__sub">SQLite local en el VPS, sin terceros</div></div>
      <span class="badge">fitordie.db</span>
    </div>
    <div class="setting">
      <div><div class="setting__label">Exportar historial</div><div class="setting__sub">CSV de comidas y pesos</div></div>
      <button class="btn btn--secondary btn--sm" type="button" data-mock="Exportar: GET /api/export.csv (lo implementa el backend)">{icon("download", "i i--sm")}CSV</button>
    </div>
    <div class="setting">
      <div><div class="setting__label">Borrar caché offline</div><div class="setting__sub">Datos locales del navegador</div></div>
      <button class="btn btn--outline btn--sm" type="button" data-mock="Caché del service worker: se limpia desde Ajustes del navegador">{icon("trash", "i i--sm")}Borrar</button>
    </div>
    <div class="setting">
      <div><div class="setting__label">Notas de seguridad</div><div class="setting__sub">Qué se valida y qué no se guarda</div></div>
      <a class="btn btn--ghost btn--sm" href="SECURITY.md">Leer {icon("chevron", "i i--sm")}</a>
    </div>
  </div>
</section>

<footer class="empty span-2">
  FitOrDie v0.1 · prototipo UI<br>
  Servido por Go en :3007 · PWA instalable · sin Node en el server
</footer>
</div>"""
    return shell("perfil", "Perfil", "perfil", body)


# ================================================================ manifest/sw

def write_manifest():
    manifest = {
        "name": "FitOrDie",
        "short_name": "FitOrDie",
        "description": "Diario de comidas, macros y rutina. Personal, sin nube.",
        "lang": "es-AR",
        "dir": "ltr",
        "start_url": "./index.html",
        "scope": "./",
        "display": "standalone",
        "display_override": ["standalone", "minimal-ui"],
        "orientation": "portrait",
        "background_color": "#0a0f0e",
        "theme_color": "#0a0f0e",
        "categories": ["health", "fitness", "productivity"],
        "icons": [
            {"src": "icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
            {"src": "icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
            {"src": "icons/icon-maskable-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
            {"src": "icons/apple-touch-icon-180.png", "sizes": "180x180", "type": "image/png"},
        ],
        "shortcuts": [
            {"name": "Cargar comida", "url": "./comidas.html", "description": "Buscar alimento y sumarlo al día"},
            {"name": "Rutina de hoy", "url": "./rutina.html", "description": "Ver el día de fuerza"},
        ],
    }
    with open(os.path.join(OUT, "manifest.webmanifest"), "w", encoding="utf-8") as fh:
        json.dump(manifest, fh, ensure_ascii=False, indent=2)
        fh.write("\n")


def main():
    pages = {
        "index.html": page_hoy(),
        "comidas.html": page_comidas(),
        "rutina.html": page_rutina(),
        "progreso.html": page_progreso(),
        "perfil.html": page_perfil(),
    }
    for name, html in pages.items():
        with open(os.path.join(OUT, name), "w", encoding="utf-8") as fh:
            fh.write(html)
        print(f"  {name:16} {len(html):>7} bytes")
    write_manifest()
    print("  manifest.webmanifest")
    print(f"\nDía demo: {nf(r0(TOT['kcal']))} kcal · P {nf(r0(TOT['p']))} · C {nf(r0(TOT['c']))} · G {nf(r0(TOT['f']))}")
    print(f"Alimentos en tabla: {len(FOODS)}")


if __name__ == "__main__":
    main()
