// Seed DEV: alimentos base con valores redondeados (estimaciones).
// source=manual + detail honesto. Revisar contra fuente real antes de prod.
// Corre con: npx tsx scripts/seed.mts (requiere DATABASE_URL en el entorno)
import postgres from "postgres";
import { drizzle } from "drizzle-orm/postgres-js";
import { foods } from "../src/db/schema.js";

const sql = postgres(process.env.DATABASE_URL!);
const db = drizzle(sql);

// kcal, prote, grasa, carbos, fibra — por 100 g
const F: Array<[string, number, number, number, number, number]> = [
  ["Pollo pechuga", 165, 31, 3.6, 0, 0],
  ["Huevo", 155, 13, 11, 1.1, 0],
  ["Palta", 160, 2, 15, 9, 7],
  ["Arroz blanco cocido", 130, 2.7, 0.3, 28, 0.4],
  ["Pan blanco", 265, 9, 3.2, 49, 2.7],
  ["Leche entera", 61, 3.2, 3.3, 4.8, 0],
  ["Queso cremoso", 300, 18, 24, 2, 0],
  ["Aceite de oliva", 884, 0, 100, 0, 0],
  ["Carne vacuna magra", 170, 26, 7, 0, 0],
  ["Papa", 77, 2, 0.1, 17, 2.2],
  ["Fideos secos", 350, 12, 1.5, 73, 3],
  ["Manzana", 52, 0.3, 0.2, 14, 2.4],
  ["Banana", 89, 1.1, 0.3, 23, 2.6],
  ["Avena", 389, 17, 7, 66, 10.6],
  ["Yogur natural", 61, 3.5, 3.3, 4.7, 0],
  ["Manteca", 717, 0.9, 81, 0.1, 0],
  ["Tomate", 18, 0.9, 0.2, 3.9, 1.2],
  ["Lechuga", 15, 1.4, 0.2, 2.9, 1.3],
  ["Cebolla", 40, 1.1, 0.1, 9.3, 1.7],
  ["Azúcar", 387, 0, 0, 100, 0],
  ["Miel", 304, 0.3, 0, 82, 0.2],
  ["Chocolate amargo 70%", 598, 7.8, 42, 46, 11],
];

const existing = await db.select({ id: foods.id }).from(foods).limit(1);
if (existing.length === 0) {
  await db.insert(foods).values(
    F.map(([name, kcal, protein, fat, carbs, fiber]) => ({
      name,
      source: "manual",
      sourceDetail:
        "Seed dev: estimación redondeada. Revisar contra fuente real antes de prod.",
      kcal100: String(kcal),
      protein100: String(protein),
      fat100: String(fat),
      carbs100: String(carbs),
      fiber100: String(fiber),
      createdBy: null,
    })),
  );
  console.log(`seed: ${F.length} alimentos insertados`);
} else {
  console.log("seed: ya hay alimentos, no toco nada");
}
await sql.end();
