import { NextResponse } from "next/server";
import { eq, isNull, or } from "drizzle-orm";
import { db } from "@/db";
import { foods, recipes } from "@/db/schema";
import { requireUser } from "@/lib/require-user";
import { parseMealText } from "@/lib/parse-food";

// POST /api/diary/parse {text} → interpreta sin guardar.
// El cliente muestra lo entendido + lo asumido y recién guarda al confirmar.
export async function POST(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let body: { text?: string };
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  const text = String(body.text ?? "").slice(0, 500).trim();
  if (!text) return NextResponse.json({ error: "Escribí algo primero" }, { status: 400 });

  const foodRows = await db
    .select({ id: foods.id, name: foods.name })
    .from(foods)
    .where(or(isNull(foods.createdBy), eq(foods.createdBy, userId)));
  const recRows = await db
    .select({ id: recipes.id, title: recipes.title })
    .from(recipes)
    .where(eq(recipes.isPublic, true));

  const refs = [
    ...foodRows.map((f) => ({ id: f.id, name: f.name, kind: "food" as const })),
    ...recRows.map((r) => ({ id: r.id, name: r.title, kind: "recipe" as const })),
  ];
  const today = new Date().toISOString().slice(0, 10);
  return NextResponse.json(parseMealText(text, refs, today));
}
