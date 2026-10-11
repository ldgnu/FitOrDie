import { NextResponse } from "next/server";
import { and, eq, isNull } from "drizzle-orm";
import { db } from "@/db";
import { auditLog, diaryEntries, entryEdits } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

async function owned(userId: string, id: string) {
  const [e] = await db
    .select()
    .from(diaryEntries)
    .where(
      and(
        eq(diaryEntries.id, id),
        eq(diaryEntries.userId, userId),
        isNull(diaryEntries.deletedAt),
      ),
    )
    .limit(1);
  return e ?? null;
}

// PATCH /api/diary/:id {qty?, notes?} — corrige con historial (sin borrado silencioso).
export async function PATCH(
  req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const { id } = await params;
  const entry = await owned(userId, id);
  if (!entry) return NextResponse.json({ error: "No encontrada" }, { status: 404 });

  let body: Record<string, unknown>;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  const diff: Record<string, [unknown, unknown]> = {};
  const patch: { qty?: string; notes?: string | null } = {};
  if (body.qty !== undefined) {
    const qty = Number(body.qty);
    if (!Number.isFinite(qty) || qty <= 0 || qty > 10000)
      return NextResponse.json({ error: "qty inválida" }, { status: 400 });
    if (String(qty) !== entry.qty) {
      diff.qty = [entry.qty, String(qty)];
      patch.qty = String(qty);
    }
  }
  if (body.notes !== undefined) {
    const notes = String(body.notes).slice(0, 1000) || null;
    if (notes !== entry.notes) {
      diff.notes = [entry.notes, notes];
      patch.notes = notes;
    }
  }
  if (Object.keys(diff).length === 0)
    return NextResponse.json({ ok: true, sinCambios: true });

  await db.insert(entryEdits).values({ entryId: id, changedBy: userId, diff });
  await db.update(diaryEntries).set(patch).where(eq(diaryEntries.id, id));
  return NextResponse.json({ ok: true });
}

// DELETE /api/diary/:id — soft delete + auditoría.
export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const { id } = await params;
  const entry = await owned(userId, id);
  if (!entry) return NextResponse.json({ error: "No encontrada" }, { status: 404 });

  await db
    .update(diaryEntries)
    .set({ deletedAt: new Date() })
    .where(eq(diaryEntries.id, id));
  await db
    .insert(auditLog)
    .values({ userId, action: "diary-delete", meta: { entryId: id } });
  return NextResponse.json({ ok: true });
}
