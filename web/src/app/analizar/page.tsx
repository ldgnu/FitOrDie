"use client";

import { useEffect, useState } from "react";
import { ketoVerdict, netCarbs } from "@/lib/nutrition";

interface Food {
  id: string;
  name: string;
  source: string;
  sourceDetail: string | null;
  kcal100: string | null;
  protein100: string | null;
  fat100: string | null;
  carbs100: string | null;
  fiber100: string | null;
  sugar100: string | null;
  sodiumMg100: string | null;
}

type Luz = "verde" | "amarillo" | "rojo" | null;

const input =
  "rounded border border-tinta/25 bg-crema px-3 py-2 text-sm outline-none focus:border-terracota";

const DOT: Record<Exclude<Luz, null>, string> = {
  verde: "bg-oliva",
  amarillo: "bg-[#c9a227]",
  rojo: "bg-[#c0392b]",
};

// Umbrales orientativos por 100 g (no son diagnóstico).
const luzAzucar = (g: number | null): Luz =>
  g === null ? null : g <= 5 ? "verde" : g <= 15 ? "amarillo" : "rojo";
const luzSodio = (mg: number | null): Luz =>
  mg === null ? null : mg <= 120 ? "verde" : mg <= 600 ? "amarillo" : "rojo";

function Fila({
  nombre, valor, unidad, luz, ref100,
}: {
  nombre: string;
  valor: string;
  unidad: string;
  luz: Luz;
  ref100: string;
}) {
  return (
    <div className="flex items-center justify-between py-1.5 text-sm">
      <span className="flex items-center gap-2">
        {luz && <span className={`h-3 w-3 rounded-full ${DOT[luz]}`} />}
        {nombre}
      </span>
      <span>
        <strong>{valor}</strong> {unidad}
        <span className="text-tinta/60"> ({ref100}/100g)</span>
      </span>
    </div>
  );
}

// ¿Esto es keto? Analizador con semáforo: verde/amarillo/rojo por nutriente.
export default function AnalizarPage() {
  const [q, setQ] = useState("");
  const [results, setResults] = useState<Food[]>([]);
  const [sel, setSel] = useState<Food | null>(null);
  const [gramos, setGramos] = useState("100");
  const [objetivo, setObjetivo] = useState(30);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const r = await fetch("/api/profile");
        if (!r.ok || !alive) return;
        const d = await r.json();
        if (alive && d?.profile?.netCarbTarget) setObjetivo(d.profile.netCarbTarget);
      } catch {
        /* sin perfil: default 30 */
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

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

  const g = Number(gramos) > 0 ? Number(gramos) : 100;
  const f = (v: string | null): number | null => (v === null ? null : Number(v));
  const esc = (v: number | null): number | null =>
    v === null ? null : Math.round(v * (g / 100) * 10) / 10;

  const netos100 = sel ? netCarbs({ carbs: f(sel.carbs100), fiber: f(sel.fiber100) }) : null;
  const netos = netos100 === null ? null : Math.round(netos100 * (g / 100) * 10) / 10;
  const verdict = sel ? ketoVerdict(netos100 !== null ? (netos100 * g) / 100 : null, objetivo) : "sin-datos";
  const luzNetos: Luz =
    netos === null ? null : netos <= objetivo * 0.3 ? "verde" : netos <= objetivo * 0.6 ? "amarillo" : "rojo";

  const azucar = sel ? esc(f(sel.sugar100)) : null;
  const sodio = sel ? esc(f(sel.sodiumMg100)) : null;
  const rojos: string[] = [];
  if (luzAzucar(azucar) === "rojo") rojos.push("azúcar");
  if (luzSodio(sodio) === "rojo") rojos.push("sodio");
  if (luzNetos === "rojo") rojos.push("carbos netos");

  const VEREDICTO: Record<string, { txt: string; cls: string }> = {
    compatible: { txt: "KETO COMPATIBLE", cls: "bg-oliva text-crema" },
    "con-ajustes": { txt: "KETO CON AJUSTES", cls: "bg-[#c9a227] text-crema" },
    dificil: { txt: "DIFÍCIL EN KETO", cls: "bg-[#c0392b] text-crema" },
    "sin-datos": { txt: "SIN DATOS", cls: "bg-tinta/20 text-tinta" },
  };
  const v = VEREDICTO[verdict];

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-8 pt-6">
      <p className="text-sm tracking-wide text-oliva">ANALIZADOR</p>
      <h1 className="mt-2 font-display text-3xl">¿Esto es keto?</h1>

      <input
        value={q}
        onChange={(e) => {
          const v = e.target.value;
          setQ(v);
          setSel(null);
          if (v.trim().length < 2) setResults([]);
        }}
        placeholder="Buscar alimento argentino (ej: pan, tamari, mate)"
        className={`${input} mt-4 w-full`}
      />
      {results.length > 0 && !sel && (
        <ul className="divide-y divide-tinta/10 rounded border border-tinta/15 bg-panel">
          {results.map((x) => (
            <li key={x.id}>
              <button
                type="button"
                onClick={() => {
                  setSel(x);
                  setResults([]);
                }}
                className="w-full px-3 py-2 text-left text-sm"
              >
                {x.name}
                <span className="text-tinta/60"> · {x.source}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {sel && (
        <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
          <div className="flex items-center justify-between gap-2">
            <h2 className="font-display text-2xl">{sel.name}</h2>
            <button onClick={() => setSel(null)} className="text-sm underline">
              cambiar
            </button>
          </div>

          <div className={`mt-3 rounded px-4 py-3 text-center font-display text-xl ${v.cls}`}>
            {v.txt}
          </div>

          <div className="mt-3 flex items-center gap-2 text-sm">
            <span>Porción:</span>
            <input
              value={gramos}
              onChange={(e) => setGramos(e.target.value)}
              inputMode="decimal"
              aria-label="Gramos"
              className="w-24 rounded border border-tinta/25 bg-crema px-2 py-1"
            />
            <span>g</span>
          </div>

          <div className="mt-2 divide-y divide-tinta/10">
            <Fila nombre="Calorías" valor={String(esc(f(sel.kcal100)) ?? "—")} unidad="kcal" luz={null} ref100={sel.kcal100 ?? "?"} />
            <Fila nombre="Proteínas" valor={String(esc(f(sel.protein100)) ?? "—")} unidad="g" luz={null} ref100={sel.protein100 ?? "?"} />
            <Fila nombre="Grasas" valor={String(esc(f(sel.fat100)) ?? "—")} unidad="g" luz={null} ref100={sel.fat100 ?? "?"} />
            <Fila nombre="Carbos totales" valor={String(esc(f(sel.carbs100)) ?? "—")} unidad="g" luz={null} ref100={sel.carbs100 ?? "?"} />
            <Fila nombre="Fibra" valor={String(esc(f(sel.fiber100)) ?? "—")} unidad="g" luz={null} ref100={sel.fiber100 ?? "?"} />
            <Fila nombre="Carbos netos" valor={netos === null ? "—" : String(netos)} unidad="g" luz={luzNetos} ref100={netos100 === null ? "?" : String(netos100)} />
            <Fila nombre="Azúcares" valor={azucar === null ? "—" : String(azucar)} unidad="g" luz={luzAzucar(azucar)} ref100={sel.sugar100 ?? "?"} />
            <Fila nombre="Sodio" valor={sodio === null ? "—" : String(sodio)} unidad="mg" luz={luzSodio(sodio)} ref100={sel.sodiumMg100 ?? "?"} />
          </div>

          {rojos.length > 0 && (
            <p className="mt-3 rounded bg-[#c0392b]/15 px-3 py-2 text-sm">
              Ojo, alto en: <strong>{rojos.join(", ")}</strong>. En keto, eso pesa.
            </p>
          )}

          <p className="mt-3 text-xs text-tinta/60">
            Fuente: {sel.source}
            {sel.sourceDetail ? ` — ${sel.sourceDetail}` : ""}. Umbrales orientativos
            por 100 g (azúcar ≤5/15 · sodio ≤120/600). No es consejo médico.
          </p>
        </section>
      )}
    </main>
  );
}
