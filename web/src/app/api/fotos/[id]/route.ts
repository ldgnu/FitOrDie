import { NextResponse } from "next/server";
import { readFile } from "fs/promises";
import { join } from "path";
import { eq } from "drizzle-orm";
import { db } from "@/db";
import { mealPhotos } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

// GET /api/fotos/:id — foto propia, jamás pública.
export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  const { id } = await params;
  const [p] = await db.select().from(mealPhotos).where(eq(mealPhotos.id, id)).limit(1);
  if (!p || p.userId !== userId)
    return NextResponse.json({ error: "No encontrada" }, { status: 404 });
  try {
    const buf = await readFile(join(process.env.UPLOADS_DIR ?? "./uploads", p.filename));
    return new Response(new Uint8Array(buf), {
      headers: {
        "Content-Type": p.mime,
        "Cache-Control": "private, max-age=3600",
      },
    });
  } catch {
    return NextResponse.json({ error: "No encontrada" }, { status: 404 });
  }
}
