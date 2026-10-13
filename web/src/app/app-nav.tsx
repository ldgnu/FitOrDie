"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSession } from "next-auth/react";

function Icon({ d }: { d: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      className="h-6 w-6"
      aria-hidden
    >
      <path d={d} />
    </svg>
  );
}

const TABS = [
  { href: "/dashboard", label: "Hoy", d: "M3 11l9-8 9 8M5 10v10h14V10" },
  { href: "/diario", label: "Diario", d: "M12 5v14M5 12h14" },
  { href: "/recetario", label: "Recetas", d: "M4 5h16v14H4zM8 9h8M8 13h8M8 17h5" },
  { href: "/progreso", label: "Progreso", d: "M4 20V10M10 20V4M16 20v-8M22 20H2" },
  { href: "/perfil", label: "Perfil", d: "M12 12a4 4 0 100-8 4 4 0 000 8zM4 21c0-4 4-6 8-6s8 2 8 6" },
];

export default function AppNav() {
  const { status } = useSession();
  const path = usePathname();
  if (status !== "authenticated") return null;
  return (
    <>
      <div className="h-24" aria-hidden />
      <nav className="fixed inset-x-0 bottom-0 z-40 border-t border-tinta/15 bg-panel/95 backdrop-blur">
        <div className="mx-auto grid max-w-2xl grid-cols-5 pb-[env(safe-area-inset-bottom)]">
          {TABS.map((t) => {
            const active = path === t.href || (t.href !== "/dashboard" && path.startsWith(t.href));
            return (
              <Link
                key={t.href}
                href={t.href}
                aria-current={active ? "page" : undefined}
                className={`flex flex-col items-center gap-0.5 py-2.5 text-[11px] ${
                  active ? "text-terracota" : "text-tinta/60"
                }`}
              >
                <Icon d={t.d} />
                {t.label}
              </Link>
            );
          })}
        </div>
      </nav>
    </>
  );
}
