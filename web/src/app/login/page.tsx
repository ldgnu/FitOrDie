"use client";

import Link from "next/link";
import { signIn } from "next-auth/react";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

function Form() {
  const router = useRouter();
  const sp = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    setBusy(true);
    setError(null);
    const res = await signIn("credentials", {
      email,
      password,
      redirect: false,
    });
    setBusy(false);
    if (res?.ok) router.push(sp.get("callbackUrl") ?? "/dashboard");
    else setError("Email o contraseña incorrectos.");
  }

  const input =
    "w-full rounded border border-tinta/25 bg-panel px-3 py-2 outline-none focus:border-terracota";

  return (
    <form onSubmit={onSubmit} className="mt-6 space-y-4">
      <div>
        <label className="text-sm" htmlFor="email">Email</label>
        <input
          id="email" type="email" required autoComplete="email"
          className={input} value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>
      <div>
        <label className="text-sm" htmlFor="password">Contraseña</label>
        <input
          id="password" type="password" required autoComplete="current-password"
          className={input} value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </div>
      {error && <p className="text-sm text-terracota">{error}</p>}
      <button
        type="submit" disabled={busy}
        className="w-full rounded bg-tinta px-5 py-3 text-crema disabled:opacity-60"
      >
        {busy ? "Entrando…" : "Entrar"}
      </button>
    </form>
  );
}

export default function LoginPage() {
  return (
    <main className="mx-auto w-full max-w-sm px-5 pb-16 pt-10">
      <p className="text-sm tracking-wide text-oliva">FITORDIE</p>
      <h1 className="mt-2 font-display text-3xl">Entrar</h1>
      <Suspense>
        <Form />
      </Suspense>
      <p className="mt-6 text-sm">
        ¿Sin cuenta?{" "}
        <Link href="/register" className="underline">Registrate</Link>
      </p>
    </main>
  );
}
