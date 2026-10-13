import { NextResponse } from "next/server";
import { requireUser } from "@/lib/require-user";
import { buscarDelivery } from "@/lib/delivery";

// GET /api/delivery?q=keto&barato=1 — delivery keto (Rappi Córdoba).
// Privado y cacheado 1h. Si el scraper está caído: 503 (no se reintenta en bucle).
export async function GET(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  const sp = new URL(req.url).searchParams;
  const q = sp.get("q")?.trim().slice(0, 60) || "keto";
  const barato = sp.get("barato") === "1";

  try {
    return NextResponse.json(await buscarDelivery(q, barato));
  } catch {
    return NextResponse.json(
      { error: "Scraper no disponible. Levantalo con: python rappi_keto.py --cookies cookies.json --serve" },
      { status: 503 },
    );
  }
}
