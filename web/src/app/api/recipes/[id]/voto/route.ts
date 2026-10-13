import { NextResponse } from "next/server";
import { eq } from "drizzle-orm";
import { db } from "@/db";
import { recipeVotes, recipes } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

// POST /api/recipes/:id/voto {vote: 1|-1|0} — me gusta / no me gusta / quitar.
export async function POST(
  req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const { id } = await params;

  let body: { vote?: number };
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  if (body.vote !== 1 && body.vote !== -1 && body.vote !== 0)
    return NextResponse.json({ error: "vote inválido" }, { status: 400 });

  const [r] = await db.select({ id: recipes.id }).from(recipes).where(eq(recipes.id, id)).limit(1);
  if (!r) return NextResponse.json({ error: "No existe" }, { status: 404 });

  const [prev] = await db
    .select()
    .from(recipeVotes)
    .where(eq(recipeVotes.userId, userId));
  const mine = prev && (prev as { recipeId: string }).recipeId === id ? prev : null;

  if (body.vote === 0) {
    if (mine) await db.delete(recipeVotes).where(eq(recipeVotes.id, (mine as { id: string }).id));
  } else if (mine) {
    await db
      .update(recipeVotes)
      .set({ vote: body.vote })
      .where(eq(recipeVotes.id, (mine as { id: string }).id));
  } else {
    await db.insert(recipeVotes).values({ userId, recipeId: id, vote: body.vote });
  }

  const all = await db.select().from(recipeVotes).where(eq(recipeVotes.recipeId, id));
  const up = all.filter((v) => v.vote === 1).length;
  const down = all.filter((v) => v.vote === -1).length;
  return NextResponse.json({ ok: true, up, down, mine: body.vote });
}
