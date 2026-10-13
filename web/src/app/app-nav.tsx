"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSession } from "next-auth/react";

const TABS = [
  { href: "/dashboard", label: "Hoy" },
  { href: "/diario", label: "Diario" },
  { href: "/recetario", label: "Recetas" },
  { href: "/progreso", label: "Progreso" },
  { href: "/perfil", label: "Perfil" },
];

// Navegación iPhone-first: barra superior sticky (no compite con el gesto
// home ni la toolbar de Safari) + botón flotante para anotar.
export default function AppNav() {
  const { status } = useSession();
  const path = usePathname();
  if (status !== "authenticated") return null;
  return (
    <>
      <header className="sticky top-0 z-40 border-b border-tinta/15 bg-crema/90 backdrop-blur">
        <div className="mx-auto flex max-w-2xl items-center gap-1 overflow-x-auto px-3">
          <Link
            href="/dashboard"
            className="mr-1 shrink-0 py-2.5 font-display text-lg"
          >
            FitOrDie
          </Link>
          {TABS.map((t) => {
            const active =
              path === t.href ||
              (t.href !== "/dashboard" && path.startsWith(t.href));
            return (
              <Link
                key={t.href}
                href={t.href}
                aria-current={active ? "page" : undefined}
                className={`shrink-0 rounded px-3 py-2 text-sm ${
                  active ? "bg-beige/70 text-tinta" : "text-tinta/60"
                }`}
              >
                {t.label}
              </Link>
            );
          })}
        </div>
      </header>
      <Link
        href="/diario"
        aria-label="Anotar comida"
        className="fixed bottom-[calc(1.25rem+env(safe-area-inset-bottom))] right-4 z-40 flex h-14 w-14 items-center justify-center rounded-full bg-terracota text-3xl leading-none text-crema"
      >
        +
      </Link>
    </>
  );
}
