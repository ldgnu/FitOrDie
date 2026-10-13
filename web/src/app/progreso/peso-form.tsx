"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

const input =
  "rounded border border-tinta/25 bg-crema px-3 py-2 text-sm outline-none focus:border-terracota";

export default function PesoForm() {
  const router = useRouter();
  const [v, setV] = useState("");
  const [msg, setMsg] = useState<string | null>(null);

  async function save(ev: React.FormEvent) {
    ev.preventDefault();
    const n = Number(v);
    if (!Number.isFinite(n) || n < 20 || n > 400) {
      setMsg("Peso inválido.");
      return;
    }
    const r = await fetch("/api/weight", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ weightKg: n }),
    });
    if (!r.ok) {
      setMsg("No se pudo guardar.");
      return;
    }
    setV("");
    setMsg("Anotado.");
    router.refresh();
  }

  return (
    <form onSubmit={save} className="mt-2 flex gap-2">
      <input
        value={v}
        onChange={(e) => setV(e.target.value)}
        inputMode="decimal"
        placeholder="kg de hoy"
        aria-label="Peso en kg"
        className={input}
      />
      <button className="rounded bg-tinta px-4 py-2 text-sm text-crema">Guardar</button>
      {msg && <p className="self-center text-sm">{msg}</p>}
    </form>
  );
}
