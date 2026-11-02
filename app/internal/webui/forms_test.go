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
		{"/rutina", []string{`action="/rutina/hoja"`, `class="tabs"`, `name="day"`}},
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

// TestHojaGuardaSeriesCompletas recorre la hoja del día: todas las series de
// todos los ejercicios en un solo POST, con la semántica de POST /api/workout
// (lo que llega es la lista final del día: agregar, editar y borrar se hacen
// sobre la hoja y se guarda entera).
func TestHojaGuardaSeriesCompletas(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()
	today := store.Today(time.FixedZone("ART", -3*60*60))

	routine, err := st.Routine(ctx)
	if err != nil || len(routine) == 0 {
		t.Fatalf("rutina: %v", err)
	}
	var dayA, dayB []store.Exercise
	for _, e := range routine {
		switch e.Day {
		case "A":
			dayA = append(dayA, e)
		case "B":
			dayB = append(dayB, e)
		}
	}
	if len(dayA) < 2 || len(dayB) == 0 {
		t.Fatalf("rutina incompleta: A=%d B=%d", len(dayA), len(dayB))
	}
	ex1, ex2 := dayA[0], dayA[1]

	// GET de la hoja: trae el formulario completo del día A y no mezcla el B.
	code, body := get(t, ts, "/rutina?day=A")
	if code != http.StatusOK {
		t.Fatalf("GET /rutina?day=A = %d", code)
	}
	for _, want := range []string{
		`action="/rutina/hoja"`, `name="day" value="A"`, `class="tabs"`,
		`name="ex" value="` + itoa(ex1.ID) + `"`, ex1.Name, "Día C",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("la hoja del día A no trae %q", want)
		}
	}
	if strings.Contains(body, dayB[0].Name) {
		t.Errorf("la hoja del día A no debe listar el ejercicio %q del día B", dayB[0].Name)
	}
	if !strings.Contains(body, `href="/rutina?date=` + today + `&amp;day=B"`) {
		t.Errorf("faltan las pestañas que cambian de día")
	}

	// Hoja completa: dos series de ex1 (una con coma decimal y nota) y una de
	// ex2 sin peso, más una fila vacía que no debe guardarse.
	sheet := url.Values{
		"csrf": {testCSRF}, "date": {today}, "day": {"A"},
		"day_notes": {"buena sesión"}, "duration": {"55"},
		"ex":   {itoa(ex1.ID), itoa(ex1.ID), itoa(ex1.ID), itoa(ex2.ID)},
		"kg":   {"80", "82,5", "", ""},
		"reps": {"10", "8", "", "12"},
		"rir":  {"2", "3", "", ""},
		"note": {"última lenta", "", "", ""},
	}
	if code, body := post(t, ts, "/rutina/hoja", sheet); code != http.StatusSeeOther {
		t.Fatalf("guardar hoja = %d (%s)", code, body)
	}

	wo, err := st.WorkoutByDay(ctx, today)
	if err != nil {
		t.Fatalf("workout: %v", err)
	}
	if wo.RoutineDay != "A" || wo.Notes != "buena sesión" || wo.DurationMin != 55 {
		t.Fatalf("cabecera del día mal guardada: %+v", wo)
	}
	if len(wo.Sets) != 3 {
		t.Fatalf("esperaba 3 series, hay %d: %+v", len(wo.Sets), wo.Sets)
	}
	if wo.Sets[0].ExerciseID != ex1.ID || wo.Sets[0].SetNumber != 1 ||
		wo.Sets[0].WeightKg == nil || *wo.Sets[0].WeightKg != 80 ||
		wo.Sets[0].Reps == nil || *wo.Sets[0].Reps != 10 ||
		wo.Sets[0].RIR == nil || *wo.Sets[0].RIR != 2 ||
		wo.Sets[0].Note != "última lenta" {
		t.Errorf("primera serie mal guardada: %+v", wo.Sets[0])
	}
	if wo.Sets[1].SetNumber != 2 || wo.Sets[1].WeightKg == nil || *wo.Sets[1].WeightKg != 82.5 {
		t.Errorf("la coma decimal no llegó a la serie 2: %+v", wo.Sets[1])
	}
	if wo.Sets[2].ExerciseID != ex2.ID || wo.Sets[2].SetNumber != 1 ||
		wo.Sets[2].WeightKg != nil || wo.Sets[2].Reps == nil || *wo.Sets[2].Reps != 12 {
		t.Errorf("serie sin peso mal guardada: %+v", wo.Sets[2])
	}

	// El día guardado vuelve en la hoja con el último registro del ejercicio
	// (la mejor serie del día: 82,5 kg × 8 le gana a 80 kg × 10).
	_, body = get(t, ts, "/rutina?date="+today+"&day=A")
	for _, want := range []string{"Último registro", "82,5 kg × 8", "buena sesión"} {
		if !strings.Contains(body, want) {
			t.Errorf("la hoja guardada no muestra %q", want)
		}
	}

	// La hoja se reemplaza completa: si sólo se manda una serie de ex1, la de
	// ex2 desaparece (borrar = no mandarla / dejar la fila vacía).
	only := url.Values{
		"csrf": {testCSRF}, "date": {today}, "day": {"A"},
		"day_notes": {"buena sesión"}, "duration": {"55"},
		"ex":   {itoa(ex1.ID), itoa(ex1.ID)},
		"kg":   {"85", ""},
		"reps": {"8", ""},
		"rir":  {"1", ""},
		"note": {"", ""},
	}
	if code, body := post(t, ts, "/rutina/hoja", only); code != http.StatusSeeOther {
		t.Fatalf("reemplazar hoja = %d (%s)", code, body)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if len(wo.Sets) != 1 || wo.Sets[0].WeightKg == nil || *wo.Sets[0].WeightKg != 85 {
		t.Fatalf("la hoja no se reemplazó completa: %+v", wo.Sets)
	}

	// Hoja vacía: deja el entrenamiento sin series pero conserva nota y duración.
	empty := url.Values{
		"csrf": {testCSRF}, "date": {today}, "day": {"A"},
		"day_notes": {"solo nota"}, "duration": {"40"},
		"ex": {itoa(ex1.ID)}, "kg": {""}, "reps": {""}, "rir": {""}, "note": {""},
	}
	if code, _ := post(t, ts, "/rutina/hoja", empty); code != http.StatusSeeOther {
		t.Fatalf("guardar hoja sin series = %d", code)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if len(wo.Sets) != 0 || wo.Notes != "solo nota" || wo.DurationMin != 40 {
		t.Fatalf("la hoja vacía pisó mal la sesión: %+v", wo)
	}

	// Un ejercicio que no es del día: 400 y no se toca nada.
	bad := url.Values{
		"csrf": {testCSRF}, "date": {today}, "day": {"A"},
		"ex":   {itoa(dayB[0].ID)},
		"kg":   {"50"}, "reps": {"10"}, "rir": {""}, "note": {""},
	}
	if code, body := post(t, ts, "/rutina/hoja", bad); code != http.StatusBadRequest {
		t.Fatalf("ejercicio de otro día = %d, esperaba 400 (%s)", code, body)
	}
	// Nota sin peso ni reps no es una serie: 400 en vez de guardar basura.
	lonely := url.Values{
		"csrf": {testCSRF}, "date": {today}, "day": {"A"},
		"ex":   {itoa(ex1.ID)},
		"kg":   {""}, "reps": {""}, "rir": {""}, "note": {"me dolió"},
	}
	if code, body := post(t, ts, "/rutina/hoja", lonely); code != http.StatusBadRequest {
		t.Fatalf("nota sin serie = %d, esperaba 400 (%s)", code, body)
	}
	// Rango absurdo.
	absurd := url.Values{
		"csrf": {testCSRF}, "date": {today}, "day": {"A"},
		"ex":   {itoa(ex1.ID)},
		"kg":   {"5000"}, "reps": {"10"}, "rir": {""}, "note": {""},
	}
	if code, _ := post(t, ts, "/rutina/hoja", absurd); code != http.StatusBadRequest {
		t.Fatalf("peso 5000 = %d, esperaba 400", code)
	}
	// Sin CSRF no se guarda nada.
	noCSRF := url.Values{
		"date": {today}, "day": {"A"},
		"ex":   {itoa(ex1.ID)},
		"kg":   {"90"}, "reps": {"5"},
	}
	if code, _ := post(t, ts, "/rutina/hoja", noCSRF); code != http.StatusForbidden {
		t.Fatalf("hoja sin csrf = %d, esperaba 403", code)
	}
	wo, _ = st.WorkoutByDay(ctx, today)
	if len(wo.Sets) != 0 {
		t.Fatalf("un POST rechazado no debe escribir series: %+v", wo.Sets)
	}
}

// TestHojaEligeElDiaQueToca cuida la resolución del día activo: lo pedido
// manda, después lo ya cargado y, si no hay nada, el que toca por calendario.
func TestHojaEligeElDiaQueToca(t *testing.T) {
	_, st, ts := newWeb(t)
	ctx := context.Background()

	// Lunes 2026-11-02 → día A por rotación (lunes = A).
	_, body := get(t, ts, "/rutina?date=2026-11-02")
	if !strings.Contains(body, `class="tab is-active" href="/rutina?date=2026-11-02&amp;day=A"`) {
		t.Errorf("el lunes debería abrir el día A: %s", body[:300])
	}
	// Martes → B.
	_, body = get(t, ts, "/rutina?date=2026-11-03")
	if !strings.Contains(body, `day=B"`) || !strings.Contains(body, `href="/rutina?date=2026-11-03&amp;day=B"`) {
		t.Errorf("el martes debería abrir el día B")
	}
	// Si el día ya tiene entrenamiento cargado, manda ese día.
	if _, err := st.SaveWorkout(ctx, "2026-11-03", "C", "", 0, nil); err != nil {
		t.Fatalf("workout previo: %v", err)
	}
	_, body = get(t, ts, "/rutina?date=2026-11-03")
	if !strings.Contains(body, `class="tab is-active" href="/rutina?date=2026-11-03&amp;day=C"`) {
		t.Errorf("con sesión C cargada, la hoja debe abrir C")
	}
}


func itoa(v int64) string { return strconv.FormatInt(v, 10) }
