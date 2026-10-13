"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { sugerir, rangoSaludable, type Sugerencia } from "@/lib/targets";

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
  weightKg: string | number | null;
  heightCm: number | null;
  age: number | null;
  sex: string | null;
  actividad: string | null;
  objetivo: string | null;
}

const input =
  "w-full rounded border border-tinta/25 bg-panel px-3 py-2 text-sm outline-none focus:border-terracota";
const csv = (a: string[] | null) => (a ?? []).join(", ");

function SugerenciaBox({
  f,
  onAplicar,
}: {
  f: { weightKg: string; heightCm: string; age: string; sex: string; actividad: string; objetivo: string; prefs: string };
  onAplicar: (s: Sugerencia) => void;
}) {
  const kg = Number(f.weightKg);
  const cm = Number(f.heightCm);
  const edad = Number(f.age);
  const okSex = f.sex === "m" || f.sex === "f";
  const okAct = ["sedentario", "ligero", "moderado", "intenso"].includes(f.actividad);
  const okObj = ["bajar", "mantener", "subir"].includes(f.objetivo);
  if (!Number.isFinite(kg) || !Number.isFinite(cm) || !Number.isFinite(edad) || !okSex || !okAct || !okObj) {
    return (
      <p className="text-sm text-tinta/70">
        Completá peso, altura, edad, sexo, actividad y objetivo para ver tu cálculo.
      </p>
    );
  }
  const keto = f.prefs.split(",").map((x) => x.trim()).includes("keto");
  const s = sugerir({
    sexo: f.sex as "m" | "f",
    kg, cm, edad,
    actividad: f.actividad as "sedentario" | "ligero" | "moderado" | "intenso",
    objetivo: f.objetivo as "bajar" | "mantener" | "subir",
    keto,
  });
  const rango = rangoSaludable(cm);
  return (
    <div className="rounded border border-oliva/40 bg-beige/40 p-3 text-sm">
      <p>
        Tu IMC: <strong>{s.imcActual}</strong> · rango saludable para tu altura:{" "}
        <strong>{rango.min}–{rango.max} kg</strong> (actual: <strong>{kg} kg</strong>)
      </p>
      <p className="mt-1">
        Quemás en reposo ~<strong>{s.tmb}</strong> y en total ~<strong>{s.tdee}</strong> kcal/día.
      </p>
      <p className="mt-1">
        Sugerencia ({f.objetivo}): <strong>{s.kcal} kcal</strong> · P {s.proteina} g ·
        netos {s.netos} g · G {s.grasa} g.
      </p>
      <button
        type="button"
        onClick={() => onAplicar(s)}
        className="mt-2 rounded bg-oliva px-4 py-2 text-crema"
      >
        Usar estos números
      </button>
    </div>
  );
}

export default function PerfilPage() {
  const [p, setP] = useState<Profile | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [f, setF] = useState({
    displayName: "", kcalTarget: "", proteinTarget: "", netCarbTarget: "",
    crudeza: "picante", units: "metric", timezone: "",
    prefs: "", allergies: "", dislikes: "",
    weightKg: "", heightCm: "", age: "", sex: "", actividad: "", objetivo: "bajar",
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
        weightKg: profile.weightKg?.toString() ?? "",
        heightCm: profile.heightCm?.toString() ?? "",
        age: profile.age?.toString() ?? "",
        sex: profile.sex ?? "",
        actividad: profile.actividad ?? "",
        objetivo: profile.objetivo ?? "bajar",
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
        weightKg: f.weightKg ? Number(f.weightKg) : null,
        heightCm: f.heightCm ? Number(f.heightCm) : null,
        age: f.age ? Number(f.age) : null,
        sex: f.sex || undefined,
        actividad: f.actividad || undefined,
        objetivo: f.objetivo || undefined,
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
        <h2 className="font-display text-2xl">Mi cuerpo</h2>
        <div className="grid grid-cols-3 gap-2">
          <div>
            <label className="text-sm">Peso kg</label>
            <input className={input} inputMode="decimal" {...set("weightKg")} />
          </div>
          <div>
            <label className="text-sm">Altura cm</label>
            <input className={input} inputMode="numeric" {...set("heightCm")} />
          </div>
          <div>
            <label className="text-sm">Edad</label>
            <input className={input} inputMode="numeric" {...set("age")} />
          </div>
        </div>
        <div className="grid grid-cols-3 gap-2">
          <div>
            <label className="text-sm">Sexo</label>
            <select className={input} {...set("sex")}>
              <option value="">—</option>
              <option value="m">Masculino</option>
              <option value="f">Femenino</option>
            </select>
          </div>
          <div>
            <label className="text-sm">Actividad</label>
            <select className={input} {...set("actividad")}>
              <option value="">—</option>
              <option value="sedentario">Poco/nada</option>
              <option value="ligero">Ligera</option>
              <option value="moderado">Moderada 3-5x</option>
              <option value="intenso">Intensa 6-7x</option>
            </select>
          </div>
          <div>
            <label className="text-sm">Objetivo</label>
            <select className={input} {...set("objetivo")}>
              <option value="bajar">Bajar</option>
              <option value="mantener">Mantener</option>
              <option value="subir">Subir</option>
            </select>
          </div>
        </div>
        <SugerenciaBox
          f={f}
          onAplicar={(s) =>
            setF((prev) => ({
              ...prev,
              kcalTarget: String(s.kcal),
              proteinTarget: String(s.proteina),
              netCarbTarget: String(s.netos),
            }))
          }
        />
        <h2 className="font-display text-2xl">Objetivos diarios</h2>
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
