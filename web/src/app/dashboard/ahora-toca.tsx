"use client";

import Link from "next/link";

const SLOTS = [
  { v: "desayuno", label: "Desayuno", hora: "9:00", desde: 5, hasta: 10 },
  { v: "almuerzo", label: "Almuerzo", hora: "13:00", desde: 10, hasta: 15 },
  { v: "merienda", label: "Merienda", hora: "18:00", desde: 15, hasta: 20 },
  { v: "cena", label: "Cena", hora: "22:00", desde: 20, hasta: 24 },
] as const;

// La hora sale del DISPOSITIVO (new Date local), jamás del servidor (UTC).
export default function AhoraToca() {
  const h = new Date().getHours();
  const saludo = h < 12 ? "Buen día" : h < 20 ? "Buenas tardes" : "Buenas noches";
  const actual = SLOTS.find((m) => h >= m.desde && h < m.hasta) ?? SLOTS[0];

  return (
    <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-tinta/70">
            {saludo}. Ahora toca
          </p>
          <p className="font-display text-2xl">
            {actual.label}{" "}
            <span className="text-base text-tinta/60">{actual.hora}</span>
          </p>
        </div>
        <Link
          href={`/diario?meal=${actual.v}`}
          className="rounded bg-terracota px-5 py-3 font-bold text-crema"
        >
          + Anotar
        </Link>
      </div>
      <div className="mt-3 grid grid-cols-4 gap-2">
        {SLOTS.filter((m) => m.v !== actual.v).map((m) => (
          <Link
            key={m.v}
            href={`/diario?meal=${m.v}`}
            className="rounded border border-tinta/25 px-2 py-2 text-center text-sm"
          >
            {m.label}
            <span className="block text-xs text-tinta/60">{m.hora}</span>
          </Link>
        ))}
        <Link
          href="/diario?meal=snack"
          className="rounded border border-tinta/25 px-2 py-2 text-center text-sm"
        >
          Snack
          <span className="block text-xs text-tinta/60">antojo</span>
        </Link>
      </div>
    </section>
  );
}
