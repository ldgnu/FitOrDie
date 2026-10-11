// Resumen del día: usado por la API y el dashboard (misma cuenta, sin duplicar).
import { and, eq, gte, isNull, lt } from "drizzle-orm";
import { db } from "@/db";
import {
  diaryEntries,
  foods,
  profiles,
  recipeIngredients,
  recipes,
} from "@/db/schema";
import {
  ketoVerdict,
  netCarbs,
  scaleNutrients,
  sumNutrients,
  type KetoVerdict,
  type Nutrients,
} from "./nutrition";

export type FoodRow = typeof foods.$inferSelect;
export type DiaryRow = typeof diaryEntries.$inferSelect;

export interface EntryWithN extends DiaryRow {
  nutrients: Nutrients | null;
  label: string;
}

export interface DaySummary {
  day: string;
  entries: EntryWithN[];
  totals: Nutrients;
  partial: boolean; // true si alguna entrada no pudo calcularse
  keto: { netos: number | null; objetivo: number; verdict: KetoVerdict };
  kcalTarget: number | null;
  proteinTarget: number | null;
}

export function foodNutrients(
  entry: { qty: string; unit: string },
  food?: FoodRow,
): Nutrients | null {
  if (!food) return null;
  if (entry.unit !== "g" && entry.unit !== "ml") return null;
  const raw = {
    kcal: food.kcal100,
    protein: food.protein100,
    fat: food.fat100,
    carbs: food.carbs100,
    fiber: food.fiber100,
  };
  if (Object.values(raw).some((v) => v == null)) return null;
  return scaleNutrients(
    {
      kcal: Number(raw.kcal),
      protein: Number(raw.protein),
      fat: Number(raw.fat),
      carbs: Number(raw.carbs),
      fiber: Number(raw.fiber),
    },
    100,
    Number(entry.qty),
  );
}

export async function recipeNutrients(
  recipeId: string,
  portions: number,
): Promise<Nutrients | null> {
  const [recipe] = await db
    .select()
    .from(recipes)
    .where(eq(recipes.id, recipeId))
    .limit(1);
  if (!recipe?.servings) return null;
  const ings = await db
    .select()
    .from(recipeIngredients)
    .where(eq(recipeIngredients.recipeId, recipeId));
  if (ings.length === 0) return null;
  const parts: Nutrients[] = [];
  for (const ing of ings) {
    if (!ing.foodId || (ing.unit !== "g" && ing.unit !== "ml")) return null;
    const [food] = await db
      .select()
      .from(foods)
      .where(eq(foods.id, ing.foodId))
      .limit(1);
    const n = foodNutrients({ qty: String(ing.qty), unit: ing.unit }, food);
    if (!n) return null;
    parts.push(n);
  }
  return scaleNutrients(sumNutrients(parts), 1, portions / recipe.servings);
}

export async function getDaySummary(
  userId: string,
  day: string,
): Promise<DaySummary> {
  const from = new Date(`${day}T00:00:00Z`);
  const to = new Date(from);
  to.setUTCDate(to.getUTCDate() + 1);

  const entries = await db
    .select()
    .from(diaryEntries)
    .where(
      and(
        eq(diaryEntries.userId, userId),
        isNull(diaryEntries.deletedAt),
        gte(diaryEntries.consumedAt, from),
        lt(diaryEntries.consumedAt, to),
      ),
    )
    .orderBy(diaryEntries.consumedAt);

  const foodMap = new Map<string, FoodRow>();
  const recipeNames = new Map<string, string>();
  for (const e of entries) {
    if (e.foodId && !foodMap.has(e.foodId)) {
      const [f] = await db
        .select()
        .from(foods)
        .where(eq(foods.id, e.foodId))
        .limit(1);
      if (f) foodMap.set(e.foodId, f);
    }
    if (e.recipeId && !recipeNames.has(e.recipeId)) {
      const [r] = await db
        .select()
        .from(recipes)
        .where(eq(recipes.id, e.recipeId))
        .limit(1);
      if (r) recipeNames.set(e.recipeId, r.title);
    }
  }

  const withN: EntryWithN[] = [];
  let partial = false;
  for (const e of entries) {
    let n: Nutrients | null = null;
    let label = e.customName ?? "Sin nombre";
    if (e.foodId) {
      n = foodNutrients(e, foodMap.get(e.foodId));
      label = foodMap.get(e.foodId)?.name ?? label;
    } else if (e.recipeId) {
      n = await recipeNutrients(e.recipeId, Number(e.qty));
      label = recipeNames.get(e.recipeId) ?? label;
    }
    if (!n) partial = true;
    withN.push({ ...e, nutrients: n, label });
  }

  const totals = sumNutrients(
    withN.map((e) => e.nutrients).filter((n): n is Nutrients => n !== null),
  );
  const netos = netCarbs(totals);
  const [profile] = await db
    .select()
    .from(profiles)
    .where(eq(profiles.userId, userId))
    .limit(1);
  const objetivo = profile?.netCarbTarget ?? 30;

  return {
    day,
    entries: withN,
    totals,
    partial,
    keto: { netos, objetivo, verdict: ketoVerdict(netos, objetivo) },
    kcalTarget: profile?.kcalTarget ?? null,
    proteinTarget: profile?.proteinTarget ?? null,
  };
}
