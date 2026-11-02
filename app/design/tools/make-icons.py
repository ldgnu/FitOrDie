#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Genera los iconos PWA de FitOrDie (PNG) sin Node ni dependencias externas.

Marca: tres barras (los tres macros) en tinta oscura sobre verde menta.
  - icon-192 / icon-512 : uso general
  - maskable-512        : full-bleed, marca dentro del 80% seguro (Android)
  - apple-touch-icon-180: iOS (sin transparencia, iOS redondea)
  - favicon-32          : pestaña del navegador

Requiere Pillow (viene con el python del sistema acá; si falta:
  pip3 install --user pillow)

Uso:  python3 tools/make-icons.py
"""

import os

from PIL import Image, ImageDraw

OUT = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "icons")

MINT = (91, 228, 155, 255)   # --p-accent
INK = (4, 35, 26, 255)       # --p-accent-ink
SS = 4                       # supersampling para bordes suaves


def draw_mark(img_size, mark_scale=1.0, bg=MINT):
    """Devuelve una imagen RGBA cuadrada con la marca centrada."""
    size = img_size * SS
    img = Image.new("RGBA", (size, size), bg)
    d = ImageDraw.Draw(img)

    # Tres barras verticales alineadas al mismo pie, centradas ópticamente.
    unit = size * (0.44 * mark_scale)      # ancho total del grupo
    gap = unit * 0.13
    bw = (unit - gap * 2) / 3
    heights = [0.52, 0.82, 0.66]           # proporciones de la barra más alta
    max_h = unit * 1.05
    base_y = size / 2 + max_h / 2
    x0 = size / 2 - unit / 2
    r = bw * 0.28

    for i, hf in enumerate(heights):
        h = max_h * hf / 0.82
        x = x0 + i * (bw + gap)
        d.rounded_rectangle(
            [x, base_y - h, x + bw, base_y],
            radius=r,
            fill=INK,
        )

    # Punto de acento arriba a la derecha del grupo: "registro del día"
    dot_r = bw * 0.30
    cx = x0 + unit - dot_r
    cy = base_y - max_h - dot_r * 1.9
    d.ellipse([cx - dot_r, cy - dot_r, cx + dot_r, cy + dot_r], fill=INK)

    return img.resize((img_size, img_size), Image.LANCZOS)


def write(img, name):
    path = os.path.join(OUT, name)
    img.save(path, "PNG", optimize=True)
    print(f"  {name:28} {img.size[0]}x{img.size[1]}  {os.path.getsize(path)} bytes")


SVG = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" role="img" aria-label="FitOrDie">
  <rect width="512" height="512" rx="112" fill="#5be49b"/>
  <g fill="#04231a">
    <rect x="142" y="216" width="56" height="130" rx="18"/>
    <rect x="228" y="136" width="56" height="210" rx="18"/>
    <rect x="314" y="178" width="56" height="168" rx="18"/>
    <circle cx="398" cy="106" r="17"/>
  </g>
</svg>
"""


def main():
    os.makedirs(OUT, exist_ok=True)
    write(draw_mark(192), "icon-192.png")
    write(draw_mark(512), "icon-512.png")
    # maskable: full-bleed y marca más chica (safe zone 80%)
    write(draw_mark(512, mark_scale=0.72), "icon-maskable-512.png")
    # iOS: sin transparencia, fondo completo
    write(draw_mark(180, mark_scale=0.86).convert("RGB"), "apple-touch-icon-180.png")
    write(draw_mark(32, mark_scale=0.9), "favicon-32.png")
    with open(os.path.join(OUT, "icon.svg"), "w", encoding="utf-8") as fh:
        fh.write(SVG)
    print("  icon.svg                     vector")


if __name__ == "__main__":
    main()
