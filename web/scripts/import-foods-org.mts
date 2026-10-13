// Importa recetas keto argentinas desde foods.org (permite scraping: robots Allow).
// Usa el JSON-LD schema.org de cada ficha: ingredientes, nutrición declarada,
// tiempos, porciones, foto. Matchea ingredientes con la tabla de alimentos
// (nutrición calculada real) y guarda el resto como texto libre (sin datos).
// Atribución siempre: source_url + sitio. Idempotente por source_url.
//
// Uso: npx tsx scripts/import-foods-org.mts [--dry] [--limit N]
// Requiere DATABASE_URL. Cortesía: ~600ms entre fichas.
import postgres from "postgres";
import { mkdir, writeFile } from "fs/promises";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { drizzle } from "drizzle-orm/postgres-js";
import { eq } from "drizzle-orm";
import { foods, recipeIngredients, recipes } from "../src/db/schema.js";
import { parseMealText } from "../src/lib/parse-food.js";

const BASE = "https://www.foods.org";
const LIST = process.env.IMPORT_URL ?? `${BASE}/es/ar/recipes?tag=keto`;
const UA = { "User-Agent": "FitOrDie/1.0 (+https://github.com/ldgnu/FitOrDie)" };
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const IMGDIR = join(ROOT, "web", "public", "recetas");

const args = process.argv.slice(2);
const DRY = args.includes("--dry");
const LIM = Number(args.find((a) => a.startsWith("--limit="))?.split("=")[1] ?? NaN);

const isoMin = (s: string): number => {
  const m = s.match(/PT(?:(\d+)H)?(?:(\d+)M)?/);
  if (!m) return 0;
  return (Number(m[1] ?? 0) * 60) + Number(m[2] ?? 0);
};

const CARNE = /pollo|carne|pescado|muslo|pechuga|bife|lomo|asado|cordero|cerdo|chorizo|vacio|matambre|trucha|merluza|atun|salmon|lengua|molleja|rinon|peceto|vitel|tira|costillar|mejilla|venado|achura|milanesa|hamburguesa/;

function categoria(title: string, desc: string, category: string): string {
  const t = `${title} ${category}`.toLowerCase();
  const d = `${title} ${desc} ${category}`.toLowerCase();
  if (/\bdesayuno\b|\bbreakfast\b/.test(d)) return "desayuno";
  if (/\bhuevos?\b|\byogur\b|\bavena\b|\bgranola\b|\bpanqueque\b|\btostada\b|\bomelette\b|\bmuffin\b/.test(t)) return "desayuno";
  if (/\bmerienda\b|\bmate\b|factura|\btorta\b|\bbudin\b|\bbizcoch/.test(t) && !CARNE.test(t)) return "merienda";
  if (/\bsnack\b|picada|\bdip\b|postre|\bdulce\b|galleta|helado|brownie|bocad|antojo|\bchip\b|bomb[oó]n|trufa|mousse|flan/.test(t)) return "bajon";
  if (/\bcena\b|livian|\blight\b/.test(d)) return "cena";
  return "almuerzo";
}

interface Card {
  url: string;
  slug: string;
}

async function get(url: string): Promise<string> {
  const r = await fetch(url, { headers: UA });
  if (!r.ok) throw new Error(`HTTP ${r.status} en ${url}`);
  return r.text();
}

async function main() {
  const sql = postgres(process.env.DATABASE_URL!);
  const db = drizzle(sql);

  const listHtml = await get(LIST);
  const seen = new Set<string>();
  const cards: Card[] = [];
  for (const m of listHtml.matchAll(/href="(\/es\/ar\/recipes\/[^"]+)"/g)) {
    const url = m[1];
    if (seen.has(url)) continue;
    seen.add(url);
    cards.push({ url, slug: url.split("/").pop()! });
  }
  console.log(`listado: ${cards.length} fichas`);
  const todo = Number.isFinite(LIM) ? cards.slice(0, LIM) : cards;

  const foodRows = await db.select({ id: foods.id, name: foods.name }).from(foods);
  const refs = foodRows.map((f) => ({ id: f.id, name: f.name, kind: "food" as const }));

  await mkdir(IMGDIR, { recursive: true });
  let nuevas = 0;
  let vinculados = 0;
  let libres = 0;
  const cats: Record<string, number> = {};

  for (const c of todo) {
    await sleep(600);
    let html: string;
    try {
      html = await get(BASE + c.url);
    } catch (e) {
      console.log(`- salteo ${c.slug}: ${e instanceof Error ? e.message : e}`);
      continue;
    }
    const jm = html.match(
      /<script type="application\/ld\+json">(\{"@context":"https:\/\/schema\.org","@type":"Recipe".*?)<\/script>/,
    );
    if (!jm) {
      console.log(`- salteo ${c.slug}: sin JSON-LD`);
      continue;
    }
    let R: Record<string, unknown>;
    try {
      R = JSON.parse(jm[1]);
    } catch {
      console.log(`- salteo ${c.slug}: JSON roto`);
      continue;
    }
    const kw = String((R.keywords as string) ?? "");
    if (!/keto/i.test(kw)) {
      console.log(`= salteo ${c.slug}: no taggeada keto`);
      continue;
    }
    const title = String(R.name ?? c.slug);
    const desc = String(R.description ?? "");
    const servings = Number(String(R.recipeYield ?? "1").match(/\d+/)?.[0] ?? 1) || 1;
    const minutes = isoMin(String(R.prepTime ?? "")) + isoMin(String(R.cookTime ?? ""));
    const difficulty = minutes <= 20 ? "facil" : minutes <= 45 ? "media" : "dificil";
    const steps = ((R.recipeInstructions as Array<{ text?: string }>) ?? [])
      .map((s, i) => `${i + 1}. ${s.text ?? ""}`)
      .join("\n");
    const nut = (R.nutrition as Record<string, string>) ?? {};
    const dec = {
      kcal: Number(String(nut.calories ?? "").match(/[\d.]+/)?.[0] ?? NaN),
      protein: Number(String(nut.proteinContent ?? "").match(/[\d.]+/)?.[0] ?? NaN),
      fat: Number(String(nut.fatContent ?? "").match(/[\d.]+/)?.[0] ?? NaN),
      carbs: Number(String(nut.carbohydrateContent ?? "").match(/[\d.]+/)?.[0] ?? NaN),
      fiber: Number(String(nut.fiberContent ?? "").match(/[\d.]+/)?.[0] ?? NaN),
    };
    const cat = categoria(title, desc, String(R.recipeCategory ?? ""));
    cats[cat] = (cats[cat] ?? 0) + 1;

    const likesM = html.match(/([\d.,]+)(?:<!-- -->|\s)*me gusta/);
    const likes = likesM ? Number(likesM[1].replace(/[.,]/g, "")) || 0 : 0;

    const imgM = html.match(/<meta property="og:image" content="([^"]+)"/);
    let photoUrl: string | null = null;
    if (imgM) {
      try {
        const ir = await fetch(imgM[1], { headers: UA });
        if (ir.ok) {
          const buf = Buffer.from(await ir.arrayBuffer());
          if (buf.length > 0 && buf.length < 2_000_000) {
            await writeFile(join(IMGDIR, `${c.slug}.jpg`), buf);
            photoUrl = `/recetas/${c.slug}.jpg`;
          }
        }
      } catch {
        /* sin foto, sigue igual */
      }
    }

    const ings = ((R.recipeIngredient as string[]) ?? []).map(String);
    if (DRY) {
      console.log(`[dry] ${title} | ${cat} | ${servings} porciones | ${minutes} min | ${ings.length} ings | ♥${likes} | foto:${photoUrl ? "sí" : "no"} | kcal:${dec.kcal}`);
      continue;
    }

    const ex = await db.select({ id: recipes.id }).from(recipes).where(eq(recipes.sourceUrl, BASE + c.url)).limit(1);
    if (ex.length > 0) {
      console.log(`= existe: ${title}`);
      continue;
    }
    const ins = await db
      .insert(recipes)
      .values({
        title: title.slice(0, 200),
        description: desc.slice(0, 1000) || null,
        servings,
        minutes: minutes || null,
        difficulty,
        instructions: steps || "Ver fuente original.",
        photoUrl,
        storageTip: null,
        isPublic: true,
        categoria: cat,
        categoriaAuto: true,
        videoUrl: null,
        sourceUrl: BASE + c.url,
        sourceSite: "foods.org",
        sourceLikes: likes,
        declaredKcal: Number.isFinite(dec.kcal) ? String(dec.kcal) : null,
        declaredProtein: Number.isFinite(dec.protein) ? String(dec.protein) : null,
        declaredFat: Number.isFinite(dec.fat) ? String(dec.fat) : null,
        declaredCarbs: Number.isFinite(dec.carbs) ? String(dec.carbs) : null,
        declaredFiber: Number.isFinite(dec.fiber) ? String(dec.fiber) : null,
        createdBy: null,
      })
      .returning({ id: recipes.id });
    const rid = ins[0].id;
    for (const line of ings) {
      const p = parseMealText(line, refs, "2000-01-01");
      const one =
        p.items.length === 1 && p.items[0].kind === "food" && p.items[0].foodId
          ? p.items[0]
          : null;
      if (one?.foodId) {
        await db.insert(recipeIngredients).values({
          recipeId: rid, foodId: one.foodId,
          qty: String(one.qty), unit: one.unit,
        });
        vinculados++;
      } else {
        await db.insert(recipeIngredients).values({
          recipeId: rid, foodId: null, freeText: line.slice(0, 300),
          qty: "1", unit: "unidad",
        });
        libres++;
      }
    }
    nuevas++;
    console.log(`+ ${title} [${cat}] (${ings.length} ings)`);
  }

  console.log(`\nhecho: ${nuevas} nuevas | ingredientes vinculados ${vinculados} / libres ${libres}`);
  console.log("categorías:", JSON.stringify(cats));
  await sql.end();
}

main().catch((e) => {
  console.error("FALLO:", e instanceof Error ? e.message : e);
  process.exit(1);
});
