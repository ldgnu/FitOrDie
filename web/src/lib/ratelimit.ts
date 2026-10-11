// Rate limit en memoria (una instancia). Suficiente para MVP + dev;
// en multi-instancia se reemplaza por Redis. Devuelve true si pasa.
const hits = new Map<string, { n: number; reset: number }>();

export function rateLimit(
  key: string,
  limit = 20,
  windowMs = 60_000,
): boolean {
  const now = Date.now();
  const h = hits.get(key);
  if (!h || now > h.reset) {
    hits.set(key, { n: 1, reset: now + windowMs });
    return true;
  }
  h.n += 1;
  return h.n <= limit;
}

export function clientIp(req: Request): string {
  return (
    req.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ?? "local"
  );
}
