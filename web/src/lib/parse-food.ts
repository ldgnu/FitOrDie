// Parser de texto libre a comida estructurada. 100% local y determinista:
// interpreta lo que puede, MARCA lo que asume y PREGUNTA lo que falta.
// Jamás inventa valores: lo no resolvible queda custom/sin-datos o pregunta.
export interface FoodRef {
  id: string;
  name: string;
  kind: "food" | "recipe";
}
export interface ParsedItem {
  label: string; // fragmento original
  kind: "food" | "recipe" | "custom";
  foodId: string | null;
  recipeId: string | null;
  qty: number;
  unit: string;
  assumed: string[];
  candidates?: Array<{ id: string; name: string; kind: "food" | "recipe" }>;
}
export interface ParseResult {
  meal: string | null;
  items: ParsedItem[];
  questions: string[];
}

const NUM_WORDS: Record<string, number> = {
  un: 1, uno: 1, una: 1, dos: 2, tres: 3, cuatro: 4, cinco: 5, seis: 6,
};
const SIZE_RE: Array<[RegExp, number]> = [
  [/chic[oa]s?/, 0.7],
  [/median[oa]s?/, 1],
  [/grandes?/, 1.3],
];
// gramos por unidad (cuando se cuenta por unidad y no por peso)
const UNIT_G: Record<string, number> = {
  huevo: 50, palta: 150, manzana: 180, banana: 120, naranja: 130,
  tomate: 120, cebolla: 150, limon: 60, papa: 150,
};
// líquidos típicos de aderezo: si no dicen cantidad, 1 cucharada (asumido)
const SPOON_ML = ["aceite", "vinagre", "tamari", "limon"];
const MEAL_RE: Array<[RegExp, string]> = [
  [/desayun/, "desayuno"],
  [/almor[sz]/, "almuerzo"],
  [/merienda|merend/, "merienda"],
  [/media ma.ana/, "media-manana"],
  [/\bcen/, "cena"],
  [/snack|picote|antojo/, "snack"],
];
const NOISE = new Set([
  "de", "la", "el", "los", "las", "del", "un", "una", "con", "y", "e",
  "en", "a", "al", "hecho", "hecha", "echo", "echa", "aderezo", "aderezado",
  "como", "mi", "me", "hoy", "ayer", "buena", "bueno", "buen", "fea", "feo",
  "rica", "rico", "muy", "mas", "poco", "mucha", "mucho", "toda", "todo",
  "grande", "mediana", "mediano", "chica", "chico",
]);

const norm = (s: string) =>
  s.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");

function singulars(s: string): string[] {
  const v = [s];
  if (s.endsWith("es") && s.length > 4) v.push(s.slice(0, -2));
  if (s.endsWith("s") && s.length > 3) v.push(s.slice(0, -1));
  return v;
}

function findCandidates(core: string, refs: FoodRef[]): FoodRef[] {
  const out: Array<{ r: FoodRef; score: number }> = [];
  for (const r of refs) {
    const n = norm(r.name);
    let score = 0;
    if (singulars(core).includes(n) || singulars(n).includes(core)) score = 3;
    else if (core.length >= 3 && (n.includes(core) || core.includes(n))) score = 2;
    if (score > 0) out.push({ r, score });
  }
  return out
    .sort((a, b) => b.score - a.score || a.r.name.localeCompare(b.r.name))
    .map((x) => x.r);
}

export function parseMealText(
  text: string,
  refs: FoodRef[],
  today: string,
): ParseResult {
  void today;
  const questions: string[] = [];
  const items: ParsedItem[] = [];
  let t = norm(text);
  if (!t.trim()) return { meal: null, items, questions: ["Texto vacío"] };

  let meal: string | null = null;
  for (const [re, m] of MEAL_RE) {
    if (re.test(t)) {
      meal = m;
      break;
    }
  }
  if (!meal) questions.push("¿Qué comida fue? (desayuno, almuerzo, merienda, cena o snack)");

  // "no X" = lo nombró para excluirlo → se saca
  t = t.replace(/\bno\b[^,+.]+/g, " ");
  const parts = t
    .split(/\s+(?:con|y|e|mas|\+)\s*|,/g)
    .map((p) => p.trim())
    .filter(Boolean);

  for (const part of parts) {
    const label = part;
    let rest = ` ${part} `;
    let qty: number | null = null;
    const assumed: string[] = [];

    let m = rest.match(/(\d+(?:[.,]\d+)?)\s*(g|gramos?|ml|cc)\b/);
    let explicitUnit: string | null = null;
    if (m) {
      qty = Number(m[1].replace(",", "."));
      explicitUnit = m[2].startsWith("g") ? "g" : "ml";
      rest = rest.replace(m[0], " ");
    }
    if (qty === null) {
      const d = rest.match(/\b(\d+(?:[.,]\d+)?)\b/);
      if (d) {
        qty = Number(d[1].replace(",", "."));
        rest = rest.replace(d[0], " ");
      } else {
        for (const w of Object.keys(NUM_WORDS)) {
          if (new RegExp(`\\b${w}\\b`).test(rest)) {
            qty = NUM_WORDS[w];
            rest = rest.replace(new RegExp(`\\b${w}\\b`), " ");
            break;
          }
        }
      }
    }

    let sizeF = 1;
    for (const [re, f] of SIZE_RE) {
      if (re.test(rest)) {
        sizeF = f;
        break;
      }
    }

    const tokens = rest.split(/[^a-zñ]+/).filter((w) => w && !NOISE.has(w));
    const core = tokens.join(" ");
    if (!core) continue;

    const cands = findCandidates(core, refs);
    if (cands.length === 0) {
      items.push({
        label, kind: "custom", foodId: null, recipeId: null,
        qty: qty ?? 1, unit: explicitUnit ?? "g",
        assumed: ["No lo encontré en tus alimentos: se guarda sin cálculo"],
      });
      questions.push(`No encontré "${label}". ¿Qué es?`);
      continue;
    }

    const chosen = cands[0];
    if (cands.length > 1) {
      questions.push(
        `"${label}" puede ser: ${cands.slice(0, 3).map((c) => c.name).join(", ")}. Elegí pre-cargado: ${chosen.name}.`,
      );
    }

    if (chosen.kind === "recipe") {
      items.push({
        label, kind: "recipe", foodId: null, recipeId: chosen.id,
        qty: qty ?? 1, unit: "porción",
        assumed: qty === null ? ["Asumí 1 porción"] : [],
        candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
      });
      continue;
    }

    // food
    if (explicitUnit) {
      items.push({
        label, kind: "food", foodId: chosen.id, recipeId: null,
        qty: qty ?? 1, unit: explicitUnit, assumed,
        candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
      });
      continue;
    }
    const key = norm(chosen.name).split(" ")[0];
    const perUnit = UNIT_G[key] ?? UNIT_G[core.split(" ")[0]];
    if (perUnit) {
      const q = (qty ?? 1) * perUnit * sizeF;
      if (qty === null) assumed.push("No dijiste cantidad: asumí 1 unidad");
      assumed.push(`${chosen.name} ≈ ${perUnit} g por unidad${sizeF !== 1 ? ` (tamaño ×${sizeF})` : ""}`);
      items.push({
        label, kind: "food", foodId: chosen.id, recipeId: null,
        qty: Math.round(q * 10) / 10, unit: "g", assumed,
        candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
      });
      continue;
    }
    if (SPOON_ML.some((s) => norm(chosen.name).includes(s))) {
      if (qty === null) {
        assumed.push("Sin cantidad: asumí 1 cucharada (10 ml)");
        qty = 10;
      }
      items.push({
        label, kind: "food", foodId: chosen.id, recipeId: null,
        qty, unit: "ml", assumed,
        candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
      });
      continue;
    }
    items.push({
      label, kind: "food", foodId: chosen.id, recipeId: null,
      qty: qty ?? 1, unit: "unidad",
      assumed: ["Sin peso por unidad: se guarda como unidad, sin cálculo"],
      candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
    });
  }

  return { meal, items, questions };
}
