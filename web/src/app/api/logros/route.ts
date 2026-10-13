import { NextResponse } from "next/server";
import { requireUser } from "@/lib/require-user";
import { resumenJuego } from "@/lib/gamificacion";

// GET /api/logros → XP, nivel, racha y logros (desbloquea los nuevos).
export async function GET() {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });
  return NextResponse.json(await resumenJuego(userId));
}
