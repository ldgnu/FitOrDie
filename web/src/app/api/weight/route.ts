import { NextResponse } from "next/server";
import { asc, eq } from "drizzle-orm";
import { db } from "@/db";
import { weights } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

// GET /api/weight → historial ascendente (máx 200).
export async function GET() {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const rows = await db
    .select()
    .from(weights)
    .where(eq(weights.userId, userId))
    .orderBy(asc(weights.measuredAt))
    .limit(200);
  return NextResponse.json({ weights: rows });
}

// POST /api/weight {measuredAt?, weightKg, note?}
export async function POST(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let body: Record<string, unknown>;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  const weightKg = Number(body.weightKg);
  if (!Number.isFinite(weightKg) || weightKg < 20 || weightKg > 400)
    return NextResponse.json({ error: "weightKg inválido" }, { status: 400 });
  const measuredAt = body.measuredAt
    ? new Date(String(body.measuredAt))
    : new Date();
  if (Number.isNaN(measuredAt.getTime()))
    return NextResponse.json({ error: "measuredAt inválida" }, { status: 400 });

  const inserted = await db
    .insert(weights)
    .values({
      userId,
      measuredAt,
      weightKg: String(weightKg),
      note: (body.note as string)?.slice(0, 500) ?? null,
    })
    .returning({ id: weights.id });
  return NextResponse.json({ ok: true, id: inserted[0].id }, { status: 201 });
}
