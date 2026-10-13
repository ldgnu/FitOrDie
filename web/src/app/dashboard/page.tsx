import Link from "next/link";
import { redirect } from "next/navigation";
import { getServerSession } from "next-auth";
import { desc, eq } from "drizzle-orm";
import { authOptions } from "@/auth";
import { db } from "@/db";
import { profiles, recipes, weights } from "@/db/schema";
import { getDaySummary } from "@/lib/day-summary";
import { resumenJuego } from "@/lib/gamificacion";
import AhoraToca from "./ahora-toca";
import LogoutButton from "./logout-button";
import WeekStrip from "./week-strip";

export const dynamic = "force-dynamic";

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
  const [prof] = await db
    .select()
    .from(profiles)
    .where(eq(profiles.userId, userId))
    .limit(1);
  const tz = prof?.timezone ?? "America/Argentina/Buenos_Aires";
  const day = new Intl.DateTimeFormat("en-CA", {
    timeZone: tz,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);

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
      ? "Hoy todavía no anotaste nada. El dato no muerde: arrancá con lo que comiste."
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
          HOY — {day}
        </p>
        <LogoutButton />
      </div>

      <p className="mt-3 border-l-4 border-terracota bg-beige/60 px-4 py-3 font-display text-xl">
        {testigo}
      </p>

      <AhoraToca />

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

      {/* Analizar */}
      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-tinta/70">Duda con algo</p>
            <p className="font-display text-2xl">¿Esto es keto?</p>
          </div>
          <Link
            href="/analizar"
            className="rounded bg-tinta px-5 py-3 text-crema"
          >
            Analizar
          </Link>
        </div>
      </section>

      {/* Delivery */}
      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-tinta/70">Sin ganas de cocinar</p>
            <p className="font-display text-2xl">Pedir afuera keto</p>
          </div>
          <Link
            href="/delivery"
            className="rounded bg-oliva px-5 py-3 font-bold text-crema"
          >
            Ver delivery
          </Link>
        </div>
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
