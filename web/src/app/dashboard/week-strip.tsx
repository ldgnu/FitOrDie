"use client";

import { useEffect, useState } from "react";

interface D {
  day: string;
  kcal: number;
  verdict: string;
  entries: number;
}

const COLOR: Record<string, string> = {
  compatible: "bg-oliva",
  "con-ajustes": "bg-terracota",
  dificil: "bg-tinta",
  "sin-datos": "bg-tinta/20",
};

export default function WeekStrip() {
  const [days, setDays] = useState<D[]>([]);
  useEffect(() => {
    fetch("/api/diary/week")
      .then((r) => r.json())
      .then((d) => setDays(d.days ?? []))
      .catch(() => {});
  }, []);
  if (days.length === 0) return null;
  return (
    <section className="mt-6">
      <h2 className="font-display text-xl">Últimos 7 días</h2>
      <div className="mt-2 grid grid-cols-7 gap-1 text-center text-[10px] sm:text-xs">
        {days.map((d) => (
          <div key={d.day} className="rounded bg-panel/50 p-1">
            <p>{d.day.slice(5)}</p>
            <div
              className={`mx-auto my-1 h-2 w-2 rounded-full ${
                d.entries > 0 ? (COLOR[d.verdict] ?? "bg-tinta/20") : "bg-tinta/10"
              }`}
            />
            <p className="tabular-nums leading-tight">{d.entries > 0 ? d.kcal : "—"}</p>
          </div>
        ))}
      </div>
    </section>
  );
}
