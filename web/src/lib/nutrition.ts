// Motor nutricional de FitOrDie — funciones puras, deterministas, sin IA.
// Todo valor desconocido se representa como null y propaga "sin datos".
// Nunca se inventa precisión: el redondeo es a 1 decimal solo en presentación.

export interface Nutrients {
  kcal: number;
  protein: number; // g
  fat: number; // g
  carbs: number; // g totales
  fiber: number; // g
}

/** Carbohidratos netos estimados = totales − fibra. Null si falta dato. */
export function netCarbs(n: {
  carbs: number | null;
  fiber: number | null;
}): number | null {
  if (n.carbs == null || n.fiber == null) return null;
  return round1(Math.max(0, n.carbs - n.fiber));
}

/** Escala nutrientes de una cantidad base a la cantidad consumida. */
export function scaleNutrients(
  base: Nutrients,
  baseQty: number,
  qty: number,
): Nutrients {
  const f = qty / baseQty;
  return {
    kcal: round1(base.kcal * f),
    protein: round1(base.protein * f),
    fat: round1(base.fat * f),
    carbs: round1(base.carbs * f),
    fiber: round1(base.fiber * f),
  };
}

/** Suma una lista de nutrientes (p. ej. ingredientes de una comida). */
export function sumNutrients(list: Nutrients[]): Nutrients {
  const zero: Nutrients = { kcal: 0, protein: 0, fat: 0, carbs: 0, fiber: 0 };
  return list.reduce(
    (acc, n) => ({
      kcal: round1(acc.kcal + n.kcal),
      protein: round1(acc.protein + n.protein),
      fat: round1(acc.fat + n.fat),
      carbs: round1(acc.carbs + n.carbs),
      fiber: round1(acc.fiber + n.fiber),
    }),
    zero,
  );
}

export type KetoVerdict =
  | "compatible"
  | "con-ajustes"
  | "dificil"
  | "sin-datos";

/**
 * Evaluación keto orientativa de UNA comida contra el objetivo diario de
 * carbohidratos netos (default 30 g, configurable por usuario).
 * Heurística v1: la comida es compatible si usa ≤30% del presupuesto diario.
 * No mide ni promete cetosis.
 */
export function ketoVerdict(
  netos: number | null,
  objetivoDiario = 30,
): KetoVerdict {
  if (netos == null || !Number.isFinite(netos)) return "sin-datos";
  const ratio = netos / objetivoDiario;
  if (ratio <= 0.3) return "compatible";
  if (ratio <= 0.6) return "con-ajustes";
  return "dificil";
}

function round1(n: number): number {
  return Math.round(n * 10) / 10;
}
