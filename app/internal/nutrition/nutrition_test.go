package nutrition

import (
	"math"
	"testing"
)

func almostEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// arroz blanco cocido: 130 kcal / 2.7 P / 0.3 G / 28 C / 0.4 fibra / 0.1 azúcar / 1 Na por 100 g
var arroz = Per100{Kcal: 130, Protein: 2.7, Fat: 0.3, Carbs: 28, Fiber: 0.4, Sugar: 0.1, Sodium: 1}

var pollo = Per100{Kcal: 165, Protein: 31, Fat: 3.6, Carbs: 0, Fiber: 0, Sugar: 0, Sodium: 74}

func TestScaleByGrams(t *testing.T) {
	tests := []struct {
		name  string
		per   Per100
		grams float64
		want  Macros
	}{
		{
			name:  "150 g de arroz = 1.5x",
			per:   arroz,
			grams: 150,
			want:  Macros{Kcal: 195, Protein: 4.05, Fat: 0.45, Carbs: 42, Fiber: 0.6, Sugar: 0.15, Sodium: 1.5},
		},
		{
			name:  "100 g devuelve los valores por 100",
			per:   arroz,
			grams: 100,
			want:  Macros{Kcal: 130, Protein: 2.7, Fat: 0.3, Carbs: 28, Fiber: 0.4, Sugar: 0.1, Sodium: 1},
		},
		{
			name:  "250 g de pollo crudo",
			per:   pollo,
			grams: 250,
			want:  Macros{Kcal: 412.5, Protein: 77.5, Fat: 9, Carbs: 0, Fiber: 0, Sugar: 0, Sodium: 185},
		},
		{
			name:  "12.5 g de aceite de oliva",
			per:   Per100{Kcal: 884, Fat: 100},
			grams: 12.5,
			want:  Macros{Kcal: 110.5, Fat: 12.5},
		},
		{
			name:  "gramos fraccionarios",
			per:   pollo,
			grams: 37.5,
			want:  Macros{Kcal: 61.875, Protein: 11.625, Fat: 1.35, Sodium: 27.75},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scale(tt.per, tt.grams)
			// Los float no se comparan con ==: 0.45 * 1.5 no es exacto en binario.
			if !almostEq(got.Kcal, tt.want.Kcal) ||
				!almostEq(got.Protein, tt.want.Protein) ||
				!almostEq(got.Fat, tt.want.Fat) ||
				!almostEq(got.Carbs, tt.want.Carbs) ||
				!almostEq(got.Fiber, tt.want.Fiber) ||
				!almostEq(got.Sugar, tt.want.Sugar) ||
				!almostEq(got.Sodium, tt.want.Sodium) {
				t.Fatalf("Scale() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestScaleLinear(t *testing.T) {
	// El escalado tiene que ser lineal: 400 g = 4 x 100 g.
	one := Scale(arroz, 100)
	four := Scale(arroz, 400)
	want := Macros{
		Kcal:    one.Kcal * 4,
		Protein: one.Protein * 4,
		Fat:     one.Fat * 4,
		Carbs:   one.Carbs * 4,
		Fiber:   one.Fiber * 4,
		Sugar:   one.Sugar * 4,
		Sodium:  one.Sodium * 4,
	}
	if four != want {
		t.Fatalf("escalado no lineal: %+v, want %+v", four, want)
	}
}

func TestScaleInvalidGrams(t *testing.T) {
	for _, g := range []float64{0, -50, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := Scale(arroz, g); got != Zero {
			t.Errorf("Scale(%v gramos) = %+v, want Zero", g, got)
		}
	}
}

func TestSumMacros(t *testing.T) {
	desayuno := Scale(pollo, 250)                    // 412.5 kcal / 77.5 P
	almuerzo := Scale(arroz, 150)                    // 195 kcal / 4.05 P
	aceite := Scale(Per100{Kcal: 884, Fat: 100}, 10) // 88.4 kcal / 10 G

	got := Sum(desayuno, almuerzo, aceite)
	want := Macros{
		Kcal:    412.5 + 195 + 88.4,
		Protein: 77.5 + 4.05,
		Fat:     9 + 0.45 + 10,
		Carbs:   42,
		Fiber:   0.6,
		Sugar:   0.15,
		Sodium:  185 + 1.5,
	}
	if !almostEq(got.Kcal, want.Kcal) || !almostEq(got.Protein, want.Protein) ||
		!almostEq(got.Fat, want.Fat) || !almostEq(got.Carbs, want.Carbs) ||
		!almostEq(got.Fiber, want.Fiber) || !almostEq(got.Sugar, want.Sugar) ||
		!almostEq(got.Sodium, want.Sodium) {
		t.Fatalf("Sum() = %+v, want %+v", got, want)
	}
}

func TestSumEmptyAndOrder(t *testing.T) {
	if got := Sum(); got != Zero {
		t.Fatalf("Sum() sin argumentos = %+v, want Zero", got)
	}
	a := Scale(pollo, 250)
	b := Scale(arroz, 150)
	if Sum(a, b) != Sum(b, a) {
		t.Fatal("Sum debería ser conmutativa")
	}
	// Sumar en cadena equivale a Sum de todos los elementos.
	if Sum(Sum(a, b), b) != Sum(a, b, b) {
		t.Fatal("Sum no es asociativa")
	}
}

func TestAddSub(t *testing.T) {
	m := Macros{Kcal: 100, Protein: 10}
	got := m.Add(Macros{Kcal: 50, Protein: 5}).Sub(Macros{Kcal: 25, Protein: 2})
	want := Macros{Kcal: 125, Protein: 13}
	if got != want {
		t.Fatalf("Add/Sub = %+v, want %+v", got, want)
	}
}

func TestRounded(t *testing.T) {
	got := Macros{
		Kcal:    195.49,
		Protein: 4.05,
		Fat:     0.449, // 0.4, no 0.5: el redondeo es por valor, no por arrastre
		Carbs:   41.999999,
		Fiber:   0.25,
		Sugar:   0.15,
		Sodium:  1.4,
	}.Rounded()
	want := Macros{Kcal: 195, Protein: 4.1, Fat: 0.4, Carbs: 42, Fiber: 0.3, Sugar: 0.2, Sodium: 1}
	if got != want {
		t.Fatalf("Rounded() = %+v, want %+v", got, want)
	}
	// 0,45 exacto sí sube a 0,5 (half-up).
	if r := (Macros{Kcal: 0, Protein: 0.45}).Rounded(); r.Protein != 0.5 {
		t.Fatalf("0.45 debía redondear a 0.5, fue %v", r.Protein)
	}
}

func TestKcalFromMacros(t *testing.T) {
	// 30 P + 40 C + 10 G = 120 + 160 + 90 = 370 kcal
	got := KcalFromMacros(Macros{Protein: 30, Carbs: 40, Fat: 10})
	if got != 370 {
		t.Fatalf("KcalFromMacros() = %v, want 370", got)
	}
}

func TestTargetsStatus(t *testing.T) {
	tg := Targets{KcalMin: 2150, KcalMax: 2400, ProteinMin: 155, ProteinMax: 170}

	if got := tg.KcalStatus(2000); got != StatusLow {
		t.Errorf("2000 kcal: %s, want bajo", got)
	}
	if got := tg.KcalStatus(2150); got != StatusOK {
		t.Errorf("2150 kcal (borde): %s, want ok", got)
	}
	if got := tg.KcalStatus(2300); got != StatusOK {
		t.Errorf("2300 kcal: %s, want ok", got)
	}
	if got := tg.KcalStatus(2401); got != StatusHigh {
		t.Errorf("2401 kcal: %s, want alto", got)
	}
	if got := tg.ProteinStatus(150); got != StatusLow {
		t.Errorf("150 g proteína: %s, want bajo", got)
	}
	if got := tg.ProteinStatus(160); got != StatusOK {
		t.Errorf("160 g proteína: %s, want ok", got)
	}
	if got := tg.ProteinStatus(180); got != StatusHigh {
		t.Errorf("180 g proteína: %s, want alto", got)
	}
}

func TestPctAndRemaining(t *testing.T) {
	if got := Pct(1075, 2150); got != 50 {
		t.Errorf("Pct(1075, 2150) = %v, want 50", got)
	}
	if got := Pct(500, 0); got != 0 {
		t.Errorf("Pct con objetivo 0 = %v, want 0", got)
	}
	if got := Pct(9999, 100); got != 200 {
		t.Errorf("Pct debe acotar a 200, got %v", got)
	}
	tg := Targets{KcalMin: 2150}
	if got := tg.Remaining(2000); got != 150 {
		t.Errorf("Remaining(2000) = %v, want 150", got)
	}
	if got := tg.Remaining(2500); got != 0 {
		t.Errorf("Remaining(2500) = %v, want 0", got)
	}
}

func TestPer100MacrosAlias(t *testing.T) {
	if arroz.Macros(150) != Scale(arroz, 150) {
		t.Fatal("Per100.Macros debe equivaler a Scale")
	}
}
