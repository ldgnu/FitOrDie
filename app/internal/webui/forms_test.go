package webui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ldgnu/fitordie/internal/store"
)

// stubCSRF es el guardián de formularios para los tests: acepta un token fijo.
type stubCSRF struct{ token string }

func (s stubCSRF) CSRFToken(http.ResponseWriter, *http.Request) string { return s.token }
func (s stubCSRF) ValidCSRFToken(r *http.Request) bool {
	return r.PostFormValue("csrf") == s.token
}

const testCSRF = "token-de-prueba"

// newWeb levanta el panel sobre una base temporal ya sembrada.
func newWeb(t *testing.T) (*Server, *store.Store, *httptest.Server) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
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

	srv, err := New(st, loc, WithCSRF(stubCSRF{testCSRF}))
	if err != nil {
		t.Fatalf("webui.New: %v", err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(func() {
		ts.Close()
		st.Close()
	})
	return srv, st, ts
}

// post hace un POST de formulario y devuelve status + body (sin seguir redirects).
func post(t *testing.T, ts *httptest.Server, path string, form url.Values) (int, string) {
	t.Helper()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.PostForm(ts.URL+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("leer body: %v", err)
	}
	return res.StatusCode, string(raw)
}

// get hace un GET y devuelve status + body.
func get(t *testing.T, ts *httptest.Server, path string) (int, string) {
	t.Helper()
	res, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("leer body: %v", err)
	}
	return res.StatusCode, string(raw)
}

// TestPaginasTraenFormulariosYCSRF cuida lo básico: las tres vistas renderizan
// con su formulario y con el token que exige el POST.
func TestPaginasTraenFormulariosYCSRF(t *testing.T) {
	_, _, ts := newWeb(t)

	cases := []struct {
		path string
		want []string
	}{
		// La previsualización (/hoy/confirmar) y los resultados de búsqueda
		// (/hoy/alimento) son condicionales: se prueban en sus propios tests.
		{"/", []string{`action="/hoy/analizar"`, `action="/hoy/manual"`}},
		{"/rutina", []string{`action="/rutina/serie"`, `id="sesion"`}},
		{"/progreso", []string{`action="/progreso/peso"`, `action="/progreso/peso/borrar"`}},
	}
	for _, c := range cases {
		code, body := get(t, ts, c.path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d", c.path, code)
		}
		if !strings.Contains(body, `name="csrf" value="`+testCSRF+`"`) {
			t.Errorf("GET %s no trae el token CSRF", c.path)
		}
		for _, w := range c.want {
			if !strings.Contains(body, w) {
				t.Errorf("GET %s no trae %q", c.path, w)
			}
		}
	}
}

// TestBuscarAlimentoYCargar cubre la búsqueda en la base (el camino rápido para
// lo que el parser no reconoce).
func TestBuscarAlimentoYCargar(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()
	today := store.Today(time.FixedZone("ART", -3*60*60))

	code, body := get(t, ts, "/?q=pollo")
	if code != http.StatusOK {
		t.Fatalf("buscar = %d", code)
	}
	if !strings.Contains(body, `action="/hoy/alimento"`) {
		t.Fatalf("la búsqueda no ofrece cargar el alimento: %s", body[:400])
	}

	foods, err := st.SearchFoods(ctx, "pollo", 1)
	if err != nil || len(foods) == 0 {
		t.Fatalf("sin resultados de pollo: %v", err)
	}
	code, _ = post(t, ts, "/hoy/alimento", url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"almuerzo"},
		"food_id": {itoa(foods[0].ID)}, "grams": {"120"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("cargar alimento = %d", code)
	}
	entries, _ := st.DayEntries(ctx, today)
	if len(entries) != 1 || entries[0].FoodName != foods[0].Name || entries[0].Grams != 120 {
		t.Fatalf("el alimento no se cargó bien: %+v", entries)
	}
}

// TestPostSinCSRFEs403: ningún POST del panel pasa sin token válido.
func TestPostSinCSRFEs403(t *testing.T) {
	_, st, ts := newWeb(t)
	today := store.Today(time.FixedZone("ART", -3*60*60))

	form := url.Values{
		"date": {today}, "meal": {"almuerzo"}, "name": {"Prueba"},
		"grams": {"100"}, "kcal": {"200"},
	}
	code, body := post(t, ts, "/hoy/manual", form)
	if code != http.StatusForbidden {
		t.Fatalf("POST sin csrf = %d, esperaba 403 (%s)", code, body)
	}
	if entries, err := st.DayEntries(context.Background(), today); err == nil && len(entries) != 0 {
		t.Fatalf("no debería haber entradas, hay %d", len(entries))
	}
}

// TestAnalizarYConfirmarCargaItens cubre el camino completo del analizador:
// previsualización sin escribir y confirmación con los macros recalculados por
// el servidor.
func TestAnalizarYConfirmarCargaItens(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()
	today := store.Today(time.FixedZone("ART", -3*60*60))

	foods, err := st.SearchFoods(ctx, "pollo", 5)
	if err != nil || len(foods) == 0 {
		t.Fatalf("buscar pollo: %v (%d resultados)", err, len(foods))
	}
	food := foods[0]

	code, body := post(t, ts, "/hoy/analizar", url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"almuerzo"},
		"text": {"150 g de " + strings.ToLower(food.Name)},
	})
	if code != http.StatusOK {
		t.Fatalf("analizar = %d (%s)", code, body)
	}
	if !strings.Contains(body, `action="/hoy/confirmar"`) {
		t.Fatalf("el análisis no muestra la previsualización: %s", body[:400])
	}
	if entries, _ := st.DayEntries(ctx, today); len(entries) != 0 {
		t.Fatalf("analizar no debe escribir en el diario (hay %d entradas)", len(entries))
	}

	code, body = post(t, ts, "/hoy/confirmar", url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"almuerzo"},
		"text": {"150 g de " + strings.ToLower(food.Name)}, "items": {"1"},
		"name_0": {food.Name}, "food_id_0": {itoa(food.ID)}, "grams_0": {"150"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("confirmar = %d, esperaba 303 (%s)", code, body)
	}

	entries, err := st.DayEntries(ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("esperaba 1 entrada, hay %d", len(entries))
	}
	if entries[0].Grams != 150 || entries[0].FoodName != food.Name {
		t.Errorf("entrada mal cargada: %+v", entries[0])
	}
	// Los macros los calcula el servidor desde foods: no vienen del form.
	want := food.Per100.Kcal * 1.5
	if diff := entries[0].Macros.Kcal - want; diff > 0.6 || diff < -0.6 {
		t.Errorf("kcal = %v, esperaba ~%v", entries[0].Macros.Kcal, want)
	}

	// La lista del día ofrece borrar cada línea cargada.
	if code, body := get(t, ts, "/?date="+today); code != http.StatusOK || !strings.Contains(body, `action="/hoy/borrar"`) {
		t.Errorf("el diario no ofrece borrar entradas (code %d)", code)
	}

	// El ítem marcado como "quitar" no se carga.
	code, _ = post(t, ts, "/hoy/confirmar", url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"almuerzo"},
		"text": {"x"}, "items": {"1"},
		"name_0": {food.Name}, "food_id_0": {itoa(food.ID)}, "grams_0": {"100"},
		"quitar_0": {"1"},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("confirmar sin ítems = %d, esperaba 400", code)
	}
	if entries, _ := st.DayEntries(ctx, today); len(entries) != 1 {
		t.Errorf("no debía cargar nada más, hay %d entradas", len(entries))
	}
}

// TestCargaManualYValidaciones: la carga a mano exige macros plausibles y
// rechaza gramos absurdos.
func TestCargaManualYValidaciones(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()
	today := store.Today(time.FixedZone("ART", -3*60*60))

	base := url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"cena"},
		"name": {"Tarta casera"}, "grams": {"180"},
		"kcal": {"210"}, "protein": {"9"}, "fat": {"12"}, "carbs": {"18"},
	}
	code, body := post(t, ts, "/hoy/manual", base)
	if code != http.StatusSeeOther {
		t.Fatalf("carga manual = %d (%s)", code, body)
	}
	entries, _ := st.DayEntries(ctx, today)
	if len(entries) != 1 || entries[0].FoodName != "Tarta casera" {
		t.Fatalf("no se guardó la carga manual: %+v", entries)
	}
	// 210 kcal/100 g * 1,8 = 378 kcal
	if entries[0].Macros.Kcal < 377 || entries[0].Macros.Kcal > 379 {
		t.Errorf("kcal = %v, esperaba ~378", entries[0].Macros.Kcal)
	}

	// Sin macros no hay forma de calcular: 400 y mensaje.
	empty := url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"cena"},
		"name": {"Sin datos"}, "grams": {"100"},
	}
	code, body = post(t, ts, "/hoy/manual", empty)
	if code != http.StatusBadRequest {
		t.Fatalf("carga manual sin macros = %d, esperaba 400", code)
	}
	if !strings.Contains(body, "al menos las calorías") {
		t.Errorf("el error no explica qué falta: %s", body[:200])
	}

	// Gramos fuera de rango.
	bad := url.Values{
		"csrf": {testCSRF}, "date": {today}, "meal": {"cena"},
		"name": {"X"}, "grams": {"9999"}, "kcal": {"100"},
	}
	if code, _ := post(t, ts, "/hoy/manual", bad); code != http.StatusBadRequest {
		t.Fatalf("gramos 9999 = %d, esperaba 400", code)
	}

	// Comida inválida.
	bad.Set("grams", "100")
	bad.Set("meal", "brunch")
	if code, _ := post(t, ts, "/hoy/manual", bad); code != http.StatusBadRequest {
		t.Fatalf("comida inválida = %d, esperaba 400", code)
	}
}

// TestPesoGuardaYBorra cubre el formulario de peso y el borrado del día.
func TestPesoGuardaYBorra(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()
	today := store.Today(time.FixedZone("ART", -3*60*60))

	code, body := post(t, ts, "/progreso/peso", url.Values{
		"csrf": {testCSRF}, "date": {today}, "kg": {"88,4"}, "note": {"en ayunas"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("guardar peso = %d (%s)", code, body)
	}
	rows, err := st.Weights(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].Kg != 88.4 || rows[0].Note != "en ayunas" {
		t.Fatalf("peso mal guardado: %+v", rows)
	}

	// La coma decimal se acepta; un valor absurdo no.
	if code, _ := post(t, ts, "/progreso/peso", url.Values{
		"csrf": {testCSRF}, "date": {today}, "kg": {"900"},
	}); code != http.StatusBadRequest {
		t.Fatalf("peso 900 = %d, esperaba 400", code)
	}

	if code, _ := post(t, ts, "/progreso/peso/borrar", url.Values{
		"csrf": {testCSRF}, "date": {today},
	}); code != http.StatusSeeOther {
		t.Fatalf("borrar peso = %d", code)
	}
	rows, _ = st.Weights(ctx, 10)
	for _, r := range rows {
		if r.Day == today {
			t.Fatalf("el peso del día no se borró: %+v", r)
		}
	}
}

// TestSerieAgregaEditaYBorra recorre el editor de series contra el store.
func TestSerieAgregaEditaYBorra(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()
	today := store.Today(time.FixedZone("ART", -3*60*60))

	routine, err := st.Routine(ctx)
	if err != nil || len(routine) == 0 {
		t.Fatalf("rutina: %v", err)
	}
	ex := routine[0]

	add := url.Values{
		"csrf": {testCSRF}, "date": {today}, "accion": {"guardar"},
		"exercise_id": {itoa(ex.ID)}, "set_number": {"1"},
		"weight": {"80"}, "reps": {"10"}, "rir": {"2"},
		"notes": {"buena sesión"}, "duration": {"55"},
	}
	if code, body := post(t, ts, "/rutina/serie", add); code != http.StatusSeeOther {
		t.Fatalf("agregar serie = %d (%s)", code, body)
	}
	wo, err := st.WorkoutByDay(ctx, today)
	if err != nil {
		t.Fatalf("workout: %v", err)
	}
	if len(wo.Sets) != 1 || wo.Notes != "buena sesión" || wo.DurationMin != 55 {
		t.Fatalf("serie mal guardada: %+v", wo)
	}
	if wo.RoutineDay != ex.Day {
		t.Errorf("routine_day = %q, esperaba %q", wo.RoutineDay, ex.Day)
	}

	// Sin número de serie toma el siguiente libre.
	add.Set("set_number", "")
	add.Set("weight", "82,5")
	add.Set("reps", "8")
	if code, _ := post(t, ts, "/rutina/serie", add); code != http.StatusSeeOther {
		t.Fatalf("agregar segunda serie = %d", code)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if len(wo.Sets) != 2 {
		t.Fatalf("esperaba 2 series, hay %d", len(wo.Sets))
	}
	nums := []int{wo.Sets[0].SetNumber, wo.Sets[1].SetNumber}
	if nums[0] != 1 || nums[1] != 2 {
		t.Errorf("numeración = %v, esperaba [1 2]", nums)
	}
	if wo.Sets[1].WeightKg == nil || *wo.Sets[1].WeightKg != 82.5 {
		t.Errorf("la segunda serie no tomó la coma decimal: %+v", wo.Sets[1])
	}

	// Editar la serie 1.
	edit := url.Values{
		"csrf": {testCSRF}, "date": {today}, "accion": {"guardar"},
		"exercise_id": {itoa(ex.ID)}, "set_number": {"1"},
		"weight": {"85"}, "reps": {"8"}, "rir": {"1"},
		"notes": {"buena sesión"}, "duration": {"55"},
	}
	if code, _ := post(t, ts, "/rutina/serie", edit); code != http.StatusSeeOther {
		t.Fatalf("editar serie = %d", code)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if len(wo.Sets) != 2 {
		t.Fatalf("editar no debe agregar series, hay %d", len(wo.Sets))
	}
	if wo.Sets[0].WeightKg == nil || *wo.Sets[0].WeightKg != 85 {
		t.Errorf("la serie 1 no se actualizó: %+v", wo.Sets[0])
	}

	// Borrar la serie 1: la 2 pasa a ser 1 (sin huecos).
	if code, _ := post(t, ts, "/rutina/serie", url.Values{
		"csrf": {testCSRF}, "date": {today}, "accion": {"borrar"},
		"exercise_id": {itoa(ex.ID)}, "set_number": {"1"},
		"notes": {"buena sesión"}, "duration": {"55"},
	}); code != http.StatusSeeOther {
		t.Fatalf("borrar serie = %d", code)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if len(wo.Sets) != 1 || wo.Sets[0].SetNumber != 1 {
		t.Fatalf("tras borrar: %+v", wo.Sets)
	}
	if wo.Sets[0].WeightKg == nil || *wo.Sets[0].WeightKg != 82.5 {
		t.Errorf("quedó la serie equivocada: %+v", wo.Sets[0])
	}

	// Solo nota del día (sin ejercicio ni series).
	if code, _ := post(t, ts, "/rutina/serie", url.Values{
		"csrf": {testCSRF}, "date": {today}, "accion": {"guardar"},
		"notes": {"solo nota"}, "duration": {"40"},
	}); code != http.StatusSeeOther {
		t.Fatalf("guardar nota = %d", code)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if wo.Notes != "solo nota" || wo.DurationMin != 40 || len(wo.Sets) != 1 {
		t.Fatalf("la nota pisó las series: %+v", wo)
	}

	// Serie con ejercicio inexistente: 400.
	if code, _ := post(t, ts, "/rutina/serie", url.Values{
		"csrf": {testCSRF}, "date": {today}, "accion": {"guardar"},
		"exercise_id": {"99999"}, "weight": {"80"}, "reps": {"10"},
	}); code != http.StatusBadRequest {
		t.Fatalf("ejercicio inexistente = %d, esperaba 400", code)
	}
}

// TestNormalizeSetsYupsertSet cuidan la renumeración: sin esto, borrar una
// serie del medio deja huecos y la sesión se vuelve ilegible.
func TestNormalizeSetsYupsertSet(t *testing.T) {
	kg := func(v float64) *float64 { return &v }
	reps := func(v int) *int { return &v }

	sets := []store.SetInput{
		{ExerciseID: 1, SetNumber: 3, WeightKg: kg(80), Reps: reps(10)},
		{ExerciseID: 1, SetNumber: 1, WeightKg: kg(70), Reps: reps(12)},
		{ExerciseID: 2, SetNumber: 7, WeightKg: kg(50), Reps: reps(15)},
	}
	got := normalizeSets(sets)
	if len(got) != 3 {
		t.Fatalf("normalizeSets perdió series: %+v", got)
	}
	if got[0].ExerciseID != 1 || got[0].SetNumber != 1 || *got[0].WeightKg != 70 {
		t.Errorf("orden del ejercicio 1: %+v", got[0])
	}
	if got[1].SetNumber != 2 || *got[1].WeightKg != 80 {
		t.Errorf("renumeración del ejercicio 1: %+v", got[1])
	}
	if got[2].ExerciseID != 2 || got[2].SetNumber != 1 {
		t.Errorf("renumeración del ejercicio 2: %+v", got[2])
	}

	// set_number vacío = siguiente libre.
	next := upsertSet(got, store.SetInput{ExerciseID: 1, WeightKg: kg(85), Reps: reps(8)})
	if len(next) != 4 {
		t.Fatalf("upsert agregó mal: %+v", next)
	}
	if last := next[len(next)-1]; last.SetNumber != 3 || *last.WeightKg != 85 {
		t.Errorf("la serie nueva no tomó el número 3: %+v", last)
	}

	// Mismo número = reemplazo, no duplicado.
	replaced := upsertSet(next, store.SetInput{ExerciseID: 1, SetNumber: 2, WeightKg: kg(90), Reps: reps(6)})
	if len(replaced) != 4 {
		t.Fatalf("el reemplazo duplicó: %+v", replaced)
	}
	count := 0
	for _, st := range replaced {
		if st.ExerciseID == 1 && st.SetNumber == 2 {
			count++
			if st.WeightKg == nil || *st.WeightKg != 90 {
				t.Errorf("no reemplazó los valores: %+v", st)
			}
		}
	}
	if count != 1 {
		t.Errorf("esperaba una sola serie 2, hay %d", count)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
