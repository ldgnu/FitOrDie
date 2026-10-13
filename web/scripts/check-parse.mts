// Unit tests del parser de texto libre (puro, sin DB).
// Corre con: npx tsx scripts/check-parse.mts
import { parseMealText, type FoodRef } from "../src/lib/parse-food.js";

const REFS: FoodRef[] = [
  { id: "huevo", name: "Huevo", kind: "food" },
  { id: "palta", name: "Palta", kind: "food" },
  { id: "aceite", name: "Aceite de oliva", kind: "food" },
  { id: "pollo", name: "Pollo pechuga", kind: "food" },
  { id: "qd", name: "Queso duro", kind: "food" },
  { id: "qc", name: "Queso crema", kind: "food" },
  { id: "qcr", name: "Queso cremoso", kind: "food" },
  { id: "mila", name: "Milanesa keto de pollo", kind: "recipe" },
  { id: "mate", name: "Mate", kind: "food" },
];

let failures = 0;
function eq(name: string, got: unknown, want: unknown) {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  console.log(`${ok ? "ok" : "FAIL"} ${name}${ok ? "" : `: got=${JSON.stringify(got)} want=${JSON.stringify(want)}`}`);
  if (!ok) failures++;
}

// La frase de Javi
const r = parseMealText(
  "hoy desayune 4 huevos con una palta chica echa aderezo con aceite de la buena no la fea",
  REFS, "2026-10-11",
);
eq("frase: meal desayuno", r.meal, "desayuno");
eq("frase: 3 items", r.items.length, 3);
eq("frase: huevos 200 g", [r.items[0].qty, r.items[0].unit, r.items[0].foodId], [200, "g", "huevo"]);
eq("frase: palta 105 g", [r.items[1].qty, r.items[1].unit, r.items[1].foodId], [105, "g", "palta"]);
eq("frase: aceite 10 ml asumido", [r.items[2].qty, r.items[2].unit, r.items[2].assumed.length > 0], [10, "ml", true]);

eq("pollo 300 g", (() => {
  const x = parseMealText("almorcé 300g de pollo", REFS, "2026-10-11");
  return [x.meal, x.items[0]?.qty, x.items[0]?.unit, x.items[0]?.foodId];
})(), ["almuerzo", 300, "g", "pollo"]);

eq("dos huevos", (() => {
  const x = parseMealText("dos huevos", REFS, "2026-10-11");
  return [x.items[0]?.qty, x.items[0]?.foodId];
})(), [100, "huevo"]);

eq("queso ambiguo → candidatos + pregunta", (() => {
  const x = parseMealText("50 g de queso", REFS, "2026-10-11");
  return [(x.items[0]?.candidates?.length ?? 0) >= 2, x.questions.length > 0];
})(), [true, true]);

eq("desconocido → custom + pregunta", (() => {
  const x = parseMealText("cené pizza", REFS, "2026-10-11");
  return [x.meal, x.items[0]?.kind, x.questions.length > 0];
})(), ["cena", "custom", true]);

eq("receta por nombre", (() => {
  const x = parseMealText("una milanesa", REFS, "2026-10-11");
  return [x.items[0]?.kind, x.items[0]?.qty, x.items[0]?.unit];
})(), ["recipe", 1, "porción"]);

eq("vacío 400-like", parseMealText("   ", REFS, "2026-10-11").questions.length > 0, true);

eq("multilinea de Javi", (() => {
  const x = parseMealText("4 huevos revueltos\n60 gr de queso cremoso\nmate", REFS, "2026-10-11");
  return [
    x.items.length,
    x.items[0]?.qty, x.items[0]?.foodId,
    x.items[1]?.qty, x.items[1]?.unit,
    x.items[2]?.qty, x.items[2]?.unit,
  ];
})(), [3, 200, "huevo", 60, "g", 500, "ml"]);

eq("cucharada", (() => {
  const x = parseMealText("1 cda de aceite", REFS, "2026-10-11");
  return [x.items[0]?.qty, x.items[0]?.unit];
})(), [15, "g"]);

if (failures > 0) {
  console.error(`${failures} checks fallaron`);
  process.exit(1);
}
console.log("parser: todo ok");
