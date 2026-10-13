// Visión para identificar comida en fotos. Proveedores configurables:
//   VISION_PROVIDER=ollama (local, default si hay OLLAMA_URL)
//   VISION_PROVIDER=gemini (nube, requiere GEMINI_API_KEY)
//   VISION_PROVIDER=groq   (nube, requiere GROQ_API_KEY)
//   ausente/manual         (la foto se guarda y se carga a mano)
// El proveedor solo INTERPRETA (qué hay + porciones estimadas); los cálculos
// los hace el motor nutricional. Todo lo estimado vuelve marcado como asumido.
// Las fotos solo salen de tu servidor si VOS configurás un proveedor de nube.
import { findCandidates, type FoodRef, type ParsedItem } from "./parse-food";

const PROMPT = `Observá esta foto de comida y devolvé SOLO un JSON válido, sin texto extra, con este formato exacto:
{"items":[{"nombre":"huevo","cantidad":2,"unidad":"unidad"},{"nombre":"palta","cantidad":150,"unidad":"g"}]}
Reglas: unidad es "g", "ml", "unidad" o "porcion". Estimá porciones en gramos cuando puedas. Si algo no es comida o no lo reconocés, no lo incluyas. Si no hay comida, devolvé {"items":[]}.`;

interface VisionItem {
  nombre?: string;
  cantidad?: number;
  unidad?: string;
}

export type Provider = "ollama" | "gemini" | "groq" | "none";

export function provider(): Provider {
  const p = (process.env.VISION_PROVIDER ?? "").toLowerCase();
  if (p === "gemini" || p === "groq" || p === "ollama" || p === "none") return p;
  if (process.env.OLLAMA_URL) return "ollama";
  return "none";
}

export function visionDisponible(): boolean {
  const p = provider();
  if (p === "ollama") return true;
  if (p === "gemini") return !!process.env.GEMINI_API_KEY;
  if (p === "groq") return !!process.env.GROQ_API_KEY;
  return false;
}

function mapear(
  raw: unknown,
  refs: FoodRef[],
): { items: ParsedItem[]; questions: string[] } {
  const list = (raw as { items?: VisionItem[] })?.items;
  const arr = Array.isArray(list) ? list : [];
  const out: ParsedItem[] = [];
  const questions: string[] = [];
  for (const it of arr) {
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
    const candShort = cands.length > 1
      ? cands.slice(0, 5).map((c) => ({ id: c.id, name: c.name, kind: c.kind }))
      : undefined;
    if (chosen.kind === "recipe") {
      out.push({
        label, kind: "recipe", foodId: null, recipeId: chosen.id,
        qty: 1, unit: "porción", assumed, candidates: candShort,
      });
    } else {
      out.push({
        label: chosen.name, kind: "food", foodId: chosen.id, recipeId: null,
        qty: Number.isFinite(qty) && qty > 0 ? qty : 100, unit, assumed,
        candidates: candShort,
      });
    }
  }
  if (out.length === 0)
    questions.push("No reconocí comida en la foto. Describila o cargala manual.");
  return { items: out, questions };
}

function extraerJSON(text: string): unknown {
  const clean = text.replace(/```json|```/g, "").trim();
  const start = clean.indexOf("{");
  const end = clean.lastIndexOf("}");
  if (start < 0 || end <= start) throw new Error("sin JSON");
  return JSON.parse(clean.slice(start, end + 1));
}

async function identificarOllama(b64: string, refs: FoodRef[]) {
  const url = process.env.OLLAMA_URL!;
  const model = process.env.OLLAMA_MODEL ?? "moondream";
  const res = await fetch(`${url}/api/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      model,
      messages: [{ role: "user", content: PROMPT, images: [b64] }],
      stream: false,
      format: "json",
      options: { temperature: 0, num_predict: 400 },
    }),
    signal: AbortSignal.timeout(120000),
  });
  if (!res.ok) throw new Error(`ollama ${res.status}`);
  const data = await res.json();
  return mapear(extraerJSON(String(data?.message?.content ?? "")), refs);
}

async function identificarGemini(b64: string, mime: string, refs: FoodRef[]) {
  const key = process.env.GEMINI_API_KEY!;
  const model = process.env.VISION_MODEL ?? "gemini-2.0-flash";
  const res = await fetch(
    `https://generativelanguage.googleapis.com/v1beta/models/${model}:generateContent`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", "x-goog-api-key": key },
      body: JSON.stringify({
        contents: [
          { parts: [{ text: PROMPT }, { inline_data: { mime_type: mime, data: b64 } }] },
        ],
        generationConfig: { temperature: 0, maxOutputTokens: 500, responseMimeType: "application/json" },
      }),
      signal: AbortSignal.timeout(120000),
    },
  );
  if (!res.ok) throw new Error(`gemini ${res.status}`);
  const data = await res.json();
  const text = (data?.candidates?.[0]?.content?.parts ?? [])
    .map((p: { text?: string }) => p.text ?? "")
    .join("");
  return mapear(extraerJSON(text), refs);
}

async function identificarGroq(b64: string, mime: string, refs: FoodRef[]) {
  const key = process.env.GROQ_API_KEY!;
  const model = process.env.VISION_MODEL ?? "meta-llama/llama-4-scout-17b-16e-instruct";
  const res = await fetch("https://api.groq.com/openai/v1/chat/completions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${key}`,
    },
    body: JSON.stringify({
      model,
      messages: [
        {
          role: "user",
          content: [
            { type: "text", text: PROMPT },
            { type: "image_url", image_url: { url: `data:${mime};base64,${b64}` } },
          ],
        },
      ],
      temperature: 0,
      max_tokens: 500,
      response_format: { type: "json_object" },
    }),
    signal: AbortSignal.timeout(120000),
  });
  if (!res.ok) throw new Error(`groq ${res.status}`);
  const data = await res.json();
  return mapear(extraerJSON(String(data?.choices?.[0]?.message?.content ?? "")), refs);
}

export async function identificarFoto(
  imageB64: string,
  mime: string,
  refs: FoodRef[],
): Promise<{ items: ParsedItem[]; questions: string[] }> {
  const p = provider();
  if (p === "gemini" && process.env.GEMINI_API_KEY)
    return identificarGemini(imageB64, mime, refs);
  if (p === "groq" && process.env.GROQ_API_KEY)
    return identificarGroq(imageB64, mime, refs);
  if (p === "ollama" && process.env.OLLAMA_URL)
    return identificarOllama(imageB64, refs);
  return {
    items: [],
    questions: ["Visión no configurada: describí la foto o cargala manual."],
  };
}
