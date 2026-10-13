// Delivery keto vía scraper local de Rappi (proyecto rappi-keto del usuario).
// Reglas: caché de 1h por query (el ToS prohíbe el hammering), nunca exponer
// el scraper directo, ordenar keto primero y ofertas primero.
export interface DeliveryItem {
  name: string;
  store: string;
  price: number;
  original_price: number | null;
  discount_pct: number | null;
  is_offer: boolean;
  is_keto: boolean;
  image: string | null;
  url: string | null;
}

interface CacheEntry {
  at: number;
  data: { query: string; timestamp: string; items: DeliveryItem[] };
}

const TTL = 3600_000; // 1 hora, mínimo razonable
const cache = new Map<string, CacheEntry>();
const BASE = process.env.RAPPI_URL ?? "http://127.0.0.1:8723";

export async function buscarDelivery(
  q: string,
  barato: boolean,
): Promise<{ query: string; timestamp: string; items: DeliveryItem[]; cached: boolean }> {
  const key = `${q}|${barato ? "b" : ""}`;
  const hit = cache.get(key);
  if (hit && Date.now() - hit.at < TTL) return { ...hit.data, cached: true };

  const r = await fetch(`${BASE}/?q=${encodeURIComponent(q)}`, {
    signal: AbortSignal.timeout(30000),
  });
  if (!r.ok) throw new Error(`scraper ${r.status}`);
  const data = await r.json();
  const items: DeliveryItem[] = ((data.items ?? []) as Record<string, unknown>[]).map((it) => ({
    name: String(it.name ?? "Sin nombre").slice(0, 200),
    store: String(it.store ?? ""),
    price: Number(it.price ?? 0),
    original_price: it.original_price == null ? null : Number(it.original_price),
    discount_pct: it.discount_pct == null ? null : Number(it.discount_pct),
    is_offer: !!it.is_offer,
    is_keto: !!it.is_keto,
    image: typeof it.image === "string" ? it.image : null,
    url: typeof it.url === "string" ? it.url : null,
  }));

  items.sort((a, b) => {
    if (barato && a.price !== b.price) return a.price - b.price;
    if (a.is_keto !== b.is_keto) return a.is_keto ? -1 : 1;
    if (a.is_offer !== b.is_offer) return a.is_offer ? -1 : 1;
    return a.price - b.price;
  });

  const out = {
    query: q,
    timestamp: String(data.timestamp ?? new Date().toISOString()),
    items,
  };
  cache.set(key, { at: Date.now(), data: out });
  return { ...out, cached: false };
}
