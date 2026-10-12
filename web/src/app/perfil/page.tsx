"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

interface Profile {
  displayName: string | null;
  units: string;
  timezone: string;
  crudeza: string;
  kcalTarget: number | null;
  proteinTarget: number | null;
  netCarbTarget: number | null;
  prefs: string[] | null;
  allergies: string[] | null;
  dislikes: string[] | null;
  notes: string | null;
}

const input =
  "w-full rounded border border-tinta/25 bg-panel px-3 py-2 text-sm outline-none focus:border-terracota";
const csv = (a: string[] | null) => (a ?? []).join(", ");

export default function PerfilPage() {
  const [p, setP] = useState<Profile | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [f, setF] = useState({
    displayName: "", kcalTarget: "", proteinTarget: "", netCarbTarget: "",
    crudeza: "picante", units: "metric", timezone: "",
    prefs: "", allergies: "", dislikes: "",
  });

  useEffect(() => {
    fetch("/api/profile").then(async (r) => {
      if (!r.ok) return;
      const { profile } = await r.json();
      if (!profile) return;
      setP(profile);
      setF({
        displayName: profile.displayName ?? "",
        kcalTarget: profile.kcalTarget?.toString() ?? "",
        proteinTarget: profile.proteinTarget?.toString() ?? "",
        netCarbTarget: profile.netCarbTarget?.toString() ?? "",
        crudeza: profile.crudeza ?? "picante",
        units: profile.units ?? "metric",
        timezone: profile.timezone ?? "",
        prefs: csv(profile.prefs),
        allergies: csv(profile.allergies),
        dislikes: csv(profile.dislikes),
      });
    });
  }, []);

  async function save(ev: React.FormEvent) {
    ev.preventDefault();
    setBusy(true);
    setMsg(null);
    const split = (s: string) =>
      s.split(",").map((x) => x.trim()).filter(Boolean);
    const r = await fetch("/api/profile", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        displayName: f.displayName || null,
        kcalTarget: f.kcalTarget ? Number(f.kcalTarget) : null,
        proteinTarget: f.proteinTarget ? Number(f.proteinTarget) : null,
        netCarbTarget: f.netCarbTarget ? Number(f.netCarbTarget) : null,
        crudeza: f.crudeza,
        units: f.units,
        timezone: f.timezone || undefined,
        prefs: split(f.prefs),
        allergies: split(f.allergies),
        dislikes: split(f.dislikes),
      }),
    });
    setBusy(false);
    if (!r.ok) {
      const d = await r.json().catch(() => null);
      setMsg(d?.error ?? "No se pudo guardar.");
      return;
    }
    setMsg("Guardado. La app ya habla con tus números.");
  }

  const set = (k: keyof typeof f) => ({
    value: f[k],
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
      setF((s) => ({ ...s, [k]: e.target.value })),
  });

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-16 pt-6">
      <Link href="/dashboard" className="text-sm underline">← Mi día</Link>
      <h1 className="mt-2 font-display text-3xl">Perfil</h1>
      <form onSubmit={save} className="mt-4 space-y-3">
        <div>
          <label className="text-sm">Nombre visible</label>
          <input className={input} {...set("displayName")} />
        </div>
        <div className="grid grid-cols-3 gap-2">
          <div>
            <label className="text-sm">Kcal/día</label>
            <input className={input} inputMode="numeric" {...set("kcalTarget")} />
          </div>
          <div>
            <label className="text-sm">Proteína g</label>
            <input className={input} inputMode="numeric" {...set("proteinTarget")} />
          </div>
          <div>
            <label className="text-sm">Netos g</label>
            <input className={input} inputMode="numeric" {...set("netCarbTarget")} />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="text-sm">Crudeza del Testigo</label>
            <select className={input} {...set("crudeza")}>
              <option value="suave">Suave</option>
              <option value="picante">Picante</option>
              <option value="sin-filtro">Sin filtro</option>
            </select>
          </div>
          <div>
            <label className="text-sm">Unidades</label>
            <select className={input} {...set("units")}>
              <option value="metric">Métricas</option>
              <option value="imperial">Imperiales</option>
            </select>
          </div>
        </div>
        <div>
          <label className="text-sm">Zona horaria</label>
          <input className={input} placeholder="America/Argentina/Buenos_Aires" {...set("timezone")} />
        </div>
        <div>
          <label className="text-sm">Preferencias (separadas por coma)</label>
          <input className={input} placeholder="keto, sin-gluten" {...set("prefs")} />
        </div>
        <div>
          <label className="text-sm">Alergias</label>
          <input className={input} {...set("allergies")} />
        </div>
        <div>
          <label className="text-sm">No me gusta / evito</label>
          <input className={input} {...set("dislikes")} />
        </div>
        {p?.notes && (
          <p className="text-sm text-tinta/70">Nota: {p.notes}</p>
        )}
        {msg && <p className="text-sm">{msg}</p>}
        <button disabled={busy} className="rounded bg-tinta px-5 py-3 text-crema disabled:opacity-60">
          {busy ? "Guardando…" : "Guardar"}
        </button>
      </form>
    </main>
  );
}
