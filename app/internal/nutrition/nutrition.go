// Package nutrition implementa el motor de cálculo de macros de FitOrDie.
//
// Todas las funciones del paquete son puras: no acceden a base de datos, red,
// reloj ni estado global. Eso las hace testeables de forma determinista y
// reutilizables tanto al cargar una comida como al agregar totales del día.
package nutrition

import (
	"fmt"
	"math"
)

// Per100 son los valores nutricionales de un alimento por cada 100 gramos.
// Es la unidad en la que vive la tabla `foods` (columnas *100 del CSV).
type Per100 struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sugar   float64 `json:"sugar"`
	Sodium  float64 `json:"sodium"`
}

// Macros son los valores absolutos de una porción (ya escalados por gramos).
// El `Sodium` se expresa en miligramos; el resto en gramos (kcal en kcal).
type Macros struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sugar   float64 `json:"sugar"`
	Sodium  float64 `json:"sodium"`
}

// Zero es la porción nula. Útil para acumuladores.
var Zero = Macros{}

// Scale devuelve los macros absolutos de `grams` gramos de un alimento cuyos
// valores están expresados por 100 g.
//
// Un peso nulo o negativo no tiene sentido nutricional: devuelve Zero en vez
// de propagar valores negativos a los totales del día.
func Scale(p Per100, grams float64) Macros {
	if grams <= 0 || math.IsNaN(grams) || math.IsInf(grams, 0) {
		return Zero
	}
	f := grams / 100
	return Macros{
		Kcal:    p.Kcal * f,
		Protein: p.Protein * f,
		Fat:     p.Fat * f,
		Carbs:   p.Carbs * f,
		Fiber:   p.Fiber * f,
		Sugar:   p.Sugar * f,
		Sodium:  p.Sodium * f,
	}
}

// Macros es un alias semántico de Scale (porción de un Per100).
func (p Per100) Macros(grams float64) Macros { return Scale(p, grams) }

// ValidatePer100 valida que los valores por 100 g sean posibles. Vive acá y no
// en la API porque también lo usa el panel web al cargar a mano.
func ValidatePer100(p Per100) error {
	checks := []struct {
		name string
		val  float64
		max  float64
	}{
		{"kcal", p.Kcal, 950},
		{"protein", p.Protein, 100},
		{"fat", p.Fat, 100},
		{"carbs", p.Carbs, 100},
		{"fiber", p.Fiber, 100},
		{"sugar", p.Sugar, 100},
		{"sodium", p.Sodium, 100000},
	}
	for _, c := range checks {
		if c.val < 0 || c.val > c.max || math.IsNaN(c.val) {
			return fmt.Errorf("per100.%s fuera de rango (0-%v)", c.name, c.max)
		}
	}
	return nil
}

// Add suma dos porciones campo por campo.
func (m Macros) Add(o Macros) Macros {
	return Macros{
		Kcal:    m.Kcal + o.Kcal,
		Protein: m.Protein + o.Protein,
		Fat:     m.Fat + o.Fat,
		Carbs:   m.Carbs + o.Carbs,
		Fiber:   m.Fiber + o.Fiber,
		Sugar:   m.Sugar + o.Sugar,
		Sodium:  m.Sodium + o.Sodium,
	}
}

// Sub resta dos porciones campo por campo (consumido - objetivo, etc.).
func (m Macros) Sub(o Macros) Macros {
	return Macros{
		Kcal:    m.Kcal - o.Kcal,
		Protein: m.Protein - o.Protein,
		Fat:     m.Fat - o.Fat,
		Carbs:   m.Carbs - o.Carbs,
		Fiber:   m.Fiber - o.Fiber,
		Sugar:   m.Sugar - o.Sugar,
		Sodium:  m.Sodium - o.Sodium,
	}
}

// Sum acumula cualquier cantidad de porciones. Sin argumentos devuelve Zero.
func Sum(ms ...Macros) Macros {
	acc := Zero
	for _, m := range ms {
		acc = acc.Add(m)
	}
	return acc
}

// IsZero informa si la porción es exactamente nula.
func (m Macros) IsZero() bool { return m == Zero }

// KcalFromMacros calcula las kcal teóricas con los factores de Atwater
// (4 kcal/g proteína y carbohidrato, 9 kcal/g grasa). Sirve para detectar
// filas del CSV con kcal incoherentes, nunca para reemplazar el valor medido.
func KcalFromMacros(m Macros) float64 {
	return 4*m.Protein + 4*m.Carbs + 9*m.Fat
}

// Rounded redondea kcal a entero y el resto a un decimal, con epsilon para
// evitar que 4.05 se convierta en 4.0 por representación binaria.
func (m Macros) Rounded() Macros {
	return Macros{
		Kcal:    math.Round(m.Kcal),
		Protein: round1(m.Protein),
		Fat:     round1(m.Fat),
		Carbs:   round1(m.Carbs),
		Fiber:   round1(m.Fiber),
		Sugar:   round1(m.Sugar),
		Sodium:  math.Round(m.Sodium),
	}
}

func round1(v float64) float64 { return math.Round(v*10+math.Copysign(1e-9, v)) / 10 }

// Targets son los objetivos diarios del perfil. Kcal y proteína son rangos
// (mínimo/máximo); carbos y grasas son referencia simple.
type Targets struct {
	KcalMin    float64 `json:"kcal_min"`
	KcalMax    float64 `json:"kcal_max"`
	ProteinMin float64 `json:"protein_min"`
	ProteinMax float64 `json:"protein_max"`
	CarbTarget float64 `json:"carb_target"`
	FatTarget  float64 `json:"fat_target"`
}

// Status clasifica un consumo contra un rango objetivo.
type Status string

const (
	StatusLow  Status = "bajo"
	StatusOK   Status = "ok"
	StatusHigh Status = "alto"
)

// KcalStatus clasifica las kcal consumidas contra el rango calórico.
func (t Targets) KcalStatus(consumed float64) Status {
	return t.status(consumed, t.KcalMin, t.KcalMax)
}

// ProteinStatus clasifica la proteína consumida contra el rango proteico.
func (t Targets) ProteinStatus(consumed float64) Status {
	return t.status(consumed, t.ProteinMin, t.ProteinMax)
}

func (t Targets) status(v, min, max float64) Status {
	switch {
	case min > 0 && v < min:
		return StatusLow
	case max > 0 && v > max:
		return StatusHigh
	default:
		return StatusOK
	}
}

// Pct devuelve el porcentaje consumido sobre el objetivo, acotado a [0, 200].
// Un objetivo nulo devuelve 0 (evita división por cero en la UI).
func Pct(consumed, target float64) float64 {
	if target <= 0 {
		return 0
	}
	p := consumed / target * 100
	if p < 0 {
		return 0
	}
	if p > 200 {
		return 200
	}
	return p
}

// Remaining son las kcal que faltan para el mínimo del objetivo, nunca negativas.
func (t Targets) Remaining(consumed float64) float64 {
	r := t.KcalMin - consumed
	if r < 0 {
		return 0
	}
	return r
}
