"use client";

import { useState } from "react";
import type { ParsedItem } from "@/lib/parse-food";

const MEALS = ["desayuno", "media-manana", "almuerzo", "merienda", "cena", "snack"];
const input =
  "rounded border border-tinta/25 bg-crema px-3 py-2 text-sm outline-none focus:border-terracota";

interface FotoRes {
  photoId: string;
  vision: boolean;
  items: ParsedItem[];
  questions: string[];
}

// Caja "sacale foto": captura de cámara (iPhone) o archivo → identifica →
// preview editable → confirmar guarda en el diario y vincula la foto.
export default function FotoBox({ onSaved }: { onSaved: () => void }) {
  const [preview, setPreview] = useState<string | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [res, setRes] = useState<FotoRes | null>(null);
  const [meal, setMeal] = useState("almuerzo");
  const [qty, setQty] = useState<Record<number, string>>({});
  const [pick, setPick] = useState<Record<number, string>>({});
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function elegir(f: File | null) {
    setFile(f);
    setRes(null);
    setMsg(null);
    setPreview(f ? URL.createObjectURL(f) : null);
  }

  async function interpretar() {
    if (!file) return;
    setBusy(true);
    setMsg(null);
    const fd = new FormData();
    fd.append("photo", file);
    const r = await fetch("/api/diary/foto", { method: "POST", body: fd });
    setBusy(false);
    if (!r.ok) {
      const d = await r.json().catch(() => null);
      setMsg(d?.error ?? "No se pudo subir la foto.");
      return;
    }
    const d: FotoRes = await r.json();
    setRes(d);
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
    let firstId: string | null = null;
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
      if (!firstId) firstId = (await r.json().catch(() => ({})))?.id ?? null;
    }
    if (firstId) {
      await fetch("/api/diary/foto", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ photoId: res.photoId, entryId: firstId }),
      });
    }
    setBusy(false);
    setRes(null);
    setFile(null);
    setPreview(null);
    onSaved();
  }

  return (
    <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
      <h2 className="font-display text-2xl">Sacale foto al plato</h2>
      <div className="mt-2 flex flex-col gap-2">
        <input
          type="file"
          accept="image/*"
          capture="environment"
          aria-label="Foto del plato"
          onChange={(e) => elegir(e.target.files?.[0] ?? null)}
          className="text-sm"
        />
        {preview && (
          <img src={preview} alt="Vista previa del plato" className="aspect-[4/3] w-full rounded object-cover" />
        )}
        <button
          onClick={interpretar}
          disabled={!file || busy}
          className="rounded bg-oliva px-4 py-2 text-sm text-crema disabled:opacity-60"
        >
          {busy ? "Leyendo…" : "Identificar qué comí"}
        </button>
      </div>

      {res && (
        <div className="mt-3 space-y-2 text-sm">
          {!res.vision && (
            <p className="text-terracota">
              Visión no configurada en este servidor: cargá manual abajo.
            </p>
          )}
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
                <strong>{it.label}</strong>{" "}
                {it.kind === "custom" && <span>(va sin cálculo)</span>}
                {it.candidates?.length ? (
                  <select
                    value={pick[i] ?? ""}
                    onChange={(e) => setPick((s) => ({ ...s, [i]: e.target.value }))}
                    className={input}
                  >
                    {it.candidates.map((c) => (
                      <option key={c.id} value={c.id}>{c.name}</option>
                    ))}
                  </select>
                ) : it.kind !== "custom" ? (
                  <span className="text-tinta/70">
                    {(it.kind === "recipe" ? "receta" : "")}
                  </span>
                ) : null}
              </p>
              <div className="mt-1 flex items-center gap-2">
                <input
                  value={qty[i] ?? ""}
                  onChange={(e) => setQty((s) => ({ ...s, [i]: e.target.value }))}
                  inputMode="decimal"
                  aria-label="Cantidad"
                  className="w-24 rounded border border-tinta/25 bg-crema px-2 py-1"
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
          {res.items.length > 0 && (
            <button
              onClick={confirmar}
              disabled={busy}
              className="rounded bg-tinta px-4 py-2 text-sm text-crema disabled:opacity-60"
            >
              {busy ? "Guardando…" : `Confirmar y guardar (${res.items.length})`}
            </button>
          )}
          {msg && <p className="text-terracota">{msg}</p>}
        </div>
      )}
    </section>
  );
}
