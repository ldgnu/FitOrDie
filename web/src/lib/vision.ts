// Visión local (Ollama) para identificar comida en fotos.
// El modelo solo INTERPRETA (qué hay + porciones estimadas); los cálculos
// los hace el motor nutricional con la tabla de alimentos. Nada se inventa:
// todo lo estimado vuelve marcado como asumido para confirmar.
import { findCandidates, type FoodRef, type ParsedItem } from "./parse-food";

const PROMPT = `Observá esta foto de comida y devolvé SOLO un JSON válido, sin texto extra, con este formato exacto:
{"items":[{"nombre":"huevo","cantidad":2,"unidad":"unidad"},{"nombre":"palta","cantidad":150,"unidad":"g"}]}
Reglas: unidad es "g", "ml", "unidad" o "porcion". Estimá porciones en gramos cuando puedas. Si algo no es comida o no lo reconocés, no lo incluyas. Si no hay comida, devolvé {"items":[]}.`;

interface VisionItem {
  nombre?: string;
  cantidad?: number;
  unidad?: string;
}

function base(): string | null {
  return process.env.OLLAMA_URL ?? null;
}

export function visionDisponible(): boolean {
  return base() !== null;
}

export async function identificarFoto(
  imageB64: string,
  refs: FoodRef[],
): Promise<{ items: ParsedItem[]; questions: string[] }> {
  const url = base();
  if (!url) {
    return {
      items: [],
      questions: ["Visión no configurada: describí la foto o cargala manual."],
    };
  }
  const model = process.env.OLLAMA_MODEL ?? "moondream";
  const res = await fetch(`${url}/api/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      model,
      messages: [{ role: "user", content: PROMPT, images: [imageB64] }],
      stream: false,
      format: "json",
      options: { temperature: 0, num_predict: 400 },
    }),
    signal: AbortSignal.timeout(120000),
  });
  if (!res.ok) throw new Error(`ollama ${res.status}`);
  const data = await res.json();
  const raw = String(data?.message?.content ?? "").replace(/```json|```/g, "").trim();
  let items: VisionItem[];
  try {
    items = JSON.parse(raw).items ?? [];
  } catch {
    throw new Error("respuesta no-JSON del modelo");
  }

  const out: ParsedItem[] = [];
  const questions: string[] = [];
  for (const it of Array.isArray(items) ? items : []) {
    const label = String(it.nombre ?? "").slice(0, 100) || "¿qué es esto?";
    const qty = Number(it.cantidad);
    const unit =
      it.unidad === "ml" ? "ml"
      : it.unidad === "unidad" ? "unidad"
      : it.unidad === "porcion" ? "porción"
      : "g";
    const cands = findCandidates(
      label.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, ""),
      refs,
    );
    const assumed = ["Estimado desde foto: verificá el plato y la cantidad"];
    if (cands.length === 0) {
      out.push({
        label, kind: "custom", foodId: null, recipeId: null,
        qty: Number.isFinite(qty) && qty > 0 ? qty : 1, unit, assumed,
      });
      questions.push(`En la foto hay "${label}" y no lo tengo: confirmalo o corregilo.`);
      continue;
    }
    const chosen = cands[0];
    if (cands.length > 1) {
      questions.push(
        `En la foto "${label}" puede ser: ${cands.slice(0, 3).map((c) => c.name).join(", ")}. Pre-cargué ${chosen.name}.`,
      );
    }
    if (chosen.kind === "recipe") {
      out.push({
        label, kind: "recipe", foodId: null, recipeId: chosen.id,
        qty: 1, unit: "porción", assumed,
        candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
      });
    } else {
      out.push({
        label: chosen.name, kind: "food", foodId: chosen.id, recipeId: null,
        qty: Number.isFinite(qty) && qty > 0 ? qty : 100, unit, assumed,
        candidates: cands.length > 1 ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind })) : undefined,
      });
    }
  }
  if (out.length === 0) questions.push("No reconocí comida en la foto. Describila o cargala manual.");
  return { items: out, questions };
}
