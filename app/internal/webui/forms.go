// Formularios del panel: los POST server-rendered (PRG) que convierten la web
// en una app usable sin pasar por el chat.
//
// Reglas de esta capa:
//   - Todo POST exige sesión (la pone el guard) y CSRF (se valida acá).
//   - El cuerpo se acota con MaxBytesReader antes de parsear.
//   - La validación de negocio vive en internal/store: la web no reimplementa
//     reglas ni confía en macros que vengan del formulario.
//   - Después de escribir se redirige (SeeOther) a la vista: recargar no
//     duplica nada.
package webui

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ldgnu/fitordie/internal/estimate"
	"github.com/ldgnu/fitordie/internal/nutrition"
	"github.com/ldgnu/fitordie/internal/store"
)

const (
	// maxFormBody acota cualquier POST del panel.
	maxFormBody = 64 << 10
	// maxPreviewItems limita los ítems que se pueden confirmar de una vez.
	maxPreviewItems = 20
	// maxAnalyzeText es el mismo tope que la API.
	maxAnalyzeText = 600
	// maxNotesLen acota la nota del entrenamiento.
	maxNotesLen = 500
	// maxQueryLen acota el texto de búsqueda de alimentos.
	maxQueryLen = 80
)

// ---------------------------------------------------------------- vistas de formulario

// previewItem es un ítem detectado por el analizador, listo para confirmar.
type previewItem struct {
	Index   int
	Name    string
	FoodID  string // vacío si no matcheó contra foods
	Grams   float64
	Macros  nutrition.Macros
	Matched bool
}

// previewView es el resultado de "Analizar" que se muestra en Hoy.
type previewView struct {
	Date      string
	Meal      string
	Text      string
	Items     []previewItem
	Unmatched []estimate.Unmatched
	Totals    nutrition.Macros
	Source    string
	Note      string
}

// manualFormView conserva lo tipeado cuando la carga manual falla.
type manualFormView struct {
	Meal    string
	Name    string
	Grams   string
	Kcal    string
	Protein string
	Fat     string
	Carbs   string
	Fiber   string
	Sugar   string
	Sodium  string
}

// weightFormView conserva lo tipeado en el formulario de peso.
type weightFormView struct {
	Date string
	Kg   string
	Note string
}

// setRowView es una serie existente del día, editable en el lugar.
type setRowView struct {
	ExerciseID   int64
	ExerciseName string
	SetNumber    int
	Weight       string
	Reps         string
	RIR          string
}

// setGroupView agrupa las series de un ejercicio.
type setGroupView struct {
	ExerciseID   int64
	ExerciseName string
	Sets         []setRowView
	Volume       float64
}

// workoutEditorView es la sesión del día tal como se edita en Rutina.
type workoutEditorView struct {
	Date        string
	RoutineDay  string
	DayName     string
	Notes       string
	DurationMin int
	Groups      []setGroupView
	SetCount    int
	Volume      float64
	Exists      bool
}

// setFormView conserva lo tipeado en el formulario de series.
type setFormView struct {
	Date       string
	RoutineDay string
	ExerciseID string
	SetNumber  string
	Weight     string
	Reps       string
	RIR        string
	Notes      string
	Duration   string
}

// ---------------------------------------------------------------- entrada de formularios

// readForm acota y parsea el cuerpo y valida el CSRF. Devuelve false si ya
// respondió (el handler debe cortar).
func (s *Server) readForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	if err := r.ParseForm(); err != nil {
		s.deny(w, http.StatusBadRequest, "No pudimos leer el formulario. Probá de nuevo.")
		return false
	}
	if s.csrf == nil || !s.csrf.ValidCSRFToken(r) {
		s.deny(w, http.StatusForbidden, "La página venció. Recargá y volvé a intentar.")
		return false
	}
	return true
}

// deny corta con un error mínimo sin depender de templates.
func (s *Server) deny(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = fmt.Fprintln(w, msg)
}

// field devuelve el valor de un campo, recortado y sin pasar maxLen runas.
func field(r *http.Request, name string, maxLen int) string {
	v := strings.TrimSpace(r.PostFormValue(name))
	if maxLen > 0 && utf8.RuneCountInString(v) > maxLen {
		v = string([]rune(v)[:maxLen])
	}
	return v
}

// number parsea un número aceptando coma decimal (89,65) y espacios.
func number(raw string) (float64, bool) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// fieldNum devuelve el número del campo (0 si vacío o inválido).
func fieldNum(r *http.Request, name string) float64 {
	v, _ := number(r.PostFormValue(name))
	return v
}

// fieldNumOK distingue "vacío" de "0" (peso 0 es válido: serie sin carga).
func fieldNumOK(r *http.Request, name string) (float64, bool) {
	return number(r.PostFormValue(name))
}

// formDate resuelve la fecha del POST, con fallback al día de hoy.
func (s *Server) formDate(r *http.Request, fallback string) string {
	if v := strings.TrimSpace(r.PostFormValue("date")); v != "" {
		if day, err := store.ParseDay(v); err == nil {
			return day
		}
	}
	return fallback
}

// redirectWith redirige a path con query (patrón PRG).
func redirectWith(w http.ResponseWriter, r *http.Request, path string, params map[string]string) {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	target := path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// flash arma el mensaje de éxito a partir de la query de la vista.
func flash(q url.Values) string {
	switch q.Get("ok") {
	case "comida":
		n := q.Get("n")
		if n == "" {
			n = "0"
		}
		if n == "1" {
			return "Se cargó 1 ítem al diario."
		}
		return "Se cargaron " + n + " ítems al diario."
	case "alimento":
		return "Alimento cargado."
	case "manual":
		return "Carga manual agregada."
	case "borrado":
		return "Registro borrado."
	case "peso":
		return "Peso guardado."
	case "peso_borrado":
		return "Peso borrado."
	case "serie":
		return "Serie guardada."
	case "serie_borrada":
		return "Serie borrada."
	case "nota":
		return "Nota del día guardada."
	}
	return ""
}

// ---------------------------------------------------------------- Hoy: analizar

// hoyAnalyze corre el parser sobre el texto y muestra la previsualización.
func (s *Server) hoyAnalyze(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	d, err := s.hoyView(r, day)
	if err != nil {
		s.fail(w, err)
		return
	}

	meal := field(r, "meal", 32)
	text := field(r, "text", maxAnalyzeText+50)
	d.Analyze = &analyzeFormView{Meal: meal, Text: text}
	d.Manual = &manualFormView{Meal: meal}

	if text == "" {
		d.Err = "Escribí qué comiste (por ejemplo: 150 g de pollo y una taza de arroz)."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}
	if !store.ValidMeal(meal) {
		d.Err = "Elegí una comida válida."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(text) > maxAnalyzeText {
		d.Err = fmt.Sprintf("El texto es muy largo (máximo %d caracteres).", maxAnalyzeText)
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	res, err := s.est.Run(r.Context(), text)
	if err != nil {
		slog.Error("webui: estimar", "err", err)
		d.Err = "No pude analizar el texto. Probá de nuevo o cargalo a mano."
		s.render(w, r, "hoy.html", d, http.StatusInternalServerError)
		return
	}

	pv := &previewView{
		Date: day, Meal: meal, Text: text, Totals: res.Totals,
		Source: res.Source, Note: res.Note,
		Unmatched: res.Unmatched,
	}
	for i, it := range res.Items {
		foodID := ""
		if it.Matched && it.FoodID > 0 {
			foodID = strconv.FormatInt(it.FoodID, 10)
		}
		pv.Items = append(pv.Items, previewItem{
			Index: i, Name: it.FoodName, FoodID: foodID,
			Grams: it.Grams, Macros: it.Macros.Rounded(), Matched: it.Matched,
		})
	}
	d.Preview = pv

	if len(pv.Items) == 0 {
		if len(pv.Unmatched) > 0 {
			d.Err = "No reconocí ninguno de los alimentos. Cargalos a mano o buscá el alimento en la base."
		} else {
			d.Err = "No encontré alimentos en el texto. Probá con cantidades (por ejemplo: 150 g de pollo)."
		}
	}

	s.render(w, r, "hoy.html", d, http.StatusOK)
}

// hoyConfirm carga de una vez los ítems que quedaron marcados en la
// previsualización. El servidor recalcula los macros: solo usa food_id +
// gramos, nunca los macros del formulario.
func (s *Server) hoyConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	d, err := s.hoyView(r, day)
	if err != nil {
		s.fail(w, err)
		return
	}

	meal := field(r, "meal", 32)
	text := field(r, "text", maxAnalyzeText+50)
	d.Analyze = &analyzeFormView{Meal: meal, Text: text}
	d.Manual = &manualFormView{Meal: meal}

	if !store.ValidMeal(meal) {
		d.Err = "Elegí una comida válida."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	n := int(fieldNum(r, "items"))
	if n < 0 {
		n = 0
	}
	if n > maxPreviewItems {
		n = maxPreviewItems
	}

	type toLoad struct {
		name   string
		foodID int64
		grams  float64
	}
	var pending []toLoad
	for i := 0; i < n; i++ {
		idx := strconv.Itoa(i)
		if r.PostFormValue("quitar_"+idx) != "" {
			continue
		}
		name := field(r, "name_"+idx, 120)
		grams := fieldNum(r, "grams_"+idx)
		foodID := int64(fieldNum(r, "food_id_"+idx))
		if grams <= 0 || grams > 5000 {
			d.Err = fmt.Sprintf("Los gramos de «%s» están fuera de rango (0 a 5000 g).", name)
			s.render(w, r, "hoy.html", d, http.StatusBadRequest)
			return
		}
		pending = append(pending, toLoad{name: name, foodID: foodID, grams: grams})
	}
	if len(pending) == 0 {
		d.Err = "No quedó ningún ítem para cargar."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	loaded := 0
	for _, p := range pending {
		if p.foodID <= 0 {
			d.Err = fmt.Sprintf("«%s» no está en la base de alimentos: cargalo a mano.", p.name)
			s.render(w, r, "hoy.html", d, http.StatusBadRequest)
			return
		}
		id := p.foodID
		if _, err := s.st.AddEntry(r.Context(), store.EntryInput{
			Day: day, Meal: meal, FoodID: &id, Grams: p.grams,
		}); err != nil {
			slog.Error("webui: confirmar comida", "err", err, "item", p.name)
			d.Err = fmt.Sprintf("Se cargaron %d de %d ítems. Falló «%s»: %s", loaded, len(pending), p.name, friendly(err))
			s.render(w, r, "hoy.html", d, http.StatusBadRequest)
			return
		}
		loaded++
	}

	redirectWith(w, r, "/", map[string]string{
		"date": day, "ok": "comida", "n": strconv.Itoa(loaded),
	})
}

// hoyManual carga un alimento escrito a mano con sus valores por 100 g.
func (s *Server) hoyManual(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	d, err := s.hoyView(r, day)
	if err != nil {
		s.fail(w, err)
		return
	}

	mv := &manualFormView{
		Meal:    field(r, "meal", 32),
		Name:    field(r, "name", 120),
		Grams:   field(r, "grams", 16),
		Kcal:    field(r, "kcal", 16),
		Protein: field(r, "protein", 16),
		Fat:     field(r, "fat", 16),
		Carbs:   field(r, "carbs", 16),
		Fiber:   field(r, "fiber", 16),
		Sugar:   field(r, "sugar", 16),
		Sodium:  field(r, "sodium", 16),
	}
	d.Manual = mv
	d.Analyze = &analyzeFormView{Meal: mv.Meal}

	if mv.Name == "" {
		d.Err = "Poné el nombre del alimento."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}
	grams, ok := number(mv.Grams)
	if !ok {
		d.Err = "Poné los gramos de la porción."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}
	per100 := nutrition.Per100{
		Kcal:    fieldNum(r, "kcal"),
		Protein: fieldNum(r, "protein"),
		Fat:     fieldNum(r, "fat"),
		Carbs:   fieldNum(r, "carbs"),
		Fiber:   fieldNum(r, "fiber"),
		Sugar:   fieldNum(r, "sugar"),
		Sodium:  fieldNum(r, "sodium"),
	}
	if per100.Kcal == 0 && per100.Protein == 0 && per100.Fat == 0 && per100.Carbs == 0 {
		d.Err = "Cargá al menos las calorías por 100 g (o los macros)."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	if _, err := s.st.AddEntry(r.Context(), store.EntryInput{
		Day: day, Meal: mv.Meal, FoodName: mv.Name, Grams: grams, Per100: &per100,
	}); err != nil {
		d.Err = friendly(err)
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	redirectWith(w, r, "/", map[string]string{"date": day, "ok": "manual"})
}

// hoyFood carga un alimento elegido de la búsqueda en la base.
func (s *Server) hoyFood(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	d, err := s.hoyView(r, day)
	if err != nil {
		s.fail(w, err)
		return
	}

	meal := field(r, "meal", 32)
	d.Analyze = &analyzeFormView{Meal: meal}
	d.Manual = &manualFormView{Meal: meal}

	foodID := int64(fieldNum(r, "food_id"))
	grams := fieldNum(r, "grams")
	if foodID <= 0 {
		d.Err = "Elegí un alimento de la lista."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}
	if grams <= 0 {
		d.Err = "Poné los gramos de la porción."
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	if _, err := s.st.AddEntry(r.Context(), store.EntryInput{
		Day: day, Meal: meal, FoodID: &foodID, Grams: grams,
	}); err != nil {
		d.Err = friendly(err)
		s.render(w, r, "hoy.html", d, http.StatusBadRequest)
		return
	}

	redirectWith(w, r, "/", map[string]string{"date": day, "ok": "alimento"})
}

// hoyDelete borra una línea del diario.
func (s *Server) hoyDelete(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	id := int64(fieldNum(r, "entry_id"))
	if id <= 0 {
		s.deny(w, http.StatusBadRequest, "Registro inválido.")
		return
	}
	if err := s.st.DeleteEntry(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.deny(w, http.StatusNotFound, "Ese registro ya no existe.")
			return
		}
		slog.Error("webui: borrar entrada", "err", err)
		s.deny(w, http.StatusInternalServerError, "No pude borrar el registro.")
		return
	}
	redirectWith(w, r, "/", map[string]string{"date": day, "ok": "borrado"})
}

// ---------------------------------------------------------------- Progreso: peso

func (s *Server) progresoPeso(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	d, err := s.progresoView(r, day)
	if err != nil {
		s.fail(w, err)
		return
	}
	kgRaw := field(r, "kg", 16)
	note := field(r, "note", 200)
	d.WeightForm = &weightFormView{Date: day, Kg: kgRaw, Note: note}

	kg, ok := number(kgRaw)
	if !ok {
		d.Err = "Poné el peso en kilos (por ejemplo 89,65)."
		s.render(w, r, "progreso.html", d, http.StatusBadRequest)
		return
	}
	if kg < 20 || kg > 400 {
		d.Err = "El peso tiene que estar entre 20 y 400 kg."
		s.render(w, r, "progreso.html", d, http.StatusBadRequest)
		return
	}
	if err := s.st.SaveWeight(r.Context(), day, kg, note); err != nil {
		slog.Error("webui: guardar peso", "err", err)
		d.Err = "No pude guardar el peso."
		s.render(w, r, "progreso.html", d, http.StatusInternalServerError)
		return
	}
	redirectWith(w, r, "/progreso", map[string]string{"ok": "peso"})
}

func (s *Server) progresoPesoBorrar(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	if err := s.st.DeleteWeight(r.Context(), day); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.deny(w, http.StatusNotFound, "Ese día no tiene peso cargado.")
			return
		}
		slog.Error("webui: borrar peso", "err", err)
		s.deny(w, http.StatusInternalServerError, "No pude borrar el peso.")
		return
	}
	redirectWith(w, r, "/progreso", map[string]string{"ok": "peso_borrado"})
}

// ---------------------------------------------------------------- Rutina: series

// rutinaSerie agrega, reemplaza o borra una serie del día. La API reemplaza
// todas las series del entrenamiento, así que siempre se manda la lista
// completa: se lee la sesión, se aplica el cambio y se guarda entera.
func (s *Server) rutinaSerie(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	d, err := s.rutinaView(r, day)
	if err != nil {
		s.fail(w, err)
		return
	}

	accion := field(r, "accion", 16)
	exerciseID := int64(fieldNum(r, "exercise_id"))
	setNumber := int(fieldNum(r, "set_number"))
	weightRaw := field(r, "weight", 16)
	repsRaw := field(r, "reps", 16)
	rirRaw := field(r, "rir", 16)
	notes := field(r, "notes", maxNotesLen)
	duration := int(fieldNum(r, "duration"))

	d.SetForm = &setFormView{
		Date: day, ExerciseID: strconv.FormatInt(exerciseID, 10),
		SetNumber: strconv.Itoa(setNumber), Weight: weightRaw, Reps: repsRaw,
		RIR: rirRaw, Notes: notes, Duration: strconv.Itoa(duration),
	}

	routine, err := s.st.Routine(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	byID := make(map[int64]store.Exercise, len(routine))
	for _, e := range routine {
		byID[e.ID] = e
	}

	// Sesión vigente del día (puede no existir todavía).
	var sets []store.SetInput
	routineDay := ""
	if wo, err := s.st.WorkoutByDay(r.Context(), day); err == nil {
		sets = append(sets, wo.Sets...)
		routineDay = wo.RoutineDay
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	if routineDay == "" {
		routineDay = "A"
	}

	if accion == "borrar" {
		ex, ok := byID[exerciseID]
		if !ok {
			d.Err = "Elegí un ejercicio válido para borrar la serie."
			s.render(w, r, "rutina.html", d, http.StatusBadRequest)
			return
		}
		if routineDay == "" {
			routineDay = ex.Day
		}
		filtered := sets[:0]
		removed := false
		for _, st := range sets {
			if st.ExerciseID == exerciseID && st.SetNumber == setNumber {
				removed = true
				continue
			}
			filtered = append(filtered, st)
		}
		if !removed {
			d.Err = "Esa serie ya no existe. Recargá la página."
			s.render(w, r, "rutina.html", d, http.StatusBadRequest)
			return
		}
		sets = filtered
	} else {
		// ¿Es solo guardar la nota del día? Sin peso y sin reps no hay serie.
		_, hasWeight := fieldNumOK(r, "weight")
		_, hasReps := fieldNumOK(r, "reps")
		if !hasWeight && !hasReps && exerciseID == 0 {
			if notes == "" && duration <= 0 {
				d.Err = "Poné peso y reps para cargar una serie, o una nota para el día."
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			if duration < 0 || duration > 600 {
				d.Err = "La duración tiene que estar entre 0 y 600 minutos."
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			if _, err := s.st.SaveWorkout(r.Context(), day, routineDay, notes, duration, sets); err != nil {
				slog.Error("webui: guardar nota", "err", err)
				d.Err = "No pude guardar la nota."
				s.render(w, r, "rutina.html", d, http.StatusInternalServerError)
				return
			}
			redirectWith(w, r, "/rutina", map[string]string{"date": day, "ok": "nota"})
			return
		}

		ex, ok := byID[exerciseID]
		if !ok {
			d.Err = "Elegí el ejercicio de la serie."
			s.render(w, r, "rutina.html", d, http.StatusBadRequest)
			return
		}
		routineDay = ex.Day

		var weight *float64
		var repsInt, rirInt *int
		if hasWeight {
			wv := fieldNum(r, "weight")
			if wv < 0 || wv > 1000 {
				d.Err = "El peso tiene que estar entre 0 y 1000 kg."
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			weight = &wv
		}
		if hasReps {
			rv := fieldNum(r, "reps")
			if rv < 0 || rv > 200 {
				d.Err = "Las reps tienen que estar entre 0 y 200."
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			ri := int(rv)
			repsInt = &ri
		}
		if rirRaw != "" {
			rv, ok := number(rirRaw)
			if !ok || rv < 0 || rv > 10 {
				d.Err = "El RIR tiene que estar entre 0 y 10."
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			ri := int(rv)
			rirInt = &ri
		}

		sets = upsertSet(sets, store.SetInput{
			ExerciseID: ex.ID, SetNumber: setNumber,
			WeightKg: weight, Reps: repsInt, RIR: rirInt,
		})
	}

	sets = normalizeSets(sets)

	if duration < 0 || duration > 600 {
		d.Err = "La duración tiene que estar entre 0 y 600 minutos."
		s.render(w, r, "rutina.html", d, http.StatusBadRequest)
		return
	}
	if _, err := s.st.SaveWorkout(r.Context(), day, routineDay, notes, duration, sets); err != nil {
		slog.Error("webui: guardar serie", "err", err)
		d.Err = "No pude guardar la serie."
		s.render(w, r, "rutina.html", d, http.StatusInternalServerError)
		return
	}

	ok := "serie"
	if accion == "borrar" {
		ok = "serie_borrada"
	}
	redirectWith(w, r, "/rutina", map[string]string{"date": day, "ok": ok})
}

// upsertSet inserta la serie o reemplaza la que tiene el mismo número dentro
// del mismo ejercicio. setNumber 0 = siguiente libre.
func upsertSet(sets []store.SetInput, in store.SetInput) []store.SetInput {
	next := 1
	for _, st := range sets {
		if st.ExerciseID != in.ExerciseID {
			continue
		}
		if in.SetNumber > 0 && st.SetNumber == in.SetNumber {
			// Reemplazo en el lugar: conserva el orden de la sesión.
			replaced := in
			replaced.SetNumber = st.SetNumber
			out := make([]store.SetInput, 0, len(sets))
			for _, other := range sets {
				if other.ExerciseID == st.ExerciseID && other.SetNumber == st.SetNumber {
					out = append(out, replaced)
					continue
				}
				out = append(out, other)
			}
			return out
		}
		if st.SetNumber >= next {
			next = st.SetNumber + 1
		}
	}
	if in.SetNumber <= 0 {
		in.SetNumber = next
	}
	return append(sets, in)
}

// normalizeSets renumera las series de cada ejercicio 1..n, en orden, y deja
// la lista ordenada por ejercicio y número. Sin esto, borrar la serie 2 deja
// huecos (1 y 3) y la sesión se vuelve ilegible.
func normalizeSets(sets []store.SetInput) []store.SetInput {
	byEx := make(map[int64][]store.SetInput, len(sets))
	order := make([]int64, 0, len(sets))
	for _, st := range sets {
		if _, seen := byEx[st.ExerciseID]; !seen {
			order = append(order, st.ExerciseID)
		}
		byEx[st.ExerciseID] = append(byEx[st.ExerciseID], st)
	}
	out := make([]store.SetInput, 0, len(sets))
	for _, id := range order {
		list := byEx[id]
		sort.SliceStable(list, func(i, j int) bool { return list[i].SetNumber < list[j].SetNumber })
		for i := range list {
			list[i].SetNumber = i + 1
			out = append(out, list[i])
		}
	}
	return out
}

// ---------------------------------------------------------------- helpers

// friendly traduce los errores del store a un mensaje para mostrar.
func friendly(err error) string {
	if errors.Is(err, store.ErrInvalidInput) {
		return strings.TrimPrefix(err.Error(), store.ErrInvalidInput.Error()+": ")
	}
	if errors.Is(err, store.ErrNotFound) {
		return "No encontré ese registro."
	}
	return "No pude guardar el registro."
}
