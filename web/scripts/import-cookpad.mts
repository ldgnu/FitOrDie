// Importa recetas keto desde Cookpad Argentina (búsqueda pública, sin login).
// Por copyright: guarda título + ingredientes (datos) + foto + link a la
// fuente. NO copia el paso a paso (vive en Cookpad, linkeado).
// Categoriza con reglas + matchea ingredientes con la tabla (nutrición real
// donde se puede). Idempotente por source_url.
//
// Uso: npx tsx scripts/import-cookpad.mts [--dry] [--max=N]
// Requiere DATABASE_URL. Cortesía: ~800ms entre páginas.
import postgres from "postgres";
import { drizzle } from "drizzle-orm/postgres-js";
import { eq } from "drizzle-orm";
import { foods, recipeIngredients, recipes } from "../src/db/schema.js";
import { parseMealText } from "../src/lib/parse-food.js";

const BASE = "https://cookpad.com";
const UA = { "User-Agent": "FitOrDie/1.0 (+https://github.com/ldgnu/FitOrDie)" };
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

const args = process.argv.slice(2);
const DRY = args.includes("--dry");
const MAX = Number(args.find((a) => a.startsWith("--max="))?.split("=")[1] ?? 1000);

const CARNE = /pollo|carne|pescado|muslo|pechuga|bife|lomo|asado|cordero|cerdo|chorizo|vacio|matambre|trucha|merluza|atun|salmon|lengua|molleja|rinon|peceto|vitel|tira|costillar|mejilla|venado|achura|milanesa|hamburguesa/;

function categoria(title: string): string {
  const t = title
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "");
  if (/\bpizza\b/.test(t)) return "cena";
  if (/\bempanada\b/.test(t)) return "almuerzo";
  if (/\bpan\b|chipa|bizcoch|rosquit|pancito|pancitos|focaccia|grisines|scon/.test(t)) return "merienda";
  if (/pascualina/.test(t)) return "almuerzo";
  if (/postre|torta|tortita|brownie|helado|budin|dulce|factura|galleta|galletita|alfajor|flan|mousse/.test(t)) return "bajon";
  if (/\bdesayuno\b/.test(t)) return "desayuno";
  if (/\bhuevos?\b|\byogur\b|\bavena\b|\bgranola\b|\bpanqueque\b|pancake|panqueca|\btostada\b|\bomelette\b|\bmuffin\b/.test(t)) return "desayuno";
  if (/\bsnack\b|picada|\bdip\b|bocad|antojo|\bchip\b/.test(t) && !CARNE.test(t)) return "bajon";
  if (/\bcena\b/.test(t)) return "cena";
  return "almuerzo";
}

const strip = (s: string) =>
  s.replace(/<script[\s\S]*?<\/script>/g, " ").replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();

interface Card {
  id: string;
  title: string;
  ings: string[];
  minutes: number | null;
  servings: number;
  img: string | null;
}

function decode(s: string): string {
  return s
    .replace(/&#(\d+);/g, (_, n) => String.fromCharCode(Number(n)))
    .replace(/&amp;/g, "&")
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&nbsp;/g, " ");
}

function parseCards(html: string): Card[] {
  const out: Card[] = [];
  const chunks = html.split('tracking-target="result"').slice(1);
  for (const block of chunks) {
    const href = block.match(/href="(\/ar\/recetas\/\d+)"/);
    if (!href) continue;
    const id = href[1].split("/").pop()!;
    const h2 = block.match(/<h2[^>]*>([\s\S]*?)<\/h2>/);
    const title = h2 ? decode(strip(h2[1])).slice(0, 200) : "";
    if (!title || /recetas ya cocinadas/i.test(title)) continue;
    const imgs = [...block.matchAll(/<img[^>]+src="(https:\/\/img-global.cpcdn\.com\/[^"]+)"/g)].map((x) => x[1]);
    const img0 = imgs.find((u) => !u.includes("avatar")) ?? imgs[0] ?? null;
    const img = img0 ? img0.replace(/\/\d+x\d+/, "/640x480") : null;
    const txt = strip(block);
    const saveIdx = txt.indexOf("cuando la quieras cocinar.");
    const tail = saveIdx >= 0 ? txt.slice(saveIdx) : txt;
    const cand: Array<{ i: number; v: number }> = [];
    for (const mm of tail.matchAll(/(\d+)\s*h\b(?:\s*(\d+))?/g)) {
      if (mm.index !== undefined) cand.push({ i: mm.index, v: Number(mm[1]) * 60 + Number(mm[2] ?? 0) });
    }
    for (const mm of tail.matchAll(/(\d+)\s*(minutos|min\b|')/g)) {
      if (mm.index !== undefined) cand.push({ i: mm.index, v: Number(mm[1]) });
    }
    cand.sort((a, b) => a.i - b.i);
    const lastC = cand[cand.length - 1];
    const minutes: number | null = lastC ? lastC.v : null;
    const timeIdx = lastC ? lastC.i : tail.length;
    const ings = tail
      .slice(0, timeIdx)
      .split("•")
      .map((s) => decode(s.replace(/^[^\p{L}\p{N}]+/u, "").trim()))
      .filter((s) => s.length > 1 && s.length < 300 && !/guarda esta receta|cuando la quieras/i.test(s));
    let servM = tail
      .slice(timeIdx, timeIdx + 150)
      .match(/(\d+)\s*(raciones|porciones|pancitos|unidades|vasos|platos|ración|porción)/i);
    if (!servM) {
      servM = tail
        .slice(-250)
        .match(/(\d+)\s*(raciones|porciones|pancitos|unidades|vasos|platos|ración|porción)/i);
    }
    const servings = servM ? Number(servM[1]) : 1;
    out.push({ id, title, ings, minutes, servings, img });
  }
  const seen = new Set<string>();
  return out.filter((c) => (seen.has(c.id) ? false : (seen.add(c.id), true)));
}

async function get(url: string): Promise<string> {
  let last: unknown = new Error("sin intentos");
  for (let i = 0; i < 4; i++) {
    try {
      const r = await fetch(url, { headers: UA });
      if (r.ok) return r.text();
      last = new Error(`HTTP ${r.status}`);
      if (r.status !== 502 && r.status !== 503 && r.status !== 429) throw last;
    } catch (e) {
      last = e;
    }
    await sleep(4000 * (i + 1));
  }
  throw last;
}

async function main() {
  const sql = postgres(process.env.DATABASE_URL!);
  const db = drizzle(sql);

  const foodRows = await db.select({ id: foods.id, name: foods.name }).from(foods);
  const refs = foodRows.map((f) => ({ id: f.id, name: f.name, kind: "food" as const }));

  let nuevas = 0;
  let vinculados = 0;
  let libres = 0;
  let page = 1;
  const cats: Record<string, number> = {};

  for (;;) {
    const html = await get(`${BASE}/ar/buscar/keto?page=${page}`);
    const cards = parseCards(html);
    if (cards.length === 0) {
      console.log(`página ${page}: sin cards, corto`);
      break;
    }
    for (const c of cards) {
      if (nuevas >= MAX) break;
      const sourceUrl = `${BASE}/ar/recetas/${c.id}`;
      if (!DRY) {
        const ex = await db.select({ id: recipes.id }).from(recipes).where(eq(recipes.sourceUrl, sourceUrl)).limit(1);
        if (ex.length > 0) continue;
      }
      const cat = categoria(c.title);
      cats[cat] = (cats[cat] ?? 0) + 1;
      const difficulty = c.minutes === null ? "media" : c.minutes <= 20 ? "facil" : c.minutes <= 45 ? "media" : "dificil";

      // Hotlink al CDN de Cookpad (verificado 200): 1000 fotos no entran al repo.
      const photoUrl: string | null = c.img ?? null;

      if (DRY) {
        console.log(`[dry] ${c.title.slice(0, 60)} | ${cat} | ${c.servings}p | ${c.minutes ?? "?"}min | ${c.ings.length}ings | foto:${photoUrl ? "sí" : "no"}`);
        nuevas++;
        continue;
      }
      const ins = await db
        .insert(recipes)
        .values({
          title: c.title,
          description: null,
          servings: c.servings,
          minutes: c.minutes,
          difficulty,
          instructions: "Paso a paso en la receta original de Cookpad (link abajo).",
          photoUrl,
          storageTip: null,
          isPublic: true,
          categoria: cat,
          categoriaAuto: true,
          videoUrl: null,
          sourceUrl,
          sourceSite: "cookpad",
          sourceLikes: 0,
          createdBy: null,
        })
        .returning({ id: recipes.id });
      const rid = ins[0].id;
      for (const line of c.ings) {
        const p = parseMealText(line, refs, "2000-01-01");
        const one = p.items.length === 1 && p.items[0].kind === "food" && p.items[0].foodId ? p.items[0] : null;
        if (one?.foodId) {
          await db.insert(recipeIngredients).values({ recipeId: rid, foodId: one.foodId, qty: String(one.qty), unit: one.unit });
          vinculados++;
        } else {
          await db.insert(recipeIngredients).values({ recipeId: rid, foodId: null, freeText: line.slice(0, 300), qty: "1", unit: "unidad" });
          libres++;
        }
      }
      nuevas++;
      if (nuevas % 50 === 0) console.log(`... ${nuevas} (pág ${page})`);
    }
    if (nuevas >= MAX) break;
    if (!/rel="next"/.test(html)) {
      console.log(`página ${page}: sin next, corto`);
      break;
    }
    page++;
    await sleep(2000);
  }

  console.log(`\nhecho: ${nuevas} nuevas | vinculados ${vinculados} / libres ${libres}`);
  console.log("categorías:", JSON.stringify(cats));
  await sql.end();
}

main().catch((e) => {
  console.error("FALLO:", e instanceof Error ? e.message : e);
  process.exit(1);
});
