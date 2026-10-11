"use client";

import Link from "next/link";
import { signIn } from "next-auth/react";
import { useRouter } from "next/navigation";
import { useState } from "react";

export default function RegisterPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    setBusy(true);
    setError(null);
    const res = await fetch("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, email, password }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => null);
      setError(data?.error ?? "No se pudo crear la cuenta.");
      setBusy(false);
      return;
    }
    const login = await signIn("credentials", {
      email,
      password,
      redirect: false,
    });
    setBusy(false);
    if (login?.ok) router.push("/dashboard");
    else router.push("/login");
  }

  const input =
    "w-full rounded border border-tinta/25 bg-white px-3 py-2 outline-none focus:border-terracota";

  return (
    <main className="mx-auto w-full max-w-sm px-5 pb-16 pt-10">
      <p className="text-sm tracking-wide text-oliva">FITORDIE</p>
      <h1 className="mt-2 font-display text-3xl">Crear cuenta</h1>
      <form onSubmit={onSubmit} className="mt-6 space-y-4">
        <div>
          <label className="text-sm" htmlFor="name">Nombre (visible)</label>
          <input
            id="name" type="text" autoComplete="nickname"
            className={input} value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div>
          <label className="text-sm" htmlFor="email">Email</label>
          <input
            id="email" type="email" required autoComplete="email"
            className={input} value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div>
          <label className="text-sm" htmlFor="password">
            Contraseña (mínimo 10 caracteres)
          </label>
          <input
            id="password" type="password" required minLength={10}
            autoComplete="new-password"
            className={input} value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        {error && <p className="text-sm text-terracota">{error}</p>}
        <button
          type="submit" disabled={busy}
          className="w-full rounded bg-tinta px-5 py-3 text-crema disabled:opacity-60"
        >
          {busy ? "Creando…" : "Crear cuenta"}
        </button>
      </form>
      <p className="mt-6 text-sm">
        ¿Ya tenés cuenta?{" "}
        <Link href="/login" className="underline">Entrá</Link>
      </p>
    </main>
  );
}
