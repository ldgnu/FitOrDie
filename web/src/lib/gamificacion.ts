// Gamificación honesta estilo Duolingo, sin manipulación:
// racha por constancia (no por perfección), XP por acciones reales,
// logros que premian hasta la honestidad (anotar un mal día suma).
// Todo se deriva de datos existentes; los desbloqueos se guardan una vez.
import { and, asc, eq, isNull } from "drizzle-orm";
import { db } from "@/db";
import { achievements, diaryEntries, profiles, weights } from "@/db/schema";
import { getDaySummary } from "./day-summary";

export const NIVELES_SUAVE = [
  { min: 0, nombre: "Cebador" },
  { min: 200, nombre: "Parrillero" },
  { min: 500, nombre: "Cocinero" },
  { min: 1000, nombre: "Chef Keto" },
  { min: 2000, nombre: "Leyenda" },
];

// Testigo: abajo te bardea, arriba te respeta a regañadientes.
export const NIVELES_PICANTE = [
  { min: 0, nombre: "Caso Perdido" },
  { min: 200, nombre: "Proyecto" },
  { min: 500, nombre: "Zafa" },
  { min: 1000, nombre: "Respetable" },
  { min: 2000, nombre: "Máquina" },
];

export interface LogroDef {
  key: string;
  titulo: string;
  desc: string;
}

export const LOGROS: LogroDef[] = [
  { key: "primer-registro", titulo: "Primer plato", desc: "Anotaste tu primera comida." },
  { key: "balanza", titulo: "A la balanza", desc: "Registraste tu primer peso." },
  { key: "keto-perfecto", titulo: "Día keto", desc: "Un día dentro de tu objetivo de netos." },
  { key: "honestidad", titulo: "Verdad brutal", desc: "Anotaste un día pasado del objetivo. Eso suma." },
  { key: "semana-fuego", titulo: "Semana de fuego", desc: "7 días seguidos anotando." },
  { key: "treinta-dias", titulo: "Constancia", desc: "30 días distintos con registros." },
  { key: "mes-keto", titulo: "Mes keto", desc: "30 días compatibles acumulados." },
  { key: "menos-5", titulo: "−5 kg", desc: "Cinco kilos menos desde tu primer peso." },
  { key: "escalon-85", titulo: "Escalón 85", desc: "Llegaste a los 85 kg." },
  { key: "escalon-82", titulo: "Escalón 82", desc: "Llegaste a los 82 kg." },
  { key: "escalon-80", titulo: "Escalón 80", desc: "Llegaste a los 80 kg." },
];

export interface ResumenJuego {
  xp: number;
  nivel: string;
  siguiente: string | null;
  faltan: number;
  progresoNivel: number;
  racha: number;
  logros: Array<LogroDef & { desbloqueado: boolean; fecha: string | null }>;
}

function diaStr(d: Date): string {
  return d.toISOString().slice(0, 10);
}

export async function resumenJuego(userId: string): Promise<ResumenJuego> {
  const [profile] = await db
    .select()
    .from(profiles)
    .where(eq(profiles.userId, userId))
    .limit(1);

  const entries = await db
    .select()
    .from(diaryEntries)
    .where(and(eq(diaryEntries.userId, userId), isNull(diaryEntries.deletedAt)))
    .orderBy(asc(diaryEntries.consumedAt));

  const wl = await db
    .select()
    .from(weights)
    .where(eq(weights.userId, userId))
    .orderBy(asc(weights.measuredAt));

  const porDia = new Map<string, typeof entries>();
  for (const e of entries) {
    const d = diaStr(new Date(e.consumedAt));
    if (!porDia.has(d)) porDia.set(d, []);
    porDia.get(d)!.push(e);
  }
  const dias = [...porDia.keys()].sort();

  // Racha: si hoy no hay nada, cuenta desde ayer (estilo Duolingo).
  const hoy = diaStr(new Date());
  let racha = 0;
  const cursor = new Date(`${(dias.includes(hoy) ? hoy : (() => { const y = new Date(); y.setUTCDate(y.getUTCDate() - 1); return diaStr(y); })())}T12:00:00Z`);
  for (;;) {
    if (!porDia.has(diaStr(cursor))) break;
    racha++;
    cursor.setUTCDate(cursor.getUTCDate() - 1);
  }

  // Días analizados (últimos 45 con registros) para perfectos/keto/honestos.
  let perfectos = 0;
  let ketoDias = 0;
  let honestos = 0;
  const recientes = dias.slice(-45);
  for (const d of recientes) {
    const s = await getDaySummary(userId, d);
    if (s.keto.verdict === "compatible") ketoDias++;
    if (profile?.kcalTarget && s.entries.length >= 2) {
      if (s.totals.kcal <= profile.kcalTarget) perfectos++;
      else honestos++;
    }
  }

  const ok = new Set<string>();
  if (entries.length > 0) ok.add("primer-registro");
  if (wl.length > 0) ok.add("balanza");
  if (ketoDias > 0) ok.add("keto-perfecto");
  if (honestos > 0) ok.add("honestidad");
  if (racha >= 7) ok.add("semana-fuego");
  if (dias.length >= 30) ok.add("treinta-dias");
  if (ketoDias >= 30) ok.add("mes-keto");
  if (wl.length >= 2) {
    const primero = Number(wl[0].weightKg);
    const ultimo = Number(wl[wl.length - 1].weightKg);
    if (primero - ultimo >= 5) ok.add("menos-5");
  }
  if (wl.length > 0) {
    const min = Math.min(...wl.map((w) => Number(w.weightKg)));
    if (min <= 85) ok.add("escalon-85");
    if (min <= 82) ok.add("escalon-82");
    if (min <= 80) ok.add("escalon-80");
  }

  const xp = entries.length * 10 + wl.length * 5 + perfectos * 50 + ketoDias * 25;

  const NIVELES = profile?.crudeza === "suave" ? NIVELES_SUAVE : NIVELES_PICANTE;

  let nivel = NIVELES[0];
  let siguiente: (typeof NIVELES)[number] | null = null;
  for (let i = 0; i < NIVELES.length; i++) {
    if (xp >= NIVELES[i].min) {
      nivel = NIVELES[i];
      siguiente = NIVELES[i + 1] ?? null;
    }
  }
  const faltan = siguiente ? siguiente.min - xp : 0;
  const base = nivel.min;
  const techo = siguiente ? siguiente.min : nivel.min + 1;
  const progresoNivel = Math.min(100, Math.round(((xp - base) / (techo - base)) * 100));

  const prev = await db
    .select()
    .from(achievements)
    .where(eq(achievements.userId, userId));
  const fechas = new Map(prev.map((a) => [a.key, a.unlockedAt]));
  for (const k of ok) {
    if (!fechas.has(k)) {
      const ins = await db
        .insert(achievements)
        .values({ userId, key: k })
        .returning({ at: achievements.unlockedAt });
      fechas.set(k, ins[0]?.at ?? new Date());
    }
  }

  return {
    xp,
    nivel: nivel.nombre,
    siguiente: siguiente?.nombre ?? null,
    faltan,
    progresoNivel,
    racha,
    logros: LOGROS.map((l) => ({
      ...l,
      desbloqueado: ok.has(l.key),
      fecha: fechas.get(l.key)?.toISOString().slice(0, 10) ?? null,
    })),
  };
}
