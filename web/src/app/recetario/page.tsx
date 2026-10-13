"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

interface R {
  id: string;
  title: string;
  description: string | null;
  minutes: number | null;
  difficulty: string | null;
  photoUrl: string | null;
  categoria: string | null;
  sourceSite: string | null;
  sourceLikes: number;
  declarado: { kcal: string | null; protein: string | null; carbs: string | null };
  netos: number | null;
  verdict: string;
}

const CATS = ["desayuno", "almuerzo", "merienda", "cena", "bajon"];

const VERDICT_TXT: Record<string, string> = {
  compatible: "Keto ✓",
  "con-ajustes": "Con ajustes",
  dificil: "Difícil keto",
  "sin-datos": "Sin datos",
};

const input =
  "rounded border border-tinta/25 bg-panel px-3 py-2 text-sm outline-none focus:border-terracota";

export default function RecetarioPage() {
  const [q, setQ] = useState("");
  const [keto, setKeto] = useState(false);
  const [rapido, setRapido] = useState(false);
  const [cat, setCat] = useState("");
  const [list, setList] = useState<R[]>([]);

  useEffect(() => {
    const t = setTimeout(async () => {
      const sp = new URLSearchParams();
      if (q.trim()) sp.set("q", q.trim());
      if (keto) sp.set("keto", "1");
      if (rapido) sp.set("maxMinutes", "15");
      if (cat) sp.set("categoria", cat);
      const r = await fetch(`/api/recipes?${sp.toString()}`);
      if (r.ok) setList((await r.json()).recipes);
    }, 250);
    return () => clearTimeout(t);
  }, [q, keto, rapido, cat]);

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-16 pt-6">
      <Link href="/" className="text-sm underline">← Inicio</Link>
      <h1 className="mt-2 font-display text-3xl">Recetario</h1>
      <p className="mt-1 text-sm text-tinta/70">
        Comida real, ingredientes accesibles. Valores por porción calculados
        desde los ingredientes.
      </p>

      <div className="mt-4 flex flex-col gap-2 sm:flex-row">
        <input
          value={q} onChange={(e) => setQ(e.target.value)}
          placeholder="Buscar (ej: pollo, tortilla)"
          className={`${input} flex-1`}
        />
      </div>
      <div className="mt-2 flex gap-2 text-sm">
        <button
          onClick={() => setKeto((v) => !v)}
          className={`rounded border px-3 py-1 ${keto ? "border-tinta bg-tinta text-crema" : "border-tinta/25"}`}
        >
          Keto
        </button>
        <button
          onClick={() => setRapido((v) => !v)}
          className={`rounded border px-3 py-1 ${rapido ? "border-tinta bg-tinta text-crema" : "border-tinta/25"}`}
        >
          Rápido (&lt;15 min)
        </button>
      </div>
      <div className="mt-2 flex flex-wrap gap-2 text-sm">
        <button
          onClick={() => setCat("")}
          className={`rounded border px-3 py-1 ${cat === "" ? "border-tinta bg-tinta text-crema" : "border-tinta/25"}`}
        >
          Todas
        </button>
        {CATS.map((c) => (
          <button
            key={c}
            onClick={() => setCat(cat === c ? "" : c)}
            className={`rounded border px-3 py-1 ${cat === c ? "border-tinta bg-tinta text-crema" : "border-tinta/25"}`}
          >
            {c === "bajon" ? "Bajón" : c[0].toUpperCase() + c.slice(1)}
          </button>
        ))}
      </div>

      <ul className="mt-4 divide-y divide-tinta/10">
        {list.map((r) => (
          <li key={r.id} className="flex gap-3 py-3">
            {r.photoUrl && (
              <img
                src={r.photoUrl}
                alt=""
                loading="lazy"
                className="h-16 w-16 shrink-0 rounded object-cover"
              />
            )}
            <div>
            <Link href={`/recetario/${r.id}`} className="font-display text-xl underline">
              {r.title}
            </Link>
            <p className="text-sm text-tinta/70">{r.description}</p>
            <p className="mt-1 text-sm">
              {r.categoria ? `${r.categoria} · ` : ""}
              {r.minutes ? `${r.minutes} min` : ""}{" "}
              {r.difficulty ? `· ${r.difficulty}` : ""} ·{" "}
              {r.netos !== null ? `${r.netos} g netos` : r.declarado?.kcal ? `~${r.declarado.kcal} kcal (fuente)` : "sin datos"} ·{" "}
              <strong>{VERDICT_TXT[r.verdict]}</strong>
              {r.sourceSite ? <span className="text-tinta/60"> · {r.sourceSite}{r.sourceLikes ? ` ♥${r.sourceLikes}` : ""}</span> : ""}
            </p>
            </div>
          </li>
        ))}
      </ul>
      {list.length === 0 && (
        <p className="mt-4 text-sm">Nada con esos filtros. Probá otra búsqueda.</p>
      )}
    </main>
  );
}
