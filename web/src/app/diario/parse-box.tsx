"use client";

import { useState } from "react";
import type { ParseResult } from "@/lib/parse-food";

const MEALS = ["desayuno", "media-manana", "almuerzo", "merienda", "cena", "snack"];
const input =
  "rounded border border-tinta/25 bg-white px-3 py-2 text-sm outline-none focus:border-terracota";

// Caja "escribilo": texto libre → preview con lo asumido → confirmar guarda.
export default function ParseBox({ onSaved }: { onSaved: () => void }) {
  const [text, setText] = useState("");
  const [res, setRes] = useState<ParseResult | null>(null);
  const [meal, setMeal] = useState("almuerzo");
  const [qty, setQty] = useState<Record<number, string>>({});
  const [pick, setPick] = useState<Record<number, string>>({});
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function interpretar(ev: React.FormEvent) {
    ev.preventDefault();
    setMsg(null);
    const r = await fetch("/api/diary/parse", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    });
    if (!r.ok) {
      setMsg("No pude interpretar eso. Probá más simple.");
      return;
    }
    const d: ParseResult = await r.json();
    setRes(d);
    if (d.meal) setMeal(d.meal);
    const q: Record<number, string> = {};
    const p: Record<number, string> = {};
    d.items.forEach((it, i) => {
      q[i] = String(it.qty);
      if (it.candidates?.length) p[i] = it.foodId ?? it.recipeId ?? "";
    });
    setQty(q);
    setPick(p);
  }

  async function confirmar() {
    if (!res) return;
    setBusy(true);
    setMsg(null);
    for (let i = 0; i < res.items.length; i++) {
      const it = res.items[i];
      const n = Number(qty[i]);
      if (!Number.isFinite(n) || n <= 0) {
        setMsg(`Cantidad inválida en "${it.label}".`);
        setBusy(false);
        return;
      }
      const sel = pick[i];
      const foodId = it.kind === "food" ? sel || it.foodId : null;
      const recipeId = it.kind === "recipe" ? sel || it.recipeId : null;
      const body: Record<string, unknown> = {
        consumedAt: new Date().toISOString(),
        meal,
        qty: n,
        unit: it.kind === "recipe" ? "porción" : it.unit,
      };
      if (foodId) body.foodId = foodId;
      else if (recipeId) body.recipeId = recipeId;
      else body.customName = it.label;
      const r = await fetch("/api/diary", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!r.ok) {
        setMsg(`No se pudo guardar "${it.label}".`);
        setBusy(false);
        return;
      }
    }
    setBusy(false);
    setRes(null);
    setText("");
    onSaved();
  }

  return (
    <section className="mt-6 rounded border border-tinta/15 bg-white/50 p-4">
      <h2 className="font-display text-2xl">Escribilo como lo dirías</h2>
      <form onSubmit={interpretar} className="mt-2 space-y-2">
        <textarea
          value={text} onChange={(e) => setText(e.target.value)}
          rows={2} placeholder="Hoy desayuné 4 huevos con una palta chica…"
          className={`${input} w-full`}
        />
        <button className="rounded bg-oliva px-4 py-2 text-sm text-crema">
          Interpretar
        </button>
      </form>

      {res && (
        <div className="mt-3 space-y-2 text-sm">
          <div className="flex items-center gap-2">
            <span>Comida:</span>
            <select value={meal} onChange={(e) => setMeal(e.target.value)} className={input}>
              {MEALS.map((m) => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </div>
          {res.items.map((it, i) => (
            <div key={i} className="rounded border border-tinta/15 bg-crema p-2">
              <p>
                <strong>{it.label}</strong> →{" "}
                {it.kind === "custom"
                  ? "no lo conozco (va sin cálculo)"
                  : it.candidates?.length ? (
                    <select
                      value={pick[i] ?? ""} onChange={(e) => setPick((s) => ({ ...s, [i]: e.target.value }))}
                      className={input}
                    >
                      {it.candidates.map((c) => (
                        <option key={c.id} value={c.id}>{c.name}</option>
                      ))}
                    </select>
                  ) : (
                    (it.kind === "food" ? "" : "receta ")
                  )}
                {it.kind !== "custom" && !it.candidates?.length && (
                  <span>{/* nombre ya mostrado abajo */}</span>
                )}
              </p>
              {it.kind !== "custom" && (
                <p className="text-tinta/70">
                  {(it.candidates?.length
                    ? it.candidates.find((c) => c.id === (pick[i] || it.foodId || it.recipeId))?.name
                    : null) ?? ""}
                </p>
              )}
              <div className="mt-1 flex items-center gap-2">
                <input
                  value={qty[i] ?? ""} onChange={(e) => setQty((s) => ({ ...s, [i]: e.target.value }))}
                  inputMode="decimal" aria-label="Cantidad" className="w-24 rounded border border-tinta/25 bg-white px-2 py-1"
                />
                <span>{it.unit}</span>
              </div>
              {it.assumed.map((a, j) => (
                <p key={j} className="text-terracota">⚠ {a}</p>
              ))}
            </div>
          ))}
          {res.questions.map((qq, i) => (
            <p key={i} className="text-tinta/80">❓ {qq}</p>
          ))}
          <button onClick={confirmar} disabled={busy} className="rounded bg-tinta px-4 py-2 text-sm text-crema disabled:opacity-60">
            {busy ? "Guardando…" : `Confirmar y guardar (${res.items.length})`}
          </button>
          {msg && <p className="text-terracota">{msg}</p>}
        </div>
      )}
    </section>
  );
}
