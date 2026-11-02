// Package estimate convierte texto libre de comidas en ítems con macros.
//
// El camino principal es local y determinista (internal/parser): no depende de
// red ni de un modelo, así que responde igual siempre. Si hay un LLM
// OpenAI-compatible configurado (LLM_BASE_URL/LLM_API_KEY/LLM_MODEL) se le pide
// una segunda opinión; el modelo solo propone nombres y gramos, y el match
// contra `foods` + el cálculo de macros siguen siendo locales. Un alimento que
// el modelo invente queda Matched=false y con macros en cero.
//
// Vive en su propio paquete porque lo usan las dos superficies: la API JSON
// (POST /api/estimate) y el panel web, que necesita exactamente el mismo
// resultado para mostrar la previsualización antes de confirmar.
package estimate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ldgnu/fitordie/internal/nutrition"
	"github.com/ldgnu/fitordie/internal/parser"
	"github.com/ldgnu/fitordie/internal/store"
)

// Timeout acota la espera al modelo: si tarda, se usa el parser local.
const Timeout = 8 * time.Second

// Options configura el LLM opcional. Con todo vacío solo corre el parser local.
type Options struct {
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
	// LLMClient permite inyectar un cliente HTTP en tests. nil = default.
	LLMClient *http.Client
}

// Enabled informa si hay un LLM configurado.
func (o Options) Enabled() bool {
	return strings.TrimSpace(o.LLMBaseURL) != "" && strings.TrimSpace(o.LLMModel) != ""
}

// Item es un alimento detectado en el texto.
type Item struct {
	FoodID     int64
	FoodName   string
	Grams      float64
	Macros     nutrition.Macros
	Matched    bool
	Confidence float64
	Quantity   string
	Input      string
}

// Unmatched es un fragmento del texto que no se pudo resolver.
type Unmatched struct {
	Text   string
	Reason string
}

// Result es el análisis completo de un texto.
type Result struct {
	Items     []Item
	Unmatched []Unmatched
	Totals    nutrition.Macros
	Source    string // "local" o "llm"
	// Note explica degradaciones (LLM caído, alimento inventado, etc.).
	Note string
}

// Estimator analiza texto contra el catálogo de alimentos del store.
type Estimator struct {
	st  *store.Store
	opt Options
}

// New construye un estimador.
func New(st *store.Store, opt Options) *Estimator {
	return &Estimator{st: st, opt: opt}
}

// Run analiza el texto. Nunca falla por culpa del LLM: si el modelo no
// responde, cae al parser local y lo deja anotado en Note.
func (e *Estimator) Run(ctx context.Context, text string) (Result, error) {
	foods, err := e.st.AllFoods(ctx)
	if err != nil {
		return Result{}, err
	}
	candidates := toParserFoods(foods)

	out := Result{Source: "local", Items: []Item{}, Unmatched: []Unmatched{}}

	// 1) Camino local determinista: siempre se calcula, sirve de fallback.
	local := parser.Parse(text, candidates)

	if e.opt.Enabled() {
		items, llmErr := e.llmItems(ctx, text, foods)
		switch {
		case llmErr != nil:
			slog.Warn("estimate: LLM no disponible, se usa parser local", "err", llmErr)
			out.Note = "LLM no disponible; se usó el parser local"
		case len(items) > 0:
			out.Source = "llm"
			out.Items, out.Unmatched, out.Totals = fromLLMItems(items, candidates, text)
		default:
			out.Note = "el LLM no devolvió items; se usó el parser local"
		}
	}

	if out.Source == "local" {
		out.Items = make([]Item, 0, len(local.Items))
		for _, it := range local.Items {
			out.Items = append(out.Items, Item{
				FoodID:     it.FoodID,
				FoodName:   it.FoodName,
				Grams:      it.Grams,
				Macros:     it.Macros.Rounded(),
				Matched:    it.Matched,
				Confidence: it.Confidence,
				Quantity:   it.Quantity,
				Input:      it.Input,
			})
		}
		out.Unmatched = make([]Unmatched, 0, len(local.Unmatched))
		for _, u := range local.Unmatched {
			out.Unmatched = append(out.Unmatched, Unmatched{Text: u.Text, Reason: u.Reason})
		}
		out.Totals = local.Totals.Rounded()
	}

	return out, nil
}

func toParserFoods(foods []store.Food) []parser.Food {
	out := make([]parser.Food, 0, len(foods))
	for _, f := range foods {
		out = append(out, parser.Food{ID: f.ID, Name: f.Name, Per100: f.Per100})
	}
	return out
}

// ---------------------------------------------------------------- LLM

type llmItem struct {
	Food     string  `json:"food"`
	Grams    float64 `json:"grams"`
	Quantity string  `json:"quantity"`
}

type llmItemsResponse struct {
	Items []llmItem `json:"items"`
}

// llmItems pide al modelo una lista de alimentos + gramos para el texto.
// El modelo NO devuelve macros: eso se calcula localmente contra `foods`.
func (e *Estimator) llmItems(ctx context.Context, text string, foods []store.Food) ([]llmItem, error) {
	client := e.opt.LLMClient
	if client == nil {
		client = &http.Client{Timeout: Timeout}
	}

	names := make([]string, 0, len(foods))
	for _, f := range foods {
		names = append(names, f.Name)
	}

	system := "Sos un asistente que convierte texto libre de comidas en JSON. " +
		"Respondés SOLO con JSON válido, sin markdown ni explicaciones. " +
		`Formato exacto: {"items":[{"food":"<nombre>","grams":<numero>,"quantity":"<texto>"}]}. ` +
		"Usá exclusivamente alimentos de la lista provista. Si el texto menciona algo que " +
		"no está en la lista, igual devolvé ese nombre tal cual, sin inventar valores nutricionales. " +
		"Nunca devuelvas macros: solo food, grams y quantity."
	user := "Alimentos disponibles:\n" + strings.Join(names, ", ") +
		"\n\nTexto: " + text

	payload, err := json.Marshal(map[string]any{
		"model":       e.opt.LLMModel,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("armar request LLM: %w", err)
	}

	url := strings.TrimRight(e.opt.LLMBaseURL, "/") + "/chat/completions"
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("crear request LLM: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(e.opt.LLMAPIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llamar LLM: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM respondió %d", res.StatusCode)
	}

	var body struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("leer respuesta LLM: %w", err)
	}
	if len(body.Choices) == 0 {
		return nil, errors.New("respuesta LLM sin choices")
	}

	return parseLLMContent(body.Choices[0].Message.Content)
}

// parseLLMContent extrae el primer objeto JSON del contenido del modelo
// (tolera ```json ... ``` y texto alrededor).
func parseLLMContent(content string) ([]llmItem, error) {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, errors.New("el LLM no devolvió JSON")
	}
	var parsed llmItemsResponse
	if err := json.Unmarshal([]byte(content[start:end+1]), &parsed); err != nil {
		return nil, fmt.Errorf("JSON del LLM inválido: %w", err)
	}
	// Descarta entradas sin nombre de alimento.
	out := make([]llmItem, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		if strings.TrimSpace(it.Food) == "" {
			continue
		}
		if it.Grams < 0 || it.Grams > 5000 {
			it.Grams = 0
		}
		out = append(out, it)
	}
	return out, nil
}

// fromLLMItems matchea cada propuesta del modelo contra `foods` y calcula los
// macros localmente. Lo que no está en la base queda Matched=false, sin macros.
func fromLLMItems(items []llmItem, foods []parser.Food, text string) ([]Item, []Unmatched, nutrition.Macros) {
	out := make([]Item, 0, len(items))
	unmatched := make([]Unmatched, 0)
	macros := make([]nutrition.Macros, 0, len(items))

	for _, it := range items {
		grams := it.Grams
		if grams <= 0 {
			grams, _ = parser.ServingGrams(it.Food)
		}

		food, conf, ok := parser.Match(it.Food, foods)
		if !ok {
			out = append(out, Item{
				FoodName:   strings.TrimSpace(it.Food),
				Grams:      round1(grams),
				Macros:     nutrition.Zero,
				Matched:    false,
				Confidence: 0,
				Quantity:   it.Quantity,
				Input:      text,
			})
			unmatched = append(unmatched, Unmatched{
				Text:   strings.TrimSpace(it.Food),
				Reason: "el modelo propuso un alimento que no está en foods",
			})
			continue
		}

		// El modelo puede acertar el nombre pero no la cantidad: se exige un
		// gramaje razonable antes de calcular macros.
		if grams < 1 || grams > 5000 {
			grams = 100
			conf *= 0.6
		}
		m := nutrition.Scale(food.Per100, grams)
		macros = append(macros, m)
		out = append(out, Item{
			FoodID:     food.ID,
			FoodName:   food.Name,
			Grams:      round1(grams),
			Macros:     m.Rounded(),
			Matched:    true,
			Confidence: round2(conf),
			Quantity:   it.Quantity,
			Input:      text,
		})
	}

	return out, unmatched, nutrition.Sum(macros...).Rounded()
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
