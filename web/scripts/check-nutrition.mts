// Sanity check del motor nutricional. Corre con: npx tsx scripts/check-nutrition.mts
import {
  ketoVerdict,
  netCarbs,
  scaleNutrients,
  sumNutrients,
} from "../src/lib/nutrition.js";

let failures = 0;
function eq(name: string, got: unknown, want: unknown) {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  console.log(`${ok ? "ok" : "FAIL"} ${name}: got=${JSON.stringify(got)} want=${JSON.stringify(want)}`);
  if (!ok) failures++;
}

// netos = totales − fibra
eq("netCarbs basico", netCarbs({ carbs: 10, fiber: 4 }), 6);
eq("netCarbs sin datos", netCarbs({ carbs: 10, fiber: null }), null);
eq("netCarbs no negativo", netCarbs({ carbs: 2, fiber: 5 }), 0);

// escalado por cantidad (100 g → 200 g duplica)
eq(
  "scale x2",
  scaleNutrients({ kcal: 100, protein: 10, fat: 5, carbs: 20, fiber: 4 }, 100, 200),
  { kcal: 200, protein: 20, fat: 10, carbs: 40, fiber: 8 },
);

// suma de ingredientes
eq(
  "sum",
  sumNutrients([
    { kcal: 100, protein: 10, fat: 5, carbs: 20, fiber: 4 },
    { kcal: 50, protein: 5, fat: 5, carbs: 0, fiber: 0 },
  ]),
  { kcal: 150, protein: 15, fat: 10, carbs: 20, fiber: 4 },
);

// veredicto keto (objetivo default 30 g)
eq("keto compatible", ketoVerdict(5), "compatible");
eq("keto con-ajustes", ketoVerdict(12), "con-ajustes");
eq("keto dificil", ketoVerdict(25), "dificil");
eq("keto sin-datos", ketoVerdict(null), "sin-datos");

if (failures > 0) {
  console.error(`${failures} checks fallaron`);
  process.exit(1);
}
console.log("motor nutricional: todo ok");
