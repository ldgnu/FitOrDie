import Link from "next/link";
import { redirect } from "next/navigation";
import { getServerSession } from "next-auth";
import { desc, eq } from "drizzle-orm";
import { authOptions } from "@/auth";
import { db } from "@/db";
import { recipes, weights } from "@/db/schema";
import { getDaySummary } from "@/lib/day-summary";
import { resumenJuego } from "@/lib/gamificacion";
import LogoutButton from "./logout-button";
import WeekStrip from "./week-strip";

export const dynamic = "force-dynamic";

const MEAL_SLOTS = [
  { v: "desayuno", label: "Desayuno", hora: "9:00", desde: 5, hasta: 10 },
  { v: "almuerzo", label: "Almuerzo", hora: "13:00", desde: 10, hasta: 15 },
  { v: "merienda", label: "Merienda", hora: "18:00", desde: 15, hasta: 20 },
  { v: "cena", label: "Cena", hora: "22:00", desde: 20, hasta: 24 },
] as const;

const ESCALONES = [85, 82, 80];

function Bar({ pct, color }: { pct: number; color: string }) {
  return (
    <div className="mt-1 h-2 rounded bg-tinta/10">
      <div className={`h-2 rounded ${color}`} style={{ width: `${Math.min(100, pct)}%` }} />
    </div>
  );
}

export default async function Dashboard() {
  const session = await getServerSession(authOptions);
  if (!session?.user?.id) redirect("/login");
  const userId = session.user.id;

  const now = new Date();
  const day = now.toISOString().slice(0, 10);
  const hour = now.getHours();
  const saludo = hour < 12 ? "Buen día" : hour < 20 ? "Buenas tardes" : "Buenas noches";
  const actual = MEAL_SLOTS.find((m) => hour >= m.desde && hour < m.hasta) ?? MEAL_SLOTS[0];

  const s = await getDaySummary(userId, day);
  const wl = await db
    .select()
    .from(weights)
    .where(eq(weights.userId, userId))
    .orderBy(desc(weights.measuredAt))
    .limit(2);
  const lastW = wl[0] ? Number(wl[0].weightKg) : null;
  const nextEscalon = lastW === null ? null : ESCALONES.find((e) => lastW > e) ?? null;

  const allRecipes = await db
    .select({ id: recipes.id, title: recipes.title, photoUrl: recipes.photoUrl })
    .from(recipes)
    .where(eq(recipes.isPublic, true));
  const dayOfYear = Math.floor(
    (now.getTime() - new Date(now.getFullYear(), 0, 0).getTime()) / 86400000,
  );
  const recetaDia = allRecipes.length > 0 ? allRecipes[dayOfYear % allRecipes.length] : null;

  const testigo =
    s.entries.length === 0
      ? actual.v === "desayuno"
        ? "Arrancá el día anotando el desayuno. Ni el mate zafa del registro."
        : `Todavía no hay nada hoy. El dato no muerde: arrancá con ${actual.label.toLowerCase()}.`
      : s.kcalTarget && s.totals.kcal > s.kcalTarget
        ? "Te pasaste del objetivo. Anotá la verdad y seguí: esconderse no sirve."
        : s.keto.verdict === "compatible"
          ? "Venís bien. Mañana repetilo."
          : "Vas tirando. Revisá los netos antes de la próxima comida.";

  const restan = s.kcalTarget !== null ? s.kcalTarget - Math.round(s.totals.kcal) : null;
  const g = await resumenJuego(userId);

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-8 pt-6">
      <div className="flex items-center justify-between">
        <p className="text-sm tracking-wide text-oliva">
          {saludo.toUpperCase()} — {day}
        </p>
        <LogoutButton />
      </div>

      <p className="mt-3 border-l-4 border-terracota bg-beige/60 px-4 py-3 font-display text-xl">
        {testigo}
      </p>

      {/* Próxima comida */}
      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-tinta/70">Ahora toca</p>
            <p className="font-display text-2xl">
              {actual.label} <span className="text-base text-tinta/60">{actual.hora}</span>
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
          {MEAL_SLOTS.filter((m) => m.v !== actual.v).map((m) => (
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

      {/* Macros */}
      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">Hoy</h2>
          <p className="text-lg">
            <strong>{Math.round(s.totals.kcal)}</strong>
            {s.kcalTarget ? (
              <span className={restan !== null && restan < 0 ? "text-terracota" : "text-tinta/70"}>
                {" "}
                / {restan !== null && restan >= 0 ? `faltan ${restan}` : `pasado por ${restan !== null ? -restan : "?"}`}
              </span>
            ) : (
              " kcal"
            )}
          </p>
        </div>
        {s.kcalTarget && (
          <Bar pct={(s.totals.kcal / s.kcalTarget) * 100} color="bg-oliva" />
        )}
        <div className="mt-3 space-y-2 text-sm">
          <div>
            <div className="flex justify-between">
              <span>Proteína{s.proteinTarget ? ` / ${s.proteinTarget} g` : ""}</span>
              <strong>{Math.round(s.totals.protein)} g</strong>
            </div>
            {s.proteinTarget && <Bar pct={(s.totals.protein / s.proteinTarget) * 100} color="bg-terracota" />}
          </div>
          <div className="flex justify-between">
            <span>Grasas</span>
            <strong>{Math.round(s.totals.fat)} g</strong>
          </div>
          <div className="flex justify-between">
            <span>Netos (obj {s.keto.objetivo} g)</span>
            <strong>{s.keto.netos ?? "—"} g · {s.keto.verdict}</strong>
          </div>
        </div>
        {s.partial && (
          <p className="mt-2 text-sm text-terracota">
            Total parcial: hay entradas sin datos completos.
          </p>
        )}
      </section>

      {/* Receta del día */}
      {recetaDia && (
        <section className="mt-4 overflow-hidden rounded border border-tinta/15 bg-panel">
          <div className="flex items-center gap-3 p-4">
            {recetaDia.photoUrl && (
              <img
                src={recetaDia.photoUrl}
                alt=""
                loading="lazy"
                className="h-16 w-16 shrink-0 rounded object-cover"
              />
            )}
            <div className="min-w-0">
              <p className="text-sm text-tinta/70">Receta del día</p>
              <Link href={`/recetario/${recetaDia.id}`} className="font-display text-xl underline">
                {recetaDia.title}
              </Link>
            </div>
          </div>
        </section>
      )}

      {/* Peso + escalón */}
      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <div className="flex items-center justify-between">
          <h2 className="font-display text-2xl">Peso</h2>
          <Link href="/progreso" className="text-sm underline">
            Ver progreso
          </Link>
        </div>
        {lastW === null ? (
          <p className="mt-2 text-sm">Sin registros. Anotá el primero.</p>
        ) : (
          <p className="mt-2 text-lg">
            <strong>{lastW} kg</strong>
            {nextEscalon !== null && nextEscalon !== undefined && (
              <span className="text-sm text-tinta/70">
                {" "}
                · faltan {Math.round((lastW - nextEscalon) * 10) / 10} para los {nextEscalon}
              </span>
            )}
            {nextEscalon === null && (
              <span className="text-sm text-oliva"> · escalones cumplidos</span>
            )}
          </p>
        )}
      </section>

      {/* Juego */}
      <section className="mt-4 rounded border border-oliva/40 bg-panel p-4">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">Nivel {g.nivel}</h2>
          <p className="text-sm">
            Racha <strong>{g.racha} {g.racha === 1 ? "día" : "días"}</strong>
          </p>
        </div>
        <Bar pct={g.progresoNivel} color="bg-terracota" />
        <p className="mt-1 text-sm">
          {g.xp} XP
          {g.siguiente ? ` · faltan ${g.faltan} para ${g.siguiente}` : " · nivel máximo"}
        </p>
        <div className="mt-2 flex flex-wrap gap-1.5">
          {g.logros
            .filter((l) => l.desbloqueado)
            .slice(-4)
            .map((l) => (
              <span key={l.key} className="rounded border border-oliva/50 px-2 py-0.5 text-xs">
                {l.titulo}
              </span>
            ))}
          {g.logros.every((l) => !l.desbloqueado) && (
            <span className="text-xs text-tinta/60">Anotá tu primera comida y empieza el juego.</span>
          )}
        </div>
        <Link href="/progreso" className="mt-2 inline-block text-sm underline">
          Ver todos los logros
        </Link>
      </section>

      <WeekStrip />
    </main>
  );
}
