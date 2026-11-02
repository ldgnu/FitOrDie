package parser

import (
	"math"
	"testing"

	"github.com/ldgnu/fitordie/internal/nutrition"
)

// testFoods replica los valores del CSV real para los alimentos que usan los
// casos de prueba (así el test no depende de SQLite).
func testFoods() []Food {
	return []Food{
		{ID: 1, Name: "Arroz blanco cocido", Per100: nutrition.Per100{Kcal: 130, Protein: 2.7, Fat: 0.3, Carbs: 28, Fiber: 0.4, Sugar: 0.1, Sodium: 1}},
		{ID: 2, Name: "Avena", Per100: nutrition.Per100{Kcal: 389, Protein: 16.9, Fat: 6.9, Carbs: 66, Fiber: 10.6, Sugar: 0.9, Sodium: 2}},
		{ID: 3, Name: "Banana", Per100: nutrition.Per100{Kcal: 89, Protein: 1.1, Fat: 0.3, Carbs: 23, Fiber: 2.6, Sugar: 12.2, Sodium: 1}},
		{ID: 4, Name: "Huevo", Per100: nutrition.Per100{Kcal: 155, Protein: 13, Fat: 11, Carbs: 1.1, Fiber: 0, Sugar: 1.1, Sodium: 142}},
		{ID: 5, Name: "Pollo pechuga", Per100: nutrition.Per100{Kcal: 165, Protein: 31, Fat: 3.6, Carbs: 0, Fiber: 0, Sugar: 0, Sodium: 74}},
		{ID: 6, Name: "Queso crema", Per100: nutrition.Per100{Kcal: 340, Protein: 6, Fat: 34, Carbs: 4, Fiber: 0, Sugar: 3.3, Sodium: 320}},
		{ID: 7, Name: "Queso duro", Per100: nutrition.Per100{Kcal: 430, Protein: 38, Fat: 29, Carbs: 4, Fiber: 0, Sugar: 1, Sodium: 1500}},
		{ID: 8, Name: "Tomate", Per100: nutrition.Per100{Kcal: 18, Protein: 0.9, Fat: 0.2, Carbs: 3.9, Fiber: 1.2, Sugar: 2.6, Sodium: 5}},
	}
}

func almost(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// findItem devuelve el item cuyo FoodName coincide.
func findItem(t *testing.T, items []Item, name string) Item {
	t.Helper()
	for _, it := range items {
		if it.FoodName == name {
			return it
		}
	}
	t.Fatalf("no se encontró el item %q en %+v", name, items)
	return Item{}
}

func TestParsePesoExplicitoPollo(t *testing.T) {
	res := Parse("250 g pollo", testFoods())
	if len(res.Items) != 1 {
		t.Fatalf("items: esperaba 1, obtuve %d (%+v)", len(res.Items), res.Items)
	}
	it := res.Items[0]
	if !it.Matched || it.FoodName != "Pollo pechuga" || it.FoodID != 5 {
		t.Fatalf("item inesperado: %+v", it)
	}
	if !almost(it.Grams, 250, 0.01) {
		t.Fatalf("gramos: esperaba 250, obtuve %v", it.Grams)
	}
	if !almost(it.Macros.Kcal, 412.5, 0.01) || !almost(it.Macros.Protein, 77.5, 0.01) {
		t.Fatalf("macros: esperaba 412.5 kcal / 77.5 P, obtuve %+v", it.Macros)
	}
	if it.Confidence < 0.8 {
		t.Fatalf("confianza baja para peso explícito: %v", it.Confidence)
	}
	if len(res.Unmatched) != 0 {
		t.Fatalf("unmatched inesperado: %+v", res.Unmatched)
	}
}

func TestParseDosHuevosUsaPorcionDeUnidad(t *testing.T) {
	res := Parse("2 huevos", testFoods())
	if len(res.Items) != 1 {
		t.Fatalf("items: esperaba 1, obtuve %d (%+v)", len(res.Items), res.Items)
	}
	it := res.Items[0]
	if it.FoodName != "Huevo" || !it.Matched {
		t.Fatalf("item inesperado: %+v", it)
	}
	// Huevo = 50 g por unidad -> 2 huevos = 100 g = 155 kcal.
	if !almost(it.Grams, 100, 0.01) {
		t.Fatalf("gramos: esperaba 100, obtuve %v", it.Grams)
	}
	if !almost(it.Macros.Kcal, 155, 0.01) {
		t.Fatalf("kcal: esperaba 155, obtuve %v", it.Macros.Kcal)
	}
}

func TestParseUnaBanana(t *testing.T) {
	res := Parse("1 banana", testFoods())
	if len(res.Items) != 1 {
		t.Fatalf("items: esperaba 1, obtuve %d (%+v)", len(res.Items), res.Items)
	}
	it := res.Items[0]
	if it.FoodName != "Banana" {
		t.Fatalf("item inesperado: %+v", it)
	}
	if !almost(it.Grams, 120, 0.01) {
		t.Fatalf("gramos: esperaba 120 (porción de tabla), obtuve %v", it.Grams)
	}
	if !almost(it.Macros.Kcal, 106.8, 0.05) {
		t.Fatalf("kcal: esperaba 106.8, obtuve %v", it.Macros.Kcal)
	}
}

func TestParseSinCantidadArrozYEnsalada(t *testing.T) {
	res := Parse("arroz y ensalada", testFoods())
	if len(res.Items) != 1 {
		t.Fatalf("items: esperaba 1 (arroz), obtuve %d (%+v)", len(res.Items), res.Items)
	}
	it := res.Items[0]
	if it.FoodName != "Arroz blanco cocido" {
		t.Fatalf("item inesperado: %+v", it)
	}
	if !almost(it.Grams, 150, 0.01) {
		t.Fatalf("gramos: esperaba 150 (porción de tabla), obtuve %v", it.Grams)
	}
	if it.Confidence > 0.8 {
		t.Fatalf("sin cantidad la confianza debe ser moderada, obtuve %v", it.Confidence)
	}
	if len(res.Unmatched) != 1 || res.Unmatched[0].Text != "ensalada" {
		t.Fatalf("unmatched: esperaba [ensalada], obtuve %+v", res.Unmatched)
	}
}

func TestParseTextoSinDatos(t *testing.T) {
	res := Parse("me fui a caminar por la costanera", testFoods())
	if len(res.Items) != 0 {
		t.Fatalf("items: esperaba 0, obtuve %+v", res.Items)
	}
	if !res.Totals.IsZero() {
		t.Fatalf("totales: esperaba cero, obtuve %+v", res.Totals)
	}
	if len(res.Unmatched) == 0 {
		t.Fatal("esperaba al menos un fragmento en unmatched")
	}
}

func TestParseFraseCompleta(t *testing.T) {
	res := Parse("comí 250 g de pollo con arroz y ensalada", testFoods())
	if len(res.Items) != 2 {
		t.Fatalf("items: esperaba 2 (pollo + arroz), obtuve %d (%+v)", len(res.Items), res.Items)
	}
	pollo := findItem(t, res.Items, "Pollo pechuga")
	arroz := findItem(t, res.Items, "Arroz blanco cocido")
	if !almost(pollo.Grams, 250, 0.01) || !almost(arroz.Grams, 150, 0.01) {
		t.Fatalf("gramos inesperados: pollo=%v arroz=%v", pollo.Grams, arroz.Grams)
	}
	// 412.5 (pollo) + 195 (arroz) = 607.5 kcal.
	if !almost(res.Totals.Kcal, 607.5, 0.6) {
		t.Fatalf("total kcal: esperaba ~607.5, obtuve %v", res.Totals.Kcal)
	}
	if len(res.Unmatched) != 1 || res.Unmatched[0].Text != "ensalada" {
		t.Fatalf("unmatched: esperaba ensalada, obtuve %+v", res.Unmatched)
	}
}

func TestParseMedidasCaserasYFracciones(t *testing.T) {
	cases := []struct {
		text  string
		food  string
		grams float64
	}{
		{"media taza de avena", "Avena", 100},
		{"1 taza de avena", "Avena", 200},
		{"1/2 taza de arroz", "Arroz blanco cocido", 100},
		{"1,5 kg de pollo", "Pollo pechuga", 1500},
	}
	for _, c := range cases {
		res := Parse(c.text, testFoods())
		if len(res.Items) != 1 {
			t.Fatalf("%q: esperaba 1 item, obtuve %+v", c.text, res.Items)
		}
		it := res.Items[0]
		if it.FoodName != c.food || !almost(it.Grams, c.grams, 0.01) {
			t.Fatalf("%q: esperaba %s %.1f g, obtuve %s %.1f g",
				c.text, c.food, c.grams, it.FoodName, it.Grams)
		}
	}
}

func TestMatchAmbiguedadBajaConfianza(t *testing.T) {
	// "queso" existe en dos alimentos: se elige uno pero con confianza baja.
	res := Parse("50 g de queso", testFoods())
	if len(res.Items) != 1 {
		t.Fatalf("items: esperaba 1, obtuve %+v", res.Items)
	}
	it := res.Items[0]
	if !it.Matched {
		t.Fatalf("queso debería matchear algún alimento: %+v", it)
	}
	if it.Confidence >= 0.9 {
		t.Fatalf("match ambiguo no debería tener confianza alta: %v", it.Confidence)
	}
}

func TestNormalizeYFolding(t *testing.T) {
	if got := normalize("Atún  AL NATURAL, escurrido"); got != "atun al natural escurrido" {
		t.Fatalf("normalize: %q", got)
	}
	if got := singular("huevos"); got != "huevo" {
		t.Fatalf("singular(huevos) = %q", got)
	}
}
