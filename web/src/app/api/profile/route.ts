import { NextResponse } from "next/server";
import { eq } from "drizzle-orm";
import { db } from "@/db";
import { profiles } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

const CRUDO = ["suave", "picante", "sin-filtro"] as const;

// GET /api/profile — perfil propio (null si no existe).
export async function GET() {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const [p] = await db.select().from(profiles).where(eq(profiles.userId, userId)).limit(1);
  return NextResponse.json({ profile: p ?? null });
}

// PATCH /api/profile — actualiza objetivos y preferencias propios.
export async function PATCH(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let body: Record<string, unknown>;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }

  const patch: Partial<typeof profiles.$inferInsert> = {};
  const num = (v: unknown, min: number, max: number): number | null | undefined => {
    if (v === undefined || v === null) return undefined;
    const n = Number(v);
    if (!Number.isFinite(n) || n < min || n > max) return null;
    return Math.round(n);
  };

  const kcal = num(body.kcalTarget, 800, 15000);
  const prot = num(body.proteinTarget, 0, 1000);
  const nc = num(body.netCarbTarget, 0, 500);
  if (kcal === null || prot === null || nc === null)
    return NextResponse.json({ error: "Objetivos fuera de rango" }, { status: 400 });
  if (kcal !== undefined) patch.kcalTarget = kcal;
  if (prot !== undefined) patch.proteinTarget = prot;
  if (nc !== undefined) patch.netCarbTarget = nc;

  if (body.crudeza !== undefined) {
    if (!CRUDO.includes(body.crudeza as never))
      return NextResponse.json({ error: "crudeza inválida" }, { status: 400 });
    patch.crudeza = body.crudeza as string;
  }
  if (body.units !== undefined) {
    if (body.units !== "metric" && body.units !== "imperial")
      return NextResponse.json({ error: "units inválidas" }, { status: 400 });
    patch.units = body.units;
  }
  if (body.displayName !== undefined) {
    const s = String(body.displayName).slice(0, 100) || null;
    patch.displayName = s;
  }
  if (body.timezone !== undefined) {
    const tz = String(body.timezone);
    try {
      new Intl.DateTimeFormat("es", { timeZone: tz });
    } catch {
      return NextResponse.json({ error: "timezone inválida" }, { status: 400 });
    }
    patch.timezone = tz;
  }
  for (const k of ["prefs", "allergies", "dislikes"] as const) {
    if (body[k] !== undefined) {
      if (!Array.isArray(body[k]) || body[k].length > 50)
        return NextResponse.json({ error: `${k} inválido` }, { status: 400 });
      patch[k] = body[k].map((s: unknown) => String(s).slice(0, 100));
    }
  }

  if (Object.keys(patch).length === 0)
    return NextResponse.json({ error: "Nada para actualizar" }, { status: 400 });

  const [p] = await db
    .insert(profiles)
    .values({ userId, ...patch })
    .onConflictDoUpdate({ target: profiles.userId, set: patch })
    .returning();
  return NextResponse.json({ ok: true, profile: p });
}
