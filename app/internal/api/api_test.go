package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ldgnu/fitordie/internal/api"
	"github.com/ldgnu/fitordie/internal/store"
)

// newTestServer levanta la API sobre una base temporal ya sembrada.
func newTestServer(t *testing.T, opt api.Options) (*httptest.Server, *store.Store) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	if err := st.Seed(context.Background(), false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	loc, err := time.LoadLocation("America/Argentina/Cordoba")
	if err != nil {
		t.Fatalf("tz: %v", err)
	}

	mux := http.NewServeMux()
	api.NewWithOptions(st, loc, opt).Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		st.Close()
	})
	return srv, st
}

// do hace un request y devuelve status + body decodificado.
func do(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatalf("armar request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	out := map[string]any{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("%s %s: JSON inválido: %v", method, path, err)
	}
	return res.StatusCode, out
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// TestSeedPesosHistoricos verifica la siembra idempotente de las pesas reales.
func TestSeedPesosHistoricos(t *testing.T) {
	srv, st := newTestServer(t, api.Options{})

	code, body := do(t, srv, http.MethodGet, "/api/weight", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/weight = %d", code)
	}
	weights, _ := body["weights"].([]any)
	if len(weights) != 2 {
		t.Fatalf("esperaba 2 pesas sembradas, hay %d (%v)", len(weights), weights)
	}

	byDay := map[string]map[string]any{}
	for _, w := range weights {
		row := w.(map[string]any)
		byDay[row["day"].(string)] = row
	}
	prev, ok := byDay["2026-10-04"]
	if !ok || num(prev["kg"]) != 96 {
		t.Fatalf("falta la pesa previa 96.0 del 04/10: %v", byDay)
	}
	if prev["note"] == "" {
		t.Fatal("la pesa previa debería tener nota (fecha estimada)")
	}
	real, ok := byDay["2026-10-05"]
	if !ok || num(real["kg"]) != 89.65 || real["note"] != "pesa real" {
		t.Fatalf("pesa real del 05/10 incorrecta: %v", real)
	}

	// Idempotente: un segundo seed no duplica ni cambia valores.
	if err := st.Seed(context.Background(), false); err != nil {
		t.Fatalf("segundo seed: %v", err)
	}
	rows, err := st.Weights(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("el seed duplicó pesas: %d filas", len(rows))
	}
}

// TestPesoHistoricoNoPisaPerfil cubre la regla central de esta tanda: cargar
// una pesa vieja actualiza el historial pero no el peso actual del perfil.
func TestPesoHistoricoNoPisaPerfil(t *testing.T) {
	srv, st := newTestServer(t, api.Options{})
	ctx := context.Background()

	code, body := do(t, srv, http.MethodPost, "/api/weight",
		map[string]any{"date": "2026-10-21", "kg": 88.4, "note": "pesa real"})
	if code != http.StatusCreated {
		t.Fatalf("POST /api/weight = %d (%v)", code, body)
	}
	if body["profile_updated"] != true {
		t.Fatalf("la pesa más reciente debe actualizar el perfil: %v", body)
	}
	p, _ := st.Profile(ctx)
	if p.WeightKg != 88.4 {
		t.Fatalf("perfil: esperaba 88.4, obtuve %v", p.WeightKg)
	}

	// Pesa histórica: entra al historial, NO toca el perfil.
	code, body = do(t, srv, http.MethodPost, "/api/weight",
		map[string]any{"date": "2026-10-01", "kg": 95.2, "note": "estimada"})
	if code != http.StatusCreated {
		t.Fatalf("POST histórico = %d (%v)", code, body)
	}
	if body["profile_updated"] != false {
		t.Fatalf("una pesa histórica no debe actualizar el perfil: %v", body)
	}
	p, _ = st.Profile(ctx)
	if p.WeightKg != 88.4 {
		t.Fatalf("el perfil se pisó con una pesa histórica: %v", p.WeightKg)
	}

	// Delta contra la pesa anterior a esa fecha (2026-10-05, 89.65), no contra
	// el último registro global ni contra el valor previo del mismo día.
	_, body = do(t, srv, http.MethodPost, "/api/weight",
		map[string]any{"date": "2026-10-21", "kg": 88.1})
	if got := num(body["delta_kg"]); got != -1.6 {
		t.Fatalf("delta contra vecino anterior: esperaba -1.6, obtuve %v", got)
	}
	if prev, ok := body["previous_kg"]; !ok || num(prev) != 89.65 {
		t.Fatalf("previous_kg: esperaba 89.65, obtuve %v", body["previous_kg"])
	}

	// Filtro from/to.
	code, body = do(t, srv, http.MethodGet, "/api/weight?from=2026-10-01&to=2026-10-05", nil)
	if code != http.StatusOK {
		t.Fatalf("GET filtrado = %d", code)
	}
	if rows, _ := body["weights"].([]any); len(rows) != 3 {
		t.Fatalf("rango 01/10-05/10: esperaba 3 pesas, hay %d", len(rows))
	}

	if code, _ := do(t, srv, http.MethodGet, "/api/weight?from=22/10/2026", nil); code != http.StatusBadRequest {
		t.Fatalf("fecha inválida en from: esperaba 400, obtuve %d", code)
	}
}

func TestPatchProfile(t *testing.T) {
	srv, _ := newTestServer(t, api.Options{})

	code, body := do(t, srv, http.MethodPatch, "/api/profile",
		map[string]any{"target_weight_min": 82, "target_weight_max": 84.5})
	if code != http.StatusOK {
		t.Fatalf("PATCH /api/profile = %d (%v)", code, body)
	}
	prof := body["profile"].(map[string]any)
	if num(prof["target_weight_min"]) != 82 || num(prof["target_weight_max"]) != 84.5 {
		t.Fatalf("perfil no actualizado: %v", prof)
	}
	// Los campos no enviados se conservan.
	if num(prof["kcal_min"]) != 2150 {
		t.Fatalf("kcal_min se perdió en el patch: %v", prof)
	}

	// POST como alias.
	code, body = do(t, srv, http.MethodPost, "/api/profile",
		map[string]any{"height_cm": 185, "kcal_min": 2200, "kcal_max": 2450,
			"protein_min": 160, "protein_max": 175})
	if code != http.StatusOK {
		t.Fatalf("POST /api/profile = %d (%v)", code, body)
	}
	prof = body["profile"].(map[string]any)
	if num(prof["kcal_max"]) != 2450 || num(prof["protein_max"]) != 175 {
		t.Fatalf("perfil no actualizado por POST: %v", prof)
	}

	// min > max -> 400
	code, body = do(t, srv, http.MethodPatch, "/api/profile",
		map[string]any{"target_weight_min": 90, "target_weight_max": 80})
	if code != http.StatusBadRequest {
		t.Fatalf("min>max: esperaba 400, obtuve %d (%v)", code, body)
	}

	// fuera de rango -> 400
	code, _ = do(t, srv, http.MethodPatch, "/api/profile",
		map[string]any{"target_weight_min": 500, "target_weight_max": 600})
	if code != http.StatusBadRequest {
		t.Fatalf("peso fuera de rango: esperaba 400, obtuve %d", code)
	}

	// patch vacío -> 400
	code, _ = do(t, srv, http.MethodPatch, "/api/profile", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("patch vacío: esperaba 400, obtuve %d", code)
	}
}

func TestGetWorkoutsConVolumen(t *testing.T) {
	srv, st := newTestServer(t, api.Options{})
	ctx := context.Background()

	routine, err := st.Routine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var exIDs []int64
	for _, e := range routine {
		if e.Day == "A" && len(exIDs) < 2 {
			exIDs = append(exIDs, e.ID)
		}
	}
	if len(exIDs) != 2 {
		t.Fatalf("rutina sin ejercicios del día A: %v", routine)
	}

	code, body := do(t, srv, http.MethodPost, "/api/workout", map[string]any{
		"date": "2026-10-21", "routine_day": "A", "duration_min": 45, "notes": "sesión test",
		"sets": []map[string]any{
			{"exercise_id": exIDs[0], "set_number": 1, "weight_kg": 100, "reps": 10, "rir": 3, "note": "última lenta"},
			{"exercise_id": exIDs[0], "set_number": 2, "weight_kg": 100, "reps": 8, "rir": 2},
			{"exercise_id": exIDs[1], "set_number": 1, "weight_kg": 40, "reps": 12, "rir": 2},
		},
	})
	if code != http.StatusCreated {
		t.Fatalf("POST /api/workout = %d (%v)", code, body)
	}

	code, body = do(t, srv, http.MethodGet, "/api/workouts?from=2026-10-20&to=2026-10-22", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/workouts = %d", code)
	}
	workouts, _ := body["workouts"].([]any)
	if len(workouts) != 1 {
		t.Fatalf("esperaba 1 entrenamiento, hay %d (%v)", len(workouts), body)
	}
	wo := workouts[0].(map[string]any)
	if wo["date"] != "2026-10-21" || wo["routine_day"] != "A" || num(wo["duration_min"]) != 45 {
		t.Fatalf("cabecera inesperada: %v", wo)
	}
	sets, _ := wo["sets"].([]any)
	if len(sets) != 3 {
		t.Fatalf("esperaba 3 series, hay %d", len(sets))
	}
	first := sets[0].(map[string]any)
	if first["exercise_name"] == "" {
		t.Fatalf("la serie no trae exercise_name: %v", first)
	}
	if num(first["volume_kg"]) != 1000 {
		t.Fatalf("volumen de la serie: esperaba 1000, obtuve %v", first["volume_kg"])
	}
	if first["note"] != "última lenta" {
		t.Fatalf("la nota de la serie no volvió: %v", first["note"])
	}
	// 100*10 + 100*8 + 40*12 = 2280
	if num(wo["volume_kg"]) != 2280 {
		t.Fatalf("volumen total: esperaba 2280, obtuve %v", wo["volume_kg"])
	}
	byEx, _ := wo["by_exercise"].([]any)
	if len(byEx) != 2 {
		t.Fatalf("esperaba 2 ejercicios agregados, hay %d (%v)", len(byEx), byEx)
	}
	if num(byEx[0].(map[string]any)["volume_kg"]) != 1800 {
		t.Fatalf("volumen por ejercicio: %v", byEx[0])
	}
	if num(body["volume_kg"]) != 2280 {
		t.Fatalf("volumen del rango: esperaba 2280, obtuve %v", body["volume_kg"])
	}

	// Rango sin resultados.
	_, body = do(t, srv, http.MethodGet, "/api/workouts?from=2026-01-01&to=2026-01-31", nil)
	if rows, _ := body["workouts"].([]any); len(rows) != 0 {
		t.Fatalf("rango vacío: esperaba 0, obtuve %d", len(rows))
	}
}

func TestEstimateLocalNoLoguea(t *testing.T) {
	srv, _ := newTestServer(t, api.Options{})

	code, body := do(t, srv, http.MethodPost, "/api/estimate", map[string]any{
		"date": "2026-10-22", "meal": "almuerzo",
		"text": "comí 250 g de pollo con arroz y ensalada",
	})
	if code != http.StatusOK {
		t.Fatalf("POST /api/estimate = %d (%v)", code, body)
	}
	if body["source"] != "local" {
		t.Fatalf("source: esperaba local, obtuve %v", body["source"])
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("esperaba 2 items (pollo, arroz), hay %d (%v)", len(items), body["items"])
	}
	pollo := items[0].(map[string]any)
	if pollo["food_name"] != "Pollo pechuga" || num(pollo["grams"]) != 250 || pollo["matched"] != true {
		t.Fatalf("item pollo inesperado: %v", pollo)
	}
	if num(pollo["macros"].(map[string]any)["kcal"]) != 413 {
		t.Fatalf("kcal de 250 g de pollo: esperaba 413, obtuve %v", pollo["macros"])
	}
	unmatched, _ := body["unmatched"].([]any)
	if len(unmatched) != 1 || unmatched[0].(map[string]any)["text"] != "ensalada" {
		t.Fatalf("unmatched: %v", body["unmatched"])
	}
	if num(body["totals"].(map[string]any)["kcal"]) != 608 {
		t.Fatalf("totales: esperaba 608 kcal, obtuve %v", body["totals"])
	}

	// No se logueó nada: el día sigue vacío.
	_, day := do(t, srv, http.MethodGet, "/api/day?date=2026-10-22", nil)
	entries := 0
	if meals, ok := day["meals"].([]any); ok {
		for _, m := range meals {
			if e, ok := m.(map[string]any)["entries"].([]any); ok {
				entries += len(e)
			}
		}
	}
	if entries != 0 {
		t.Fatalf("POST /api/estimate no debe escribir en el diario (entradas: %d)", entries)
	}

	// meal inválida -> 400
	if code, _ := do(t, srv, http.MethodPost, "/api/estimate",
		map[string]any{"meal": "brunch", "text": "2 huevos"}); code != http.StatusBadRequest {
		t.Fatalf("meal inválida: esperaba 400, obtuve %d", code)
	}
	// texto vacío -> 400
	if code, _ := do(t, srv, http.MethodPost, "/api/estimate",
		map[string]any{"meal": "cena", "text": "  "}); code != http.StatusBadRequest {
		t.Fatalf("texto vacío: esperaba 400, obtuve %d", code)
	}
}

// TestEstimateLLMNoInventaMacros verifica que si el modelo propone un alimento
// que no está en `foods`, el item sale matched=false con macros en cero.
func TestEstimateLLMNoInventaMacros(t *testing.T) {
	var gotPath string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		content := "```json\n" +
			`{"items":[{"food":"pollo","grams":200,"quantity":"200 g"},{"food":"salsa de dragon","grams":50}]}` +
			"\n```"
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(llm.Close)

	srv, _ := newTestServer(t, api.Options{
		LLMBaseURL: llm.URL,
		LLMAPIKey:  "test-key",
		LLMModel:   "test-model",
		LLMClient:  llm.Client(),
	})

	code, body := do(t, srv, http.MethodPost, "/api/estimate", map[string]any{
		"meal": "almuerzo", "text": "pollo con salsa de dragon",
	})
	if code != http.StatusOK {
		t.Fatalf("POST /api/estimate = %d (%v)", code, body)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("el cliente LLM llamó a %q", gotPath)
	}
	if body["source"] != "llm" {
		t.Fatalf("source: esperaba llm, obtuve %v", body["source"])
	}

	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("esperaba 2 items, hay %d (%v)", len(items), body["items"])
	}
	pollo := items[0].(map[string]any)
	if pollo["matched"] != true || num(pollo["grams"]) != 200 {
		t.Fatalf("item pollo inesperado: %v", pollo)
	}
	inventado := items[1].(map[string]any)
	if inventado["matched"] != false || inventado["food_id"] != nil {
		t.Fatalf("alimento inventado debería venir sin food_id: %v", inventado)
	}
	if num(inventado["macros"].(map[string]any)["kcal"]) != 0 {
		t.Fatalf("no se deben inventar macros: %v", inventado["macros"])
	}
	if unmatched, _ := body["unmatched"].([]any); len(unmatched) != 1 {
		t.Fatalf("esperaba 1 unmatched, obtuve %v", body["unmatched"])
	}
	// 200 g de pollo = 330 kcal, y el inventado no suma.
	if got := num(body["totals"].(map[string]any)["kcal"]); got != 330 {
		t.Fatalf("totales: esperaba 330, obtuve %v", got)
	}
}

// TestEstimateLLMCaidoUsaLocal: si el LLM falla, la respuesta no se rompe.
func TestEstimateLLMCaidoUsaLocal(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(llm.Close)

	srv, _ := newTestServer(t, api.Options{
		LLMBaseURL: llm.URL, LLMModel: "test-model", LLMClient: llm.Client(),
	})

	code, body := do(t, srv, http.MethodPost, "/api/estimate", map[string]any{
		"meal": "desayuno", "text": "2 huevos",
	})
	if code != http.StatusOK {
		t.Fatalf("POST /api/estimate = %d (%v)", code, body)
	}
	if body["source"] != "local" {
		t.Fatalf("source: esperaba local, obtuve %v", body["source"])
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("esperaba 1 item, hay %d", len(items))
	}
	if note, _ := body["note"].(string); note == "" {
		t.Fatal("esperaba una nota explicando el fallback")
	}
}
