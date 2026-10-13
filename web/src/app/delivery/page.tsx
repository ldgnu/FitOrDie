"use client";

import { useEffect, useState } from "react";
import type { DeliveryItem } from "@/lib/delivery";

const QUERIES = ["keto", "ensalada", "pollo", "carne", "sin tacc"];
const MEALS = ["desayuno", "media-manana", "almuerzo", "merienda", "cena", "snack"];
const input =
  "rounded border border-tinta/25 bg-white px-3 py-2 text-sm outline-none focus:border-terracota";
const fmt = new Intl.NumberFormat("es-AR", {
  style: "currency",
  currency: "ARS",
  maximumFractionDigits: 0,
});

export default function DeliveryPage() {
  const [q, setQ] = useState("keto");
  const [barato, setBarato] = useState(false);
  const [soloKeto, setSoloKeto] = useState(true);
  const [meal, setMeal] = useState("almuerzo");
  const [data, setData] = useState<{ items: DeliveryItem[]; cached: boolean } | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function buscar(query: string) {
    setBusy(true);
    setMsg(null);
    const r = await fetch(
      `/api/delivery?q=${encodeURIComponent(query)}${barato ? "&barato=1" : ""}`,
    );
    setBusy(false);
    if (!r.ok) {
      const d = await r.json().catch(() => null);
      setMsg(d?.error ?? "No se pudo buscar.");
      setData(null);
      return;
    }
    setData(await r.json());
  }

  useEffect(() => {
    let alive = true;
    (async () => {
      const r = await fetch(`/api/delivery?q=keto`);
      if (r.ok && alive) setData(await r.json());
    })();
    return () => {
      alive = false;
    };
  }, []);

  async function anotar(it: DeliveryItem) {
    const r = await fetch("/api/diary", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        consumedAt: new Date().toISOString(),
        meal,
        customName: `${it.name} (Rappi)`,
        qty: 1,
        unit: "porción",
        notes: `Rappi ${fmt.format(it.price)} · local ${it.store}${it.discount_pct ? ` · -${it.discount_pct}%` : ""}`,
      }),
    });
    setMsg(r.ok ? `Anotado: ${it.name}. Ojo: sin datos nutricionales.` : "No se pudo anotar.");
  }

  const items = (data?.items ?? []).filter((it) => !soloKeto || it.is_keto);

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-8 pt-6">
      <p className="text-sm tracking-wide text-oliva">DELIVERY KETO — CÓRDOBA</p>
      <h1 className="mt-2 font-display text-3xl">Pedir afuera</h1>
      <p className="mt-1 text-sm text-tinta/70">
        Rappi cerca tuyo, keto primero. Precios en ARS.
      </p>

      <div className="mt-4 flex gap-2">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && buscar(q)}
          placeholder="keto, ensalada, pollo…"
          className={`${input} flex-1`}
        />
        <button onClick={() => buscar(q)} disabled={busy} className="rounded bg-tinta px-4 py-2 text-sm text-crema disabled:opacity-60">
          {busy ? "…" : "Buscar"}
        </button>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-2 text-sm">
        {QUERIES.map((x) => (
          <button key={x} onClick={() => { setQ(x); buscar(x); }} className="rounded border border-tinta/25 px-3 py-1">
            {x}
          </button>
        ))}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-3 text-sm">
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={soloKeto} onChange={(e) => setSoloKeto(e.target.checked)} />
          Solo keto
        </label>
        <label className="flex items-center gap-1">
          <input type="checkbox" checked={barato} onChange={(e) => setBarato(e.target.checked)} />
          Barato primero
        </label>
        <select value={meal} onChange={(e) => setMeal(e.target.value)} className={input} aria-label="Comida para anotar">
          {MEALS.map((m) => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
      </div>

      {msg && <p className="mt-3 text-sm">{msg}</p>}
      {data && (
        <p className="mt-2 text-xs text-tinta/60">
          {items.length} opciones{data.cached ? " (caché, se actualiza cada hora)" : ""}
        </p>
      )}

      <ul className="mt-3 space-y-3">
        {items.map((it, i) => (
          <li key={`${it.store}-${it.name}-${i}`} className="flex gap-3 rounded border border-tinta/15 bg-panel p-3">
            {it.image && (
              <img src={it.image} alt="" loading="lazy" className="h-16 w-16 shrink-0 rounded object-cover" />
            )}
            <div className="min-w-0 flex-1">
              <p className="font-bold leading-tight">
                {it.name}{" "}
                {!it.is_keto && <span className="text-xs font-normal text-terracota">(revisar keto)</span>}
              </p>
              <p className="text-sm">
                <strong>{fmt.format(it.price)}</strong>
                {it.is_offer && it.discount_pct ? (
                  <span className="text-oliva"> −{it.discount_pct}%</span>
                ) : null}
              </p>
              <div className="mt-1 flex gap-3 text-sm">
                {it.url && (
                  <a href={it.url} target="_blank" rel="noopener noreferrer" className="underline">
                    Ver en Rappi
                  </a>
                )}
                <button onClick={() => anotar(it)} className="underline">
                  Anotar
                </button>
              </div>
            </div>
          </li>
        ))}
      </ul>
      {data && items.length === 0 && (
        <p className="mt-4 text-sm">Nada keto por acá. Probá otra búsqueda.</p>
      )}
    </main>
  );
}
