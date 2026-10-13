// Backfill único: azúcares (g) y sodio (mg) por 100 g para los alimentos base.
// Valores de referencia estándar, marcados como manual (estimación) en la tabla.
// Corre con: npx tsx scripts/backfill-azucar-sodio.mts
import postgres from "postgres";
import { readFileSync, writeFileSync } from "fs";
import { join } from "path";

const CSV = join(import.meta.dirname, "..", "..", "data", "alimentos.csv");

// [azúcar g, sodio mg] por 100 g
const V: Record<string, [number, number]> = {
  "Pollo pechuga": [0, 74],
  "Huevo": [1.1, 142],
  "Palta": [0.7, 7],
  "Arroz blanco cocido": [0.1, 1],
  "Pan blanco": [5, 490],
  "Leche entera": [5.1, 44],
  "Queso cremoso": [3, 350],
  "Aceite de oliva": [0, 2],
  "Carne vacuna magra": [0, 72],
  "Papa": [0.8, 6],
  "Fideos secos": [2.7, 6],
  "Manzana": [10.4, 1],
  "Banana": [12.2, 1],
  "Avena": [1, 2],
  "Yogur natural": [4.7, 46],
  "Manteca": [0.1, 643],
  "Tomate": [2.6, 5],
  "Lechuga": [0.8, 28],
  "Cebolla": [4.2, 4],
  "Azúcar": [100, 0],
  "Miel": [82, 4],
  "Chocolate amargo 70%": [24, 12],
  "Queso duro": [1, 1500],
  "Queso crema": [3.3, 320],
  "Jamón cocido": [1, 1200],
  "Harina de almendra": [4.3, 1],
  "Yogur griego sin azúcar": [3.2, 36],
  "Nueces": [2.6, 2],
  "Arándanos": [10, 1],
  "Atún al natural escurrido": [0, 350],
  "Zapallo": [3, 2],
  "Bife de chorizo": [0, 65],
  "Coliflor": [1.9, 30],
  "Carne picada": [0, 70],
  "Zapallitos": [2.5, 8],
  "Merluza": [0, 90],
  "Chocolate 85%": [13, 10],
  "Pimiento": [4.2, 4],
  "Tamari": [5, 5000],
  "Vinagre de manzana": [0.4, 5],
  "Limón": [2.5, 2],
  "Ajo": [1, 17],
  "Orégano seco": [4, 15],
  "Perejil": [0.9, 56],
  "Chimichurri": [1, 800],
  "Mollejas": [0, 90],
  "Mate": [0.2, 10],
  "Café negro": [0, 2],
  "Té": [0.2, 3],
};

const lines = readFileSync(CSV, "utf8").split("\n");
const head = lines[0];
let n = 0;
const missing: string[] = [];
for (let i = 1; i < lines.length; i++) {
  if (!lines[i].trim()) continue;
  const m = lines[i].match(/^"((?:[^"]|"")*)"/);
  if (!m) continue;
  const name = m[1].replace(/""/g, '"');
  const v = V[name];
  if (!v) {
    missing.push(name);
    continue;
  }
  // agrega las 2 columnas finales si faltan
  const cols = lines[i].split('","');
  while (cols.length < 10) cols.push("");
  cols[8] = `"${v[0]}"`;
  cols[9] = `"${v[1]}"`;
  lines[i] = cols.join('","');
  n++;
}
writeFileSync(CSV, lines.join("\n"));
console.log(`backfill: ${n} filas`);
if (missing.length) console.log("sin datos:", missing.join(" | "));

// sincroniza a DB vía load-csv (mismo proceso, simple)
const sql = postgres(process.env.DATABASE_URL!);
const { drizzle } = await import("drizzle-orm/postgres-js");
const { eq } = await import("drizzle-orm");
const { foods } = await import("../src/db/schema.js");
const db = drizzle(sql);
let upd = 0;
for (const [name, [sugar, sodium]] of Object.entries(V)) {
  const r = await db
    .update(foods)
    .set({ sugar100: String(sugar), sodiumMg100: String(sodium) })
    .where(eq(foods.name, name))
    .returning({ id: foods.id });
  if (r.length) upd++;
}
await sql.end();
console.log(`db: ${upd} actualizados`);
void head;
