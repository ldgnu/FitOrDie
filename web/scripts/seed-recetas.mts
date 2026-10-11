// Seed global del recetario keto base: alimentos + recetas.
// Todo entra como source=manual (estimación, a revisar). Idempotente por nombre.
// Corre con: npx tsx scripts/seed-recetas.mts (requiere DATABASE_URL)
import postgres from "postgres";
import { drizzle } from "drizzle-orm/postgres-js";
import { eq } from "drizzle-orm";
import { foods, recipeIngredients, recipes } from "../src/db/schema.js";

const sql = postgres(process.env.DATABASE_URL!);
const db = drizzle(sql);
const DETAIL =
  "Seed dev (recetario keto base): estimación redondeada. Revisar contra fuente real antes de prod.";

// nombre, kcal, prote, grasa, carbos, fibra — por 100 g
const FOODS: Array<[string, number, number, number, number, number]> = [
  ["Queso duro", 430, 38, 29, 4, 0],
  ["Queso crema", 340, 6, 34, 4, 0],
  ["Jamón cocido", 145, 18, 6, 1, 0],
  ["Harina de almendra", 590, 21, 50, 22, 12],
  ["Yogur griego sin azúcar", 97, 9, 5, 4, 0],
  ["Nueces", 654, 15, 65, 14, 7],
  ["Arándanos", 57, 0.7, 0.3, 14, 2.4],
  ["Atún al natural escurrido", 130, 29, 1, 0, 0],
  ["Zapallo", 40, 1, 0.3, 10, 1.5],
  ["Bife de chorizo", 250, 26, 16, 0, 0],
  ["Coliflor", 25, 1.9, 0.3, 5, 2],
  ["Carne picada", 240, 24, 16, 0, 0],
  ["Zapallitos", 20, 1, 0.3, 4, 1],
  ["Merluza", 90, 18, 1, 0, 0],
  ["Chocolate 85%", 590, 8, 46, 28, 11],
  ["Pimiento", 26, 1, 0.3, 5.3, 1.9],
  ["Tamari", 60, 10, 0, 6, 1],
  ["Vinagre de manzana", 21, 0, 0, 0.9, 0],
  ["Limón", 29, 1.1, 0.3, 9, 2.8],
  ["Ajo", 149, 6.4, 0.5, 33, 2.1],
  ["Orégano seco", 265, 9, 4, 69, 38],
  ["Perejil", 36, 3, 0.8, 6, 3.3],
  ["Chimichurri", 450, 1, 48, 4, 2],
  ["Mollejas", 230, 25, 14, 0, 0],
];

interface Ing {
  food: string;
  qty: number;
  unit: string;
}
interface Recipe {
  title: string;
  description: string;
  servings: number;
  minutes: number;
  difficulty: string;
  instructions: string;
  storageTip: string;
  ings: Ing[];
}

const RECIPES: Recipe[] = [
  {
    title: "Tortilla de 2 huevos con queso crema",
    description: "Rápida y llenadora.",
    servings: 1, minutes: 10, difficulty: "facil",
    instructions: "1. Batí los huevos con sal. 2. Sartén caliente con un poco de manteca o aceite. 3. Volcá, agregá el queso crema en el centro. 4. Doblá y dorá de los dos lados.",
    storageTip: "Mejor recién hecha.",
    ings: [
      { food: "Huevo", qty: 100, unit: "g" },
      { food: "Queso crema", qty: 30, unit: "g" },
    ],
  },
  {
    title: "Milanesa keto de pollo",
    description: "Empanada en harina de almendra y parmesano. Crocante sin harina.",
    servings: 1, minutes: 25, difficulty: "media",
    instructions: "1. Aplaná la pechuga. 2. Pasala por huevo batido con ajo. 3. Empaná en mezcla de harina de almendra, queso duro rallado y orégano. 4. Sartén fuerte con aceite hasta dorar.",
    storageTip: "Freezar ya empanadas, cocinar directo.",
    ings: [
      { food: "Pollo pechuga", qty: 200, unit: "g" },
      { food: "Huevo", qty: 50, unit: "g" },
      { food: "Harina de almendra", qty: 30, unit: "g" },
      { food: "Queso duro", qty: 20, unit: "g" },
      { food: "Ajo", qty: 5, unit: "g" },
      { food: "Orégano seco", qty: 2, unit: "g" },
    ],
  },
  {
    title: "Huevos revueltos con queso crema, jamón y palta",
    description: "Desayuno que llena de verdad.",
    servings: 1, minutes: 10, difficulty: "facil",
    instructions: "1. Sartén a fuego medio con manteca. 2. Huevos batidos, revolvé lento. 3. Sumá el queso crema y el jamón en tiras. 4. Serví con la palta al lado.",
    storageTip: "Solo recién hecho.",
    ings: [
      { food: "Huevo", qty: 150, unit: "g" },
      { food: "Queso crema", qty: 50, unit: "g" },
      { food: "Jamón cocido", qty: 40, unit: "g" },
      { food: "Palta", qty: 100, unit: "g" },
    ],
  },
  {
    title: "Yogur griego con nueces y arándanos",
    description: "Merienda dulce sin azúcar.",
    servings: 1, minutes: 5, difficulty: "facil",
    instructions: "Mezclá todo en un bowl. Listo.",
    storageTip: "Las nueces aparte hasta servir.",
    ings: [
      { food: "Yogur griego sin azúcar", qty: 180, unit: "g" },
      { food: "Nueces", qty: 30, unit: "g" },
      { food: "Arándanos", qty: 10, unit: "g" },
    ],
  },
  {
    title: "Tortilla de atún y palta",
    description: "Cena rápida con buena proteína.",
    servings: 1, minutes: 15, difficulty: "facil",
    instructions: "1. Batí los huevos. 2. Sartén caliente, volcá y agregá el atún escurrido. 3. Doblá, serví con palta y tomate.",
    storageTip: "Mejor recién hecha.",
    ings: [
      { food: "Huevo", qty: 100, unit: "g" },
      { food: "Atún al natural escurrido", qty: 120, unit: "g" },
      { food: "Palta", qty: 100, unit: "g" },
      { food: "Tomate", qty: 50, unit: "g" },
    ],
  },
  {
    title: "Crema de zapallo con pollo",
    description: "Cena de olla, rinde 2 porciones.",
    servings: 2, minutes: 30, difficulty: "media",
    instructions: "1. Herví el zapallo en cubos. 2. Salteá el pollo en tiras. 3. Mixeá el zapallo con el queso crema. 4. Serví con el pollo encima.",
    storageTip: "Heladera 3 días, freeza bien.",
    ings: [
      { food: "Zapallo", qty: 400, unit: "g" },
      { food: "Pollo pechuga", qty: 200, unit: "g" },
      { food: "Queso crema", qty: 50, unit: "g" },
    ],
  },
];

const have = new Set((await db.select({ name: foods.name }).from(foods)).map((r) => r.name));
for (const [name, kcal, protein, fat, carbs, fiber] of FOODS) {
  if (have.has(name)) continue;
  await db.insert(foods).values({
    name, source: "manual", sourceDetail: DETAIL,
    kcal100: String(kcal), protein100: String(protein), fat100: String(fat),
    carbs100: String(carbs), fiber100: String(fiber), createdBy: null,
  });
  console.log("+ food", name);
}

const byName = new Map((await db.select().from(foods)).map((f) => [f.name, f.id]));
for (const rec of RECIPES) {
  const ex = await db.select({ id: recipes.id }).from(recipes).where(eq(recipes.title, rec.title)).limit(1);
  if (ex.length > 0) {
    console.log("= recipe existe:", rec.title);
    continue;
  }
  const missing = rec.ings.filter((i) => !byName.has(i.food));
  if (missing.length > 0) throw new Error(`faltan alimentos: ${missing.map((m) => m.food).join(", ")}`);
  const ins = await db
    .insert(recipes)
    .values({
      title: rec.title, description: rec.description, servings: rec.servings,
      minutes: rec.minutes, difficulty: rec.difficulty, instructions: rec.instructions,
      storageTip: rec.storageTip, isPublic: true, createdBy: null,
    })
    .returning({ id: recipes.id });
  for (const ing of rec.ings) {
    await db.insert(recipeIngredients).values({
      recipeId: ins[0].id, foodId: byName.get(ing.food)!,
      qty: String(ing.qty), unit: ing.unit,
    });
  }
  console.log("+ recipe", rec.title);
}
await sql.end();
