import { NextResponse } from "next/server";
import { and, eq } from "drizzle-orm";
import { db } from "@/db";
import { favorites, foods, recipes } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

// GET /api/favoritos?kind=recipe|food — favoritos propios con títulos.
export async function GET(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const kind = new URL(req.url).searchParams.get("kind");
  const rows = await db
    .select()
    .from(favorites)
    .where(
      kind ? and(eq(favorites.userId, userId), eq(favorites.kind, kind)) : eq(favorites.userId, userId),
    );
  const out = [];
  for (const f of rows) {
    if (f.kind === "recipe") {
      const [r] = await db.select().from(recipes).where(eq(recipes.id, f.itemId)).limit(1);
      if (r) out.push({ kind: f.kind, itemId: f.itemId, title: r.title, photoUrl: r.photoUrl, createdAt: f.createdAt });
    } else {
      const [fd] = await db.select().from(foods).where(eq(foods.id, f.itemId)).limit(1);
      if (fd) out.push({ kind: f.kind, itemId: f.itemId, title: fd.name, photoUrl: null, createdAt: f.createdAt });
    }
  }
  return NextResponse.json({ favoritos: out });
}

// POST /api/favoritos {kind, itemId} — agregar (idempotente).
export async function POST(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  let body: { kind?: string; itemId?: string };
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  if ((body.kind !== "food" && body.kind !== "recipe") || !body.itemId)
    return NextResponse.json({ error: "Datos inválidos" }, { status: 400 });

  if (body.kind === "recipe") {
    const [r] = await db.select({ id: recipes.id, isPublic: recipes.isPublic, createdBy: recipes.createdBy }).from(recipes).where(eq(recipes.id, body.itemId)).limit(1);
    if (!r || (!r.isPublic && r.createdBy !== userId))
      return NextResponse.json({ error: "No existe" }, { status: 404 });
  } else {
    const [f] = await db.select({ id: foods.id }).from(foods).where(eq(foods.id, body.itemId)).limit(1);
    if (!f) return NextResponse.json({ error: "No existe" }, { status: 404 });
  }
  const [ex] = await db
    .select({ id: favorites.id })
    .from(favorites)
    .where(and(eq(favorites.userId, userId), eq(favorites.kind, body.kind), eq(favorites.itemId, body.itemId)))
    .limit(1);
  if (!ex) await db.insert(favorites).values({ userId, kind: body.kind, itemId: body.itemId });
  return NextResponse.json({ ok: true }, { status: 201 });
}

// DELETE /api/favoritos?kind=&itemId= — quitar.
export async function DELETE(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const sp = new URL(req.url).searchParams;
  const kind = sp.get("kind");
  const itemId = sp.get("itemId");
  if ((kind !== "food" && kind !== "recipe") || !itemId)
    return NextResponse.json({ error: "Datos inválidos" }, { status: 400 });
  await db
    .delete(favorites)
    .where(and(eq(favorites.userId, userId), eq(favorites.kind, kind), eq(favorites.itemId, itemId)));
  return NextResponse.json({ ok: true });
}
