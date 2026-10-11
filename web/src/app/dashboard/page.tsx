import Link from "next/link";
import { redirect } from "next/navigation";
import { getServerSession } from "next-auth";
import { desc, eq } from "drizzle-orm";
import { authOptions } from "@/auth";
import { db } from "@/db";
import { weights } from "@/db/schema";
import { getDaySummary } from "@/lib/day-summary";
import LogoutButton from "./logout-button";

export const dynamic = "force-dynamic";

const VERDICT_TXT: Record<string, string> = {
  compatible: "Compatible con tu objetivo keto.",
  "con-ajustes": "Va con ajustes: ojo con las porciones.",
  dificil: "Difícil de encajar hoy.",
  "sin-datos": "Sin datos suficientes para evaluar.",
};

export default async function Dashboard() {
  const session = await getServerSession(authOptions);
  if (!session?.user?.id) redirect("/login");

  const day = new Date().toISOString().slice(0, 10);
  const s = await getDaySummary(session.user.id, day);
  const wl = await db
    .select()
    .from(weights)
    .where(eq(weights.userId, session.user.id))
    .orderBy(desc(weights.measuredAt))
    .limit(2);
  const lastW = wl[0] ? Number(wl[0].weightKg) : null;
  const trend =
    wl.length === 2
      ? Math.round((Number(wl[0].weightKg) - Number(wl[1].weightKg)) * 10) / 10
      : null;

  const testigo =
    s.entries.length === 0
      ? "Hoy todavía no anotaste nada. El dato no muerde: arrancá con lo que comiste."
      : s.kcalTarget && s.totals.kcal > s.kcalTarget
        ? "Te pasaste del objetivo. Anotá la verdad y seguí: esconderse no sirve."
        : s.keto.verdict === "compatible"
          ? "Venís bien. Mañana repetilo."
          : "Vas tirando. Revisá los netos antes de la próxima comida.";

  const pct = s.kcalTarget
    ? Math.min(100, Math.round((s.totals.kcal / s.kcalTarget) * 100))
    : null;

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-16 pt-6">
      <div className="flex items-center justify-between">
        <p className="text-sm tracking-wide text-oliva">MI DÍA — {day}</p>
        <LogoutButton />
      </div>

      <p className="mt-3 border-l-4 border-terracota bg-beige/60 px-4 py-3 font-display text-xl">
        {testigo}
      </p>

      <section className="mt-6 rounded border border-tinta/15 bg-white/50 p-4">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">Calorías</h2>
          <p className="text-lg">
            <strong>{Math.round(s.totals.kcal)}</strong>
            {s.kcalTarget ? ` / ${s.kcalTarget}` : " kcal"}
          </p>
        </div>
        {pct !== null && (
          <div className="mt-2 h-2 rounded bg-tinta/10">
            <div
              className="h-2 rounded bg-oliva"
              style={{ width: `${pct}%` }}
            />
          </div>
        )}
        <div className="mt-3 grid grid-cols-3 gap-2 text-center text-sm">
          <div className="rounded bg-beige/60 p-2">
            <p className="font-bold">{Math.round(s.totals.protein)} g</p>
            <p>proteínas{s.proteinTarget ? ` / ${s.proteinTarget}` : ""}</p>
          </div>
          <div className="rounded bg-beige/60 p-2">
            <p className="font-bold">{Math.round(s.totals.fat)} g</p>
            <p>grasas</p>
          </div>
          <div className="rounded bg-beige/60 p-2">
            <p className="font-bold">{Math.round(s.totals.carbs)} g</p>
            <p>carbos</p>
          </div>
        </div>
        <p className="mt-3 text-sm">
          Netos: <strong>{s.keto.netos ?? "—"} g</strong> (objetivo{" "}
          {s.keto.objetivo} g) — {VERDICT_TXT[s.keto.verdict]}
        </p>
        {s.partial && (
          <p className="mt-1 text-sm text-terracota">
            Algunas entradas no tienen datos completos: el total es parcial.
          </p>
        )}
      </section>

      <section className="mt-6">
        <h2 className="font-display text-2xl">Comidas de hoy</h2>
        {s.entries.length === 0 ? (
          <p className="mt-2">Nada registrado todavía.</p>
        ) : (
          <ul className="mt-2 divide-y divide-tinta/10">
            {s.entries.map((e) => (
              <li key={e.id} className="py-2 text-sm">
                <strong>{e.meal}</strong> — {e.label} ({e.qty} {e.unit})
                {e.nutrients ? (
                  <span className="text-tinta/70">
                    {" "}
                    · {Math.round(e.nutrients.kcal)} kcal
                  </span>
                ) : (
                  <span className="text-terracota"> · sin datos</span>
                )}
              </li>
            ))}
          </ul>
        )}
        <Link
          href="/diario"
          className="mt-3 inline-block rounded bg-tinta px-5 py-3 text-crema"
        >
          Anotar comida
        </Link>
      </section>

      <section className="mt-6 rounded border border-tinta/15 bg-white/50 p-4">
        <h2 className="font-display text-2xl">Peso</h2>
        {lastW === null ? (
          <p className="mt-2 text-sm">
            Sin registros. Anotá el primero en el diario.
          </p>
        ) : (
          <p className="mt-2 text-lg">
            <strong>{lastW} kg</strong>
            {trend !== null && (
              <span className="text-sm">
                {" "}
                ({trend > 0 ? "+" : ""}
                {trend} kg vs anterior)
              </span>
            )}
          </p>
        )}
      </section>
    </main>
  );
}
