import { NextResponse } from "next/server";
import { and, eq } from "drizzle-orm";
import { db } from "@/db";
import { favorites, foods, recipeIngredients, recipeVotes, recipes } from "@/db/schema";
import { requireUser } from "@/lib/require-user";
import { ketoVerdict, netCarbs, scaleNutrients } from "@/lib/nutrition";
import { recipeNutrients } from "@/lib/day-summary";

// GET /api/recipes/:id — pública si la receta es pública.
export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const userId = await requireUser();
  const { id } = await params;
  const [r] = await db
    .select()
    .from(recipes)
    .where(eq(recipes.id, id))
    .limit(1);
  if (!r) return NextResponse.json({ error: "No existe" }, { status: 404 });
  if (!r.isPublic && r.createdBy !== userId)
    return NextResponse.json({ error: "No existe" }, { status: 404 });

  const ings = await db
    .select()
    .from(recipeIngredients)
    .where(eq(recipeIngredients.recipeId, id));

  const det = [];
  for (const ing of ings) {
    let name = ing.freeText ?? "Ingrediente";
    if (ing.foodId) {
      const [f] = await db.select().from(foods).where(eq(foods.id, ing.foodId)).limit(1);
      if (f) name = f.name;
    }
    det.push({ name, qty: ing.qty, unit: ing.unit, substitution: ing.substitution });
  }

  const per = await recipeNutrients(id, 1);
  const total = per ? scaleNutrients(per, 1, r.servings) : null;
  const netos = per ? netCarbs(per) : null;

  const votes = await db.select().from(recipeVotes).where(eq(recipeVotes.recipeId, id));
  const up = votes.filter((v) => v.vote === 1).length;
  const down = votes.filter((v) => v.vote === -1).length;
  let mine = 0;
  let favorite = false;
  if (userId) {
    const mv = votes.find((v) => v.userId === userId);
    if (mv) mine = mv.vote;
    const [f] = await db
      .select({ id: favorites.id })
      .from(favorites)
      .where(and(eq(favorites.userId, userId), eq(favorites.kind, "recipe"), eq(favorites.itemId, id)))
      .limit(1);
    favorite = !!f;
  }

  return NextResponse.json({
    recipe: r,
    ingredients: det,
    perPortion: per,
    total,
    netos,
    verdict: ketoVerdict(netos, 30),
    declarado: {
      kcal: r.declaredKcal, protein: r.declaredProtein, fat: r.declaredFat,
      carbs: r.declaredCarbs, fiber: r.declaredFiber, fuente: r.sourceSite,
    },
    votos: { up, down, mine },
    favorite,
  });
}
