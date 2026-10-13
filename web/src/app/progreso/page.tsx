import { redirect } from "next/navigation";
import { getServerSession } from "next-auth";
import { asc, eq } from "drizzle-orm";
import { authOptions } from "@/auth";
import { db } from "@/db";
import { weights } from "@/db/schema";
import { resumenJuego } from "@/lib/gamificacion";
import WeekStrip from "../dashboard/week-strip";
import PesoForm from "./peso-form";

export const dynamic = "force-dynamic";

const ESCALONES = [85, 82, 80];

function Chart({ data }: { data: Array<{ t: string; kg: number }> }) {
  if (data.length === 0)
    return <p className="mt-2 text-sm">Sin registros todavía.</p>;
  if (data.length === 1)
    return (
      <p className="mt-2 text-lg">
        <strong>{data[0].kg} kg</strong> · {data[0].t} (primer punto)
      </p>
    );
  const vals = data.map((d) => d.kg);
  const min = Math.min(...vals);
  const max = Math.max(...vals);
  const span = max - min || 1;
  const W = 600;
  const H = 160;
  const pts = data
    .map(
      (d, i) =>
        `${((i / (data.length - 1)) * W).toFixed(1)},${(
          H - 12 - ((d.kg - min) / span) * (H - 32)
        ).toFixed(1)}`,
    )
    .join(" ");
  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} className="mt-2 w-full" role="img" aria-label="Evolución del peso">
        <polyline points={pts} fill="none" stroke="#a3ad7f" strokeWidth="3" strokeLinejoin="round" />
        {data.map((d, i) => {
          const cx = (i / (data.length - 1)) * W;
          const cy = H - 12 - ((d.kg - min) / span) * (H - 32);
          return <circle key={d.t} cx={cx} cy={cy} r="4" fill="#cf8261" />;
        })}
      </svg>
      <div className="flex justify-between text-xs text-tinta/60">
        <span>
          {data[0].t} · {data[0].kg}
        </span>
        <span>
          {data[data.length - 1].t} · {data[data.length - 1].kg}
        </span>
      </div>
    </div>
  );
}

export default async function ProgresoPage() {
  const session = await getServerSession(authOptions);
  if (!session?.user?.id) redirect("/login");
  const userId = session.user.id;

  const rows = await db
    .select()
    .from(weights)
    .where(eq(weights.userId, userId))
    .orderBy(asc(weights.measuredAt))
    .limit(200);
  const data = rows.map((r) => ({
    t: new Date(r.measuredAt).toISOString().slice(0, 10),
    kg: Number(r.weightKg),
  }));
  const last = data.length > 0 ? data[data.length - 1].kg : null;
  const g = await resumenJuego(userId);

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-8 pt-6">
      <p className="text-sm tracking-wide text-oliva">PROGRESO</p>
      <h1 className="mt-2 font-display text-3xl">Tu evolución</h1>

      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <h2 className="font-display text-2xl">Peso</h2>
        <Chart data={data.slice(-30)} />
        <PesoForm />
      </section>

      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <h2 className="font-display text-2xl">Escalones</h2>
        <ul className="mt-2 space-y-2">
          {ESCALONES.map((e) => {
            const done = last !== null && last <= e;
            return (
              <li key={e} className="flex items-center justify-between text-sm">
                <span className={done ? "text-oliva" : ""}>
                  {done ? "✓ " : "○ "}{e} kg
                </span>
                {last !== null && !done && (
                  <span className="text-tinta/60">
                    faltan {Math.round((last - e) * 10) / 10}
                  </span>
                )}
              </li>
            );
          })}
        </ul>
        {last === null && <p className="mt-2 text-sm">Anotá tu peso para activar los escalones.</p>}
      </section>

      <section className="mt-4 rounded border border-tinta/15 bg-panel p-4">
        <h2 className="font-display text-2xl">Logros</h2>
        <ul className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {g.logros.map((l) => (
            <li
              key={l.key}
              className={`rounded border p-2 text-sm ${
                l.desbloqueado ? "border-oliva/50" : "border-tinta/10 opacity-50"
              }`}
            >
              <strong>{l.titulo}</strong>
              <span className="block text-xs text-tinta/70">{l.desc}</span>
              {l.fecha && <span className="text-xs text-oliva">{l.fecha}</span>}
            </li>
          ))}
        </ul>
      </section>

      <WeekStrip />

      <section className="mt-4">
        <h2 className="font-display text-2xl">Historial</h2>
        {data.length === 0 ? (
          <p className="mt-2 text-sm">Vacío.</p>
        ) : (
          <ul className="mt-2 divide-y divide-tinta/10 text-sm">
            {[...data].reverse().slice(0, 20).map((d, i) => (
              <li key={`${d.t}-${i}`} className="py-1.5">
                {d.t} — <strong>{d.kg} kg</strong>
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
