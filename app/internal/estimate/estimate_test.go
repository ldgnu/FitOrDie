package estimate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ldgnu/fitordie/internal/estimate"
	"github.com/ldgnu/fitordie/internal/store"
)

// newStore abre una base temporal sembrada (alimentos + rutina).
func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "estimate.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	if err := st.Seed(context.Background(), false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestRunLocalSinLLM: sin LLM configurado el parser local tiene que resolver el
// texto y calcular macros. Es el caso de producción por defecto.
func TestRunLocalSinLLM(t *testing.T) {
	st := newStore(t)
	est := estimate.New(st, estimate.Options{})

	res, err := est.Run(context.Background(), "150 g de pollo y 200 g de arroz blanco")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Source != "local" {
		t.Errorf("source = %q, esperaba local", res.Source)
	}
	if len(res.Items) == 0 {
		t.Fatalf("no detectó ningún alimento: %+v", res)
	}
	for _, it := range res.Items {
		if !it.Matched || it.FoodID <= 0 {
			t.Errorf("ítem sin matchear: %+v", it)
		}
		if it.Macros.Kcal <= 0 {
			t.Errorf("ítem sin calorías: %+v", it)
		}
		if it.Grams <= 0 {
			t.Errorf("ítem sin gramos: %+v", it)
		}
	}
	if res.Totals.Kcal <= 0 {
		t.Errorf("totales en cero: %+v", res.Totals)
	}
}

// TestRunTextoVacioONoEntendido: si no hay alimentos, no inventa nada.
func TestRunTextoNoEntendido(t *testing.T) {
	st := newStore(t)
	est := estimate.New(st, estimate.Options{})

	res, err := est.Run(context.Background(), "zzzz qqqq")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("no debería matchear nada: %+v", res.Items)
	}
	if len(res.Unmatched) == 0 {
		t.Error("debería reportar el texto como no reconocido")
	}
}

// TestRunLLMCaidoCaeLocal: si el modelo no responde, la estimación sigue
// funcionando con el parser local y deja la degradación anotada.
func TestRunLLMCaidoCaeLocal(t *testing.T) {
	st := newStore(t)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(llm.Close)

	est := estimate.New(st, estimate.Options{
		LLMBaseURL: llm.URL, LLMModel: "test-model", LLMClient: llm.Client(),
	})
	res, err := est.Run(context.Background(), "150 g de pollo")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Source != "local" {
		t.Errorf("source = %q, esperaba local tras el fallo del LLM", res.Source)
	}
	if !strings.Contains(res.Note, "parser local") {
		t.Errorf("falta la nota de degradación: %q", res.Note)
	}
	if len(res.Items) == 0 {
		t.Error("el parser local debería haber resuelto el texto")
	}
}

// TestRunLLMInventaAlimentoSinMacros: el modelo solo propone nombres; lo que no
// está en foods queda sin macros (nunca se inventan valores).
func TestRunLLMInventaAlimentoSinMacros(t *testing.T) {
	st := newStore(t)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"items\":[{\"food\":\"pollo\",\"grams\":150},{\"food\":\"unicornio\",\"grams\":100}]}"}}]}`))
	}))
	t.Cleanup(llm.Close)

	est := estimate.New(st, estimate.Options{
		LLMBaseURL: llm.URL, LLMModel: "test-model", LLMClient: llm.Client(),
	})
	res, err := est.Run(context.Background(), "pollo y unicornio")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Source != "llm" {
		t.Fatalf("source = %q, esperaba llm", res.Source)
	}
	if len(res.Items) != 2 {
		t.Fatalf("esperaba 2 ítems, hay %d", len(res.Items))
	}
	if !res.Items[0].Matched || res.Items[0].Macros.Kcal <= 0 {
		t.Errorf("el pollo debería matchear con macros: %+v", res.Items[0])
	}
	if res.Items[1].Matched || res.Items[1].Macros.Kcal != 0 {
		t.Errorf("el inventado no puede traer macros: %+v", res.Items[1])
	}
	if len(res.Unmatched) != 1 {
		t.Errorf("esperaba 1 no reconocido, hay %d", len(res.Unmatched))
	}
}
