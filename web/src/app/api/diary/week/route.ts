import { NextResponse } from "next/server";
import { requireUser } from "@/lib/require-user";
import { getDaySummary } from "@/lib/day-summary";

// GET /api/diary/week?start=YYYY-MM-DD — 7 días desde start (default: hace 6 días).
// Resumen por día: kcal, proteína, netos, veredicto y cantidad de registros.
export async function GET(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let start = new URL(req.url).searchParams.get("start") ?? "";
  if (!/^\d{4}-\d{2}-\d{2}$/.test(start)) {
    const d = new Date();
    d.setUTCDate(d.getUTCDate() - 6);
    start = d.toISOString().slice(0, 10);
  }
  const days: Array<{
    day: string; kcal: number; protein: number; netos: number | null;
    verdict: string; entries: number; partial: boolean;
  }> = [];
  const base = new Date(`${start}T12:00:00Z`);
  for (let i = 0; i < 7; i++) {
    const dt = new Date(base);
    dt.setUTCDate(dt.getUTCDate() + i);
    const day = dt.toISOString().slice(0, 10);
    const s = await getDaySummary(userId, day);
    days.push({
      day,
      kcal: Math.round(s.totals.kcal),
      protein: Math.round(s.totals.protein),
      netos: s.keto.netos,
      verdict: s.keto.verdict,
      entries: s.entries.length,
      partial: s.partial,
    });
  }
  return NextResponse.json({ days });
}
