"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";

interface Ing {
  name: string;
  qty: string;
  unit: string;
  substitution: string | null;
}
interface Detail {
  recipe: {
    id: string; title: string; description: string | null; servings: number;
    minutes: number | null; difficulty: string | null; instructions: string;
    storageTip: string | null;
  };
  ingredients: Ing[];
  perPortion: { kcal: number; protein: number; fat: number; carbs: number; fiber: number } | null;
  netos: number | null;
  verdict: string;
}

const MEALS = ["desayuno", "media-manana", "almuerzo", "merienda", "cena", "snack"];
const input =
  "rounded border border-tinta/25 bg-white px-3 py-2 text-sm outline-none focus:border-terracota";

export default function RecetaPage() {
  const { id } = useParams<{ id: string }>();
  const [d, setD] = useState<Detail | null>(null);
  const [meal, setMeal] = useState("almuerzo");
  const [portions, setPortions] = useState("1");
  const [msg, setMsg] = useState<string | null>(null);

  useEffect(() => {
    fetch(`/api/recipes/${id}`).then(async (r) => {
      if (r.ok) setD(await r.json());
    });
  }, [id]);

  async function registrar(ev: React.FormEvent) {
    ev.preventDefault();
    setMsg(null);
    const n = Number(portions);
    if (!Number.isFinite(n) || n <= 0 || n > 20) {
      setMsg("Porciones inválidas.");
      return;
    }
    const r = await fetch("/api/diary", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        consumedAt: new Date().toISOString(),
        meal,
        recipeId: id,
        qty: n,
        unit: "porción",
      }),
    });
    if (r.status === 401) {
      setMsg("Entrá a tu cuenta primero para anotar.");
      return;
    }
    if (!r.ok) {
      setMsg("No se pudo anotar.");
      return;
    }
    setMsg("Anotada en tu diario. Bien.");
  }

  if (!d) return <main className="p-5">Cargando…</main>;
  const p = d.perPortion;

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-16 pt-6">
      <Link href="/recetario" className="text-sm underline">← Recetario</Link>
      <h1 className="mt-2 font-display text-3xl">{d.recipe.title}</h1>
      {d.recipe.description && <p className="mt-2">{d.recipe.description}</p>}
      <p className="mt-1 text-sm text-tinta/70">
        {d.recipe.minutes ? `${d.recipe.minutes} min · ` : ""}
        {d.recipe.difficulty ? `${d.recipe.difficulty} · ` : ""}
        {d.recipe.servings} {d.recipe.servings === 1 ? "porción" : "porciones"}
      </p>

      <h2 className="mt-6 font-display text-2xl">Ingredientes</h2>
      <ul className="mt-2 divide-y divide-tinta/10 text-sm">
        {d.ingredients.map((ing, i) => (
          <li key={i} className="py-1">
            {ing.qty} {ing.unit} — <strong>{ing.name}</strong>
            {ing.substitution && (
              <span className="text-tinta/60"> (o {ing.substitution})</span>
            )}
          </li>
        ))}
      </ul>

      <h2 className="mt-6 font-display text-2xl">Preparación</h2>
      <p className="mt-2 text-sm leading-relaxed">{d.recipe.instructions}</p>
      {d.recipe.storageTip && (
        <p className="mt-2 text-sm text-tinta/70">Conservación: {d.recipe.storageTip}</p>
      )}

      <h2 className="mt-6 font-display text-2xl">Por porción (estimado)</h2>
      {p ? (
        <p className="mt-2 text-sm">
          <strong>{Math.round(p.kcal)} kcal</strong> · P {Math.round(p.protein)} · G{" "}
          {Math.round(p.fat)} · C {Math.round(p.carbs)} · netos{" "}
          <strong>{d.netos} g</strong> ({d.verdict})
        </p>
      ) : (
        <p className="mt-2 text-sm text-terracota">
          Sin datos suficientes para calcular esta receta.
        </p>
      )}

      <h2 className="mt-6 font-display text-2xl">Anotar en mi diario</h2>
      <form onSubmit={registrar} className="mt-2 flex flex-col gap-2 sm:flex-row">
        <select value={meal} onChange={(e) => setMeal(e.target.value)} className={input}>
          {MEALS.map((m) => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
        <input
          value={portions} onChange={(e) => setPortions(e.target.value)}
          inputMode="decimal" aria-label="Porciones" className={input}
        />
        <button className="rounded bg-tinta px-4 py-2 text-sm text-crema">
          Anotar
        </button>
      </form>
      {msg && <p className="mt-2 text-sm">{msg}</p>}
    </main>
  );
}
