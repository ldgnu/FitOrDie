import { NextResponse } from "next/server";
import { and, ilike, lte, or, eq } from "drizzle-orm";
import { db } from "@/db";
import { recipes } from "@/db/schema";
import { requireUser } from "@/lib/require-user";
import { ketoVerdict, netCarbs } from "@/lib/nutrition";
import { recipeNutrients } from "@/lib/day-summary";

// GET /api/recipes?q=&keto=1&maxMinutes=15 — pública (propias si hay sesión).
export async function GET(req: Request) {
  const userId = await requireUser();
  const sp = new URL(req.url).searchParams;
  const q = sp.get("q")?.trim() ?? "";
  const ketoOnly = sp.get("keto") === "1";
  const maxMinutes = Number(sp.get("maxMinutes") ?? NaN);
  const categoria = sp.get("categoria") ?? "";
  const CATS = ["desayuno", "almuerzo", "merienda", "cena", "bajon"];

  const rows = await db
    .select()
    .from(recipes)
    .where(
      and(
        userId
          ? or(eq(recipes.isPublic, true), eq(recipes.createdBy, userId))
          : eq(recipes.isPublic, true),
        q ? ilike(recipes.title, `%${q}%`) : undefined,
        Number.isFinite(maxMinutes) ? lte(recipes.minutes, maxMinutes) : undefined,
        categoria && CATS.includes(categoria) ? eq(recipes.categoria, categoria) : undefined,
      ),
    )
    .limit(100);

  const out = [];
  for (const r of rows) {
    const per = await recipeNutrients(r.id, 1);
    const netos = per ? netCarbs(per) : null;
    const verdict = ketoVerdict(netos, 30);
    if (ketoOnly && verdict !== "compatible") continue;
    out.push({
      id: r.id, title: r.title, description: r.description,
      servings: r.servings, minutes: r.minutes, difficulty: r.difficulty,
      photoUrl: r.photoUrl, categoria: r.categoria,
      sourceSite: r.sourceSite, sourceUrl: r.sourceUrl, sourceLikes: r.sourceLikes,
      declarado: {
        kcal: r.declaredKcal, protein: r.declaredProtein,
        fat: r.declaredFat, carbs: r.declaredCarbs, fiber: r.declaredFiber,
      },
      perPortion: per, netos, verdict,
    });
  }
  return NextResponse.json({ recipes: out });
}
