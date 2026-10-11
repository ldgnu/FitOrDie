import { NextResponse } from "next/server";
import { db } from "@/db";
import { diaryEntries } from "@/db/schema";
import { requireUser } from "@/lib/require-user";
import { getDaySummary } from "@/lib/day-summary";

export const MEALS = [
  "desayuno",
  "media-manana",
  "almuerzo",
  "merienda",
  "cena",
  "snack",
] as const;

// GET /api/diary?day=2026-10-11 → entradas + totales + keto del día.
export async function GET(req: Request) {
  const userId = await requireUser();
  if (!userId)
    return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  const day =
    new URL(req.url).searchParams.get("day") ??
    new Date().toISOString().slice(0, 10);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day))
    return NextResponse.json(
      { error: "day inválido (YYYY-MM-DD)" },
      { status: 400 },
    );
  return NextResponse.json(await getDaySummary(userId, day));
}

// POST /api/diary {consumedAt, meal, foodId?|recipeId?|customName?, qty, unit, notes?}
export async function POST(req: Request) {
  const userId = await requireUser();
  if (!userId)
    return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let body: Record<string, unknown>;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  const meal = String(body.meal ?? "");
  const qty = Number(body.qty);
  const unit = String(body.unit ?? "").slice(0, 16);
  const consumedAt = new Date(String(body.consumedAt ?? ""));
  if (!MEALS.includes(meal as never))
    return NextResponse.json({ error: "meal inválida" }, { status: 400 });
  if (!Number.isFinite(qty) || qty <= 0 || qty > 10000)
    return NextResponse.json({ error: "qty inválida" }, { status: 400 });
  if (!unit)
    return NextResponse.json({ error: "unit requerida" }, { status: 400 });
  if (Number.isNaN(consumedAt.getTime()))
    return NextResponse.json({ error: "consumedAt inválida" }, { status: 400 });
  if (!body.foodId && !body.recipeId && !body.customName)
    return NextResponse.json(
      { error: "Falta alimento, receta o nombre" },
      { status: 400 },
    );

  const inserted = await db
    .insert(diaryEntries)
    .values({
      userId,
      consumedAt,
      meal,
      foodId: (body.foodId as string) ?? null,
      recipeId: (body.recipeId as string) ?? null,
      customName: (body.customName as string)?.slice(0, 200) ?? null,
      qty: String(qty),
      unit,
      notes: (body.notes as string)?.slice(0, 1000) ?? null,
    })
    .returning({ id: diaryEntries.id });

  return NextResponse.json({ ok: true, id: inserted[0].id }, { status: 201 });
}
