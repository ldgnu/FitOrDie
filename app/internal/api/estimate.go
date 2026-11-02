// Handler HTTP de estimación de comidas por texto libre.
//
// La lógica vive en internal/estimate (compartida con el panel web); acá solo
// se valida el request y se arma el JSON de respuesta.
package api

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ldgnu/fitordie/internal/estimate"
	"github.com/ldgnu/fitordie/internal/nutrition"
	"github.com/ldgnu/fitordie/internal/store"
)

// maxEstimateText limita el texto analizado (una comida, no un diario).
const maxEstimateText = 600

type estimateRequest struct {
	Date string `json:"date"`
	Meal string `json:"meal"`
	Text string `json:"text"`
}

type estimateItemJSON struct {
	FoodID     *int64           `json:"food_id"`
	FoodName   string           `json:"food_name"`
	Grams      float64          `json:"grams"`
	Macros     nutrition.Macros `json:"macros"`
	Matched    bool             `json:"matched"`
	Confidence float64          `json:"confidence"`
	Quantity   string           `json:"quantity,omitempty"`
	Input      string           `json:"input,omitempty"`
}

type unmatchedJSON struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

type estimateResponse struct {
	Date      string             `json:"date"`
	Meal      string             `json:"meal"`
	Text      string             `json:"text"`
	Source    string             `json:"source"` // "local" o "llm"
	Items     []estimateItemJSON `json:"items"`
	Unmatched []unmatchedJSON    `json:"unmatched"`
	Totals    nutrition.Macros   `json:"totals"`
	// Note explica degradaciones (LLM caído, alimento inventado, etc.).
	Note string `json:"note,omitempty"`
}

// estimator arma el estimador con la configuración opcional de LLM.
func (a *API) estimator() *estimate.Estimator {
	return estimate.New(a.st, estimate.Options{
		LLMBaseURL: a.opt.LLMBaseURL,
		LLMAPIKey:  a.opt.LLMAPIKey,
		LLMModel:   a.opt.LLMModel,
		LLMClient:  a.opt.LLMClient,
	})
}

func (a *API) postEstimate(w http.ResponseWriter, r *http.Request) {
	var req estimateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	day, err := a.resolveDay(req.Date)
	if err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !store.ValidMeal(req.Meal) {
		a.fail(w, http.StatusBadRequest, "comida inválida: usá "+mealSlugs())
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		a.fail(w, http.StatusBadRequest, "falta el texto a analizar")
		return
	}
	if utf8.RuneCountInString(text) > maxEstimateText {
		a.fail(w, http.StatusBadRequest, fmt.Sprintf("texto demasiado largo (máx %d caracteres)", maxEstimateText))
		return
	}

	res, err := a.estimator().Run(r.Context(), text)
	if err != nil {
		a.failStore(w, err)
		return
	}

	out := estimateResponse{
		Date: day, Meal: req.Meal, Text: text,
		Source: res.Source, Note: res.Note,
		Items:     make([]estimateItemJSON, 0, len(res.Items)),
		Unmatched: make([]unmatchedJSON, 0, len(res.Unmatched)),
		Totals:    res.Totals,
	}
	for _, it := range res.Items {
		var id *int64
		if it.Matched && it.FoodID > 0 {
			v := it.FoodID
			id = &v
		}
		out.Items = append(out.Items, estimateItemJSON{
			FoodID: id, FoodName: it.FoodName, Grams: it.Grams,
			Macros: it.Macros.Rounded(), Matched: it.Matched,
			Confidence: it.Confidence, Quantity: it.Quantity, Input: it.Input,
		})
	}
	for _, u := range res.Unmatched {
		out.Unmatched = append(out.Unmatched, unmatchedJSON{Text: u.Text, Reason: u.Reason})
	}

	// NO se loguea nada: el cliente confirma con POST /api/log.
	writeJSON(w, http.StatusOK, out)
}
