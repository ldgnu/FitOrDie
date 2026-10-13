"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";
import ParseBox from "./parse-box";

interface Nutrients {
  kcal: number;
  protein: number;
  fat: number;
  carbs: number;
  fiber: number;
}
interface Entry {
  id: string;
  meal: string;
  label: string;
  qty: string;
  unit: string;
  notes: string | null;
  nutrients: Nutrients | null;
}
interface DayData {
  day: string;
  entries: Entry[];
  totals: Nutrients;
  partial: boolean;
  keto: { netos: number | null; objetivo: number; verdict: string };
}
interface Food {
  id: string;
  name: string;
  kcal100: string | null;
  source: string;
}

const MEALS: Array<{ v: string; label: string }> = [
  { v: "desayuno", label: "Desayuno" },
  { v: "media-manana", label: "Media mañana" },
  { v: "almuerzo", label: "Almuerzo" },
  { v: "merienda", label: "Merienda" },
  { v: "cena", label: "Cena" },
  { v: "snack", label: "Snack / antojo" },
];

const VERDICT_TXT: Record<string, string> = {
  compatible: "Compatible",
  "con-ajustes": "Con ajustes",
  dificil: "Difícil de encajar",
  "sin-datos": "Sin datos",
};

const btn = "rounded bg-tinta px-4 py-2 text-sm text-crema disabled:opacity-60";
const input =
  "w-full rounded border border-tinta/25 bg-panel px-3 py-2 text-sm outline-none focus:border-terracota";

function DiarioInner() {
  const sp = useSearchParams();
  const param = sp.get("meal");
  const [day, setDay] = useState(() => new Date().toISOString().slice(0, 10));
  const [data, setData] = useState<DayData | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [results, setResults] = useState<Food[]>([]);
  const [sel, setSel] = useState<Food | null>(null);
  const [meal, setMeal] = useState(
    param && MEALS.some((x) => x.v === param) ? param : "almuerzo",
  );
  const [qty, setQty] = useState("150");
  const [unit, setUnit] = useState("g");
  const [customName, setCustomName] = useState("");
  const [busy, setBusy] = useState(false);
  const [editQty, setEditQty] = useState<Record<string, string>>({});
  const [weight, setWeight] = useState("");

  const load = useCallback(async () => {
    const r = await fetch(`/api/diary?day=${day}`);
    if (r.ok) setData(await r.json());
  }, [day]);

  useEffect(() => {
    let alive = true;
    (async () => {
      const r = await fetch(`/api/diary?day=${day}`);
      if (r.ok && alive) setData(await r.json());
    })();
    return () => {
      alive = false;
    };
  }, [day]);

  useEffect(() => {
    const query = q.trim();
    if (query.length < 2) return;
    let alive = true;
    const t = setTimeout(async () => {
      const r = await fetch(`/api/foods?q=${encodeURIComponent(query)}`);
      if (r.ok && alive) setResults((await r.json()).foods);
    }, 300);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [q]);

  function shift(d: number) {
    const dt = new Date(`${day}T12:00:00`);
    dt.setDate(dt.getDate() + d);
    setDay(dt.toISOString().slice(0, 10));
  }

  async function add(ev: React.FormEvent) {
    ev.preventDefault();
    setMsg(null);
    const ref = sel
      ? { foodId: sel.id }
      : customName.trim()
        ? { customName: customName.trim() }
        : null;
    if (!ref) {
      setMsg("Elegí un alimento o escribí un nombre.");
      return;
    }
    const n = Number(qty);
    if (!Number.isFinite(n) || n <= 0) {
      setMsg("Cantidad inválida.");
      return;
    }
    setBusy(true);
    const r = await fetch("/api/diary", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        consumedAt: new Date(`${day}T12:00:00`).toISOString(),
        meal,
        qty: n,
        unit,
        ...ref,
      }),
    });
    setBusy(false);
    if (!r.ok) {
      const d = await r.json().catch(() => null);
      setMsg(d?.error ?? "No se pudo guardar.");
      return;
    }
    setSel(null);
    setQ("");
    setCustomName("");
    load();
  }

  async function del(id: string) {
    await fetch(`/api/diary/${id}`, { method: "DELETE" });
    load();
  }

  async function saveQty(id: string) {
    const v = Number(editQty[id]);
    if (!Number.isFinite(v) || v <= 0) return;
    await fetch(`/api/diary/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ qty: v }),
    });
    load();
  }

  async function addWeight(ev: React.FormEvent) {
    ev.preventDefault();
    const v = Number(weight);
    if (!Number.isFinite(v) || v < 20 || v > 400) {
      setMsg("Peso inválido.");
      return;
    }
    await fetch("/api/weight", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ weightKg: v }),
    });
    setWeight("");
    setMsg("Peso anotado.");
  }

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-16 pt-6">
      <Link href="/dashboard" className="text-sm underline">
        ← Mi día
      </Link>
      <div className="mt-2 flex items-center gap-2">
        <button onClick={() => shift(-1)} className={btn}>←</button>
        <input
          type="date" value={day}
          onChange={(e) => e.target.value && setDay(e.target.value)}
          className={input}
        />
        <button onClick={() => shift(1)} className={btn}>→</button>
      </div>

      <ParseBox onSaved={load} />

      {data && (
        <section className="mt-4 rounded border border-tinta/15 bg-panel/50 p-4 text-sm">
          <p>
            <strong>{Math.round(data.totals.kcal)} kcal</strong> · P{" "}
            {Math.round(data.totals.protein)} · G{" "}
            {Math.round(data.totals.fat)} · C{" "}
            {Math.round(data.totals.carbs)} · netos{" "}
            <strong>{data.keto.netos ?? "—"}</strong> (
            {VERDICT_TXT[data.keto.verdict]})
          </p>
          {data.partial && (
            <p className="text-terracota">
              Total parcial: hay entradas sin datos completos.
            </p>
          )}
        </section>
      )}

      <section className="mt-6">
        <h2 className="font-display text-2xl">Anotar comida</h2>
        <form onSubmit={add} className="mt-3 space-y-3">
          <div className="grid grid-cols-2 gap-2">
            <select value={meal} onChange={(e) => setMeal(e.target.value)} className={input}>
              {MEALS.map((m) => (
                <option key={m.v} value={m.v}>{m.label}</option>
              ))}
            </select>
            <div className="flex gap-2">
              <input value={qty} onChange={(e) => setQty(e.target.value)} inputMode="decimal" className={input} aria-label="Cantidad" />
              <select value={unit} onChange={(e) => setUnit(e.target.value)} className={input} aria-label="Unidad">
                <option value="g">g</option>
                <option value="ml">ml</option>
                <option value="unidad">unid.</option>
              </select>
            </div>
          </div>
          <input
            value={q} onChange={(e) => { const v = e.target.value; setQ(v); setSel(null); if (v.trim().length < 2) setResults([]); }}
            placeholder="Buscar alimento (ej: pollo)"
            className={input}
          />
          {results.length > 0 && !sel && (
            <ul className="divide-y divide-tinta/10 rounded border border-tinta/15 bg-panel">
              {results.map((f) => (
                <li key={f.id}>
                  <button
                    type="button"
                    onClick={() => { setSel(f); setResults([]); }}
                    className="w-full px-3 py-2 text-left text-sm"
                  >
                    {f.name}
                    <span className="text-tinta/60">
                      {" "}· {f.kcal100 ? `${f.kcal100} kcal/100g` : "sin datos"} · {f.source}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {sel ? (
            <p className="text-sm">
              Elegido: <strong>{sel.name}</strong>{" "}
              <button type="button" onClick={() => setSel(null)} className="underline">cambiar</button>
            </p>
          ) : (
            <input
              value={customName} onChange={(e) => setCustomName(e.target.value)}
              placeholder="O escribí libre (ej: empanada de carne)"
              className={input}
            />
          )}
          {msg && <p className="text-sm text-terracota">{msg}</p>}
          <button type="submit" disabled={busy} className={btn}>
            {busy ? "Guardando…" : "Guardar"}
          </button>
        </form>
      </section>

      <section className="mt-6">
        <h2 className="font-display text-2xl">Registros</h2>
        {!data || data.entries.length === 0 ? (
          <p className="mt-2 text-sm">Nada este día.</p>
        ) : (
          <ul className="mt-2 divide-y divide-tinta/10">
            {data.entries.map((e) => (
              <li key={e.id} className="py-2 text-sm">
                <strong>{e.meal}</strong> — {e.label} ({e.qty} {e.unit})
                {e.nutrients ? ` · ${Math.round(e.nutrients.kcal)} kcal` : " · sin datos"}
                <div className="mt-1 flex items-center gap-2">
                  <input
                    value={editQty[e.id] ?? String(e.qty)}
                    onChange={(ev) => setEditQty((s) => ({ ...s, [e.id]: ev.target.value }))}
                    inputMode="decimal" aria-label="Corregir cantidad"
                    className="w-20 rounded border border-tinta/25 bg-panel px-2 py-1 text-sm"
                  />
                  <button onClick={() => saveQty(e.id)} className="text-sm underline">corregir</button>
                  <button onClick={() => del(e.id)} className="text-sm underline">borrar</button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="mt-6" id="peso">
        <h2 className="font-display text-2xl">Peso de hoy</h2>
        <form onSubmit={addWeight} className="mt-2 flex gap-2">
          <input
            value={weight} onChange={(e) => setWeight(e.target.value)}
            inputMode="decimal" placeholder="kg" aria-label="Peso en kg"
            className={input}
          />
          <button type="submit" className={btn}>Guardar</button>
        </form>
      </section>
    </main>
  );
}

export default function DiarioPage() {
  return (
    <Suspense fallback={<main className="mx-auto w-full max-w-2xl p-5">Cargando…</main>}>
      <DiarioInner />
    </Suspense>
  );
}
