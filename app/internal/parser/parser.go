// Package parser convierte texto libre ("comí 250 g de pollo con arroz") en
// items de comida estructurados y comparables contra la tabla `foods`.
//
// Es determinista: no usa red ni IA, así que el mismo texto siempre da el mismo
// resultado y se puede testear. Un LLM opcional (capa API) puede proponer items
// distintos, pero el matching contra `foods` y el cálculo de macros siempre
// pasan por acá: si un alimento no está en la base, se devuelve matched=false y
// macros en cero, nunca valores inventados.
package parser

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ldgnu/fitordie/internal/nutrition"
)

// Food es el mínimo que necesita el parser: identidad, nombre y valores por
// 100 g (los mismos que viven en la tabla `foods`).
type Food struct {
	ID     int64
	Name   string
	Per100 nutrition.Per100
}

// Item es un alimento detectado en el texto, ya escalado a gramos.
type Item struct {
	FoodID     int64
	FoodName   string
	Grams      float64
	Macros     nutrition.Macros
	Matched    bool
	Confidence float64
	Quantity   string // "250 g", "2 unidades", "1 taza", "porción estimada"
	Input      string // fragmento original del texto
}

// Unmatched es un fragmento del texto que no se pudo asociar a ningún alimento.
type Unmatched struct {
	Text   string
	Reason string
}

// Result es el producto del parseo de un texto completo.
type Result struct {
	Items     []Item
	Unmatched []Unmatched
	Totals    nutrition.Macros
}

// Parse procesa el texto completo. Nunca devuelve error: lo que no se entiende
// va a Unmatched. Los totales solo suman items con matched=true.
func Parse(text string, foods []Food) Result {
	res := Result{Items: []Item{}, Unmatched: []Unmatched{}}

	for _, raw := range splitSegments(text) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		seg := normalize(raw)
		if seg == "" || isFiller(seg) {
			continue
		}

		item, ok := parseSegment(seg, foods)
		if !ok {
			res.Unmatched = append(res.Unmatched, Unmatched{
				Text:   raw,
				Reason: "sin coincidencia en foods",
			})
			continue
		}
		item.Input = raw
		res.Items = append(res.Items, item)
	}

	matched := make([]nutrition.Macros, 0, len(res.Items))
	for _, it := range res.Items {
		if it.Matched {
			matched = append(matched, it.Macros)
		}
	}
	// Sin redondear: la API redondea al serializar. El parser devuelve el valor
	// exacto del motor para no acumular error entre items y totales.
	res.Totals = nutrition.Sum(matched...)
	return res
}

// Match busca el alimento más parecido a query y devuelve la confianza del
// match (0-1). ok=false cuando nada supera el umbral mínimo.
func Match(query string, foods []Food) (Food, float64, bool) {
	return matchFood(normalize(query), foods)
}

// ServingGrams devuelve el peso típico de una porción/unidad del alimento.
// La segunda devolución indica si el valor es de tabla (true) o el default.
func ServingGrams(name string) (float64, bool) {
	return servingFor(name)
}

// ---------------------------------------------------------------- segmentos

// sepRe corta el texto en fragmentos de alimento. La coma solo separa si viene
// seguida de espacio (así "1,5 kg" queda entero) y no se corta por "/" porque
// se usa en fracciones ("1/2 taza").
var sepRe = regexp.MustCompile(`(?i)\s*(?:;\s*|\+\s*|&\s*|,\s+|\s+y\s+|\s+con\s+|\s+e\s+|\s+mas\s+|\s+and\s+)\s*`)

func splitSegments(text string) []string {
	return sepRe.Split(text, -1)
}

// fillerWords son verbos/muletillas que no aportan alimento.
var fillerWords = map[string]bool{
	"comi": true, "almorce": true, "cene": true, "merende": true,
	"desayune": true, "tome": true, "sumo": true, "agregue": true, "cargue": true,
	"hoy": true, "ayer": true, "anoche": true, "manana": true, "tarde": true,
	"noche": true, "de": true, "del": true, "la": true, "el": true, "los": true,
	"las": true, "un": true, "una": true, "unos": true, "unas": true, "mi": true,
	"me": true, "aprox": true, "aproximadamente": true, "poco": true, "poquito": true,
	"mas": true, "al": true, "para": true, "con": true, "y": true,
	"eso": true, "esto": true,
}

func isFiller(seg string) bool {
	toks := strings.Fields(seg)
	if len(toks) == 0 {
		return true
	}
	for _, t := range toks {
		if !fillerWords[t] {
			return false
		}
	}
	return true
}

// trimFillerPrefix saca muletillas del principio ("de pollo" -> "pollo").
func trimFillerPrefix(s string) string {
	for {
		toks := strings.Fields(s)
		if len(toks) == 0 {
			return ""
		}
		if !fillerWords[toks[0]] {
			return s
		}
		// "un"/"una" ya se consumieron como cantidad; acá solo sobran.
		s = strings.TrimSpace(strings.Join(toks[1:], " "))
	}
}

// ---------------------------------------------------------------- cantidades

type qtyKind int

const (
	qtyNone qtyKind = iota
	qtyWeight
	qtyMeasure
	qtyUnits
)

type qty struct {
	kind  qtyKind
	grams float64 // solo qtyWeight: gramos ya resueltos
	value float64 // qtyMeasure / qtyUnits: cantidad
	unit  string  // unidad de medida normalizada (singular)
	label string
}

var (
	fracRe       = regexp.MustCompile(`^(\d+)\s*/\s*(\d+)\b`)
	numRe        = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)`)
	weightUnitRe = regexp.MustCompile(`^(kilogramos?|kilos?|kg|gramos?|grs|gr|g)\b`)
	measureRe    = regexp.MustCompile(`^(cucharaditas?|cucharadas?|tazas?|vasos?|latas?|platos?|punados?|fetas?|rodajas?|porciones?|porcion|scoops?|unidades?|unid|u)\b`)
)

var wordNumbers = map[string]float64{
	"un": 1, "una": 1, "uno": 1,
	"dos": 2, "tres": 3, "cuatro": 4, "cinco": 5, "seis": 6,
	"siete": 7, "ocho": 8, "nueve": 9, "diez": 10,
	"medio": 0.5, "media": 0.5, "mitad": 0.5,
}

// measureGrams es el peso estándar de una medida casera.
var measureGrams = map[string]float64{
	"taza": 200, "cucharada": 15, "cucharadita": 5, "vaso": 250,
	"lata": 120, "plato": 300, "punado": 30, "feta": 30,
	"rodaja": 30, "porcion": 150, "scoop": 30,
}

// servingGrams es el peso típico de una "unidad" de cada alimento. Evita
// inventar 100 g cuando se dice "2 huevos" o "1 banana".
var servingGrams = map[string]float64{
	"huevo":                     50,
	"banana":                    120,
	"manzana":                   180,
	"palta":                     100,
	"pan blanco":                30,
	"nueces":                    15,
	"arroz blanco cocido":       150,
	"fideos secos":              80,
	"avena":                     40,
	"leche entera":              200,
	"papa":                      200,
	"tomate":                    100,
	"lechuga":                   50,
	"queso crema":               30,
	"queso cremoso":             30,
	"queso duro":                30,
	"yogur griego sin azucar":   170,
	"yogur natural":             190,
	"atun al natural escurrido": 120,
	"aceite de oliva":           10,
	"miel":                      21,
	"chocolate 85%":             20,
	"chocolate amargo 70%":      20,
	"merluza":                   150,
	"carne vacuna magra":        150,
	"pollo pechuga":             150,
}

// defaultServing se usa cuando el alimento no tiene porción de tabla.
const defaultServing = 100.0

func servingFor(name string) (float64, bool) {
	n := normalize(name)
	if g, ok := servingGrams[n]; ok {
		return g, true
	}
	if i := strings.IndexByte(n, ' '); i > 0 {
		if g, ok := servingGrams[n[:i]]; ok {
			return g, true
		}
	}
	return defaultServing, false
}

// takeQuantity separa la cantidad del resto del fragmento.
func takeQuantity(seg string) (qty, string) {
	if m := fracRe.FindStringSubmatch(seg); m != nil {
		n, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		if d > 0 && n > 0 {
			return applyUnit(float64(n)/float64(d), strings.TrimSpace(seg[len(m[0]):]))
		}
	}
	if m := numRe.FindStringSubmatch(seg); m != nil {
		n, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		if err == nil && n > 0 {
			return applyUnit(n, strings.TrimSpace(seg[len(m[0]):]))
		}
	}

	toks := strings.Fields(seg)
	if len(toks) > 0 {
		if v, ok := wordNumbers[toks[0]]; ok {
			rest := strings.TrimSpace(strings.Join(toks[1:], " "))
			// "un cuarto" / "tres cuartos"
			if len(toks) > 1 {
				switch {
				case toks[0] == "un" || toks[0] == "una":
					if toks[1] == "cuarto" || toks[1] == "cuarta" {
						return applyUnit(0.25, strings.TrimSpace(strings.Join(toks[2:], " ")))
					}
				case toks[0] == "tres":
					if toks[1] == "cuartos" {
						return applyUnit(0.75, strings.TrimSpace(strings.Join(toks[2:], " ")))
					}
				}
			}
			return applyUnit(v, rest)
		}
	}
	return qty{kind: qtyNone}, seg
}

// applyUnit interpreta la unidad que sigue al número.
func applyUnit(n float64, rest string) (qty, string) {
	if n <= 0 {
		return qty{kind: qtyNone}, rest
	}
	if m := weightUnitRe.FindStringSubmatch(rest); m != nil {
		g := n * weightUnitGrams(m[1])
		return qty{kind: qtyWeight, grams: g, label: fmt.Sprintf("%g g", roundG(g))},
			strings.TrimSpace(rest[len(m[0]):])
	}
	if m := measureRe.FindStringSubmatch(rest); m != nil {
		u := singularUnit(m[1])
		tail := strings.TrimSpace(rest[len(m[0]):])
		if u == "unidad" {
			return qty{kind: qtyUnits, value: n, label: fmt.Sprintf("%g unidades", n)}, tail
		}
		return qty{kind: qtyMeasure, value: n, unit: u,
			label: fmt.Sprintf("%g %s", n, pluralUnit(u, n))}, tail
	}
	return qty{kind: qtyUnits, value: n, label: fmt.Sprintf("%g unidades", n)}, rest
}

func weightUnitGrams(u string) float64 {
	switch u {
	case "kg", "kilo", "kilos", "kilogramo", "kilogramos":
		return 1000
	}
	return 1
}

func singularUnit(u string) string {
	switch u {
	case "u", "unid", "unidad", "unidades":
		return "unidad"
	case "cucharaditas", "cucharadita":
		return "cucharadita"
	case "cucharadas", "cucharada":
		return "cucharada"
	case "tazas", "taza":
		return "taza"
	case "vasos", "vaso":
		return "vaso"
	case "latas", "lata":
		return "lata"
	case "platos", "plato":
		return "plato"
	case "punados", "punado":
		return "punado"
	case "fetas", "feta":
		return "feta"
	case "rodajas", "rodaja":
		return "rodaja"
	case "porciones", "porcion":
		return "porcion"
	case "scoops", "scoop":
		return "scoop"
	}
	return u
}

func pluralUnit(u string, n float64) string {
	if n == 1 {
		return u
	}
	switch u {
	case "taza", "porcion", "cucharada", "cucharadita", "rodaja", "feta", "lata":
		return u + "s"
	}
	return u
}

// ---------------------------------------------------------------- matching

const matchThreshold = 0.55

func matchFood(query string, foods []Food) (Food, float64, bool) {
	q := normalize(query)
	q = trimFillerPrefix(q)
	if q == "" {
		return Food{}, 0, false
	}
	qt := tokens(q)

	best, second := 0.0, 0.0
	var bestFood Food
	for _, f := range foods {
		s := score(q, qt, f)
		switch {
		case s > best:
			second = best
			best = s
			bestFood = f
		case s > second:
			second = s
		}
	}
	if best < matchThreshold {
		return Food{}, 0, false
	}

	conf := best
	// Varios alimentos empatan (ej. "queso" con crema/cremoso/duro): se elige
	// el primero pero con confianza baja, para que el cliente pueda preguntar.
	if second >= matchThreshold && best-second < 0.05 && best < 0.95 {
		conf = math.Max(0.3, best-0.25)
	}
	return bestFood, round2(conf), true
}

func score(q string, qt []string, f Food) float64 {
	fn := normalize(f.Name)
	ft := tokens(fn)

	switch {
	case q == fn:
		return 1.0
	case singular(q) == singular(fn):
		return 0.97
	case len(q) >= 4 && strings.Contains(fn, q):
		return 0.9
	case len(fn) >= 4 && strings.Contains(q, fn):
		return 0.88
	case len(ft) > 0 && containsAll(qt, ft):
		return 0.86
	case len(qt) == 1 && len(ft) > 0 && singular(ft[0]) == singular(qt[0]):
		return 0.8
	}

	// Parecido ortográfico global ("pollo pechuga asada" vs "pollo pechuga").
	if sim := similarity(q, fn); sim >= 0.8 {
		return 0.6 * sim
	}
	return 0
}

func containsAll(haystack, needles []string) bool {
	for _, n := range needles {
		found := false
		for _, h := range haystack {
			if h == n || singular(h) == singular(n) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func singular(s string) string {
	if len(s) > 4 && strings.HasSuffix(s, "es") {
		return strings.TrimSuffix(s, "es")
	}
	if len(s) > 3 && strings.HasSuffix(s, "s") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

// similarity es 1 - distancia de Levenshtein normalizada.
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return 0
	}
	d := levenshtein(a, b)
	return 1 - float64(d)/float64(max(la, lb))
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// ---------------------------------------------------------------- gramos

// parseSegment analiza un fragmento ya normalizado.
func parseSegment(seg string, foods []Food) (Item, bool) {
	seg = trimFillerPrefix(seg)
	if seg == "" {
		return Item{}, false
	}

	q, rest := takeQuantity(seg)
	rest = trimFillerPrefix(strings.TrimSpace(rest))

	food, matchConf, ok := matchFood(rest, foods)
	if !ok {
		return Item{}, false
	}

	grams, qtyConf, label := resolveGrams(q, food)
	if grams <= 0 {
		return Item{}, false
	}

	return Item{
		FoodID:     food.ID,
		FoodName:   food.Name,
		Grams:      roundG(grams),
		Macros:     nutrition.Scale(food.Per100, grams),
		Matched:    true,
		Confidence: round2(math.Max(0.1, matchConf*qtyConf)),
		Quantity:   label,
	}, true
}

// resolveGrams traduce la cantidad a gramos y devuelve la confianza asociada.
func resolveGrams(q qty, f Food) (grams, conf float64, label string) {
	switch q.kind {
	case qtyWeight:
		return clampGrams(q.grams), 1.0, q.label
	case qtyMeasure:
		per, ok := measureGrams[q.unit]
		if !ok {
			per, _ = servingFor(f.Name)
		}
		c := 0.85
		if !ok {
			c = 0.6
		}
		return clampGrams(q.value * per), c, q.label
	case qtyUnits:
		per, known := servingFor(f.Name)
		c := 0.7
		if known {
			c = 0.9
		}
		return clampGrams(q.value * per), c, q.label
	}

	per, known := servingFor(f.Name)
	if known {
		return clampGrams(per), 0.75, fmt.Sprintf("porción estimada %g g", roundG(per))
	}
	return clampGrams(per), 0.55, fmt.Sprintf("porción estimada %g g", roundG(per))
}

func clampGrams(g float64) float64 {
	if g < 1 {
		return 1
	}
	if g > 5000 {
		return 5000
	}
	return g
}

// ---------------------------------------------------------------- texto

func tokens(s string) []string {
	return strings.Fields(s)
}

func normalize(s string) string {
	// Coma decimal (1,5 kg) antes de tratar la coma como separador.
	s = decimalCommaRe.ReplaceAllString(s, "$1.$2")
	s = strings.ToLower(strings.TrimSpace(s))
	s = foldAccents(s)
	// Se conservan dígitos, letras, punto y barra; el resto separa.
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevSpace = false
		case r == '.' || r == '/' || r == '%':
			b.WriteRune(r)
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var decimalCommaRe = regexp.MustCompile(`(\d),(\d)`)

func foldAccents(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'ä', 'â', 'ã':
			return 'a'
		case 'é', 'è', 'ë', 'ê':
			return 'e'
		case 'í', 'ì', 'ï', 'î':
			return 'i'
		case 'ó', 'ò', 'ö', 'ô', 'õ':
			return 'o'
		case 'ú', 'ù', 'ü', 'û':
			return 'u'
		case 'ñ':
			return 'n'
		case 'ç':
			return 'c'
		}
		return r
	}, s)
}

func roundG(v float64) float64 { return math.Round(v*10) / 10 }

func round2(v float64) float64 { return math.Round(v*100) / 100 }
