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
	case "hoja":
		return "Hoja del día guardada."
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

// ---------------------------------------------------------------- Rutina: hoja del día

// maxSheetRows acota las filas que puede mandar una hoja (3 días x 30 series).
const maxSheetRows = 200

// maxSetNote acota la nota libre de una serie.
const maxSetNote = 200

// sheetRow es una fila cruda de la hoja, tal como llegó del formulario: se
// guarda así para poder devolver el formulario con lo tipeado si algo falla.
type sheetRow struct {
	exID int64
	kg   string
	reps string
	rir  string
	note string
}

// at devuelve vals[i] o "" si el índice no existe (los navegadores mandan
// siempre un valor por input, pero no confiamos en eso).
func at(vals []string, i int) string {
	if i < 0 || i >= len(vals) {
		return ""
	}
	return vals[i]
}

// parseSheetRows lee las filas paralelas del formulario (ex/kg/reps/rir/note).
func parseSheetRows(r *http.Request) ([]sheetRow, error) {
	exVals := r.PostForm["ex"]
	if len(exVals) > maxSheetRows {
		return nil, fmt.Errorf("demasiadas filas (máximo %d)", maxSheetRows)
	}
	kgVals, repsVals := r.PostForm["kg"], r.PostForm["reps"]
	rirVals, noteVals := r.PostForm["rir"], r.PostForm["note"]

	rows := make([]sheetRow, 0, len(exVals))
	for i := range exVals {
		raw := strings.TrimSpace(exVals[i])
		if raw == "" {
			continue
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("hay una fila sin un ejercicio válido")
		}
		rows = append(rows, sheetRow{
			exID: id, kg: at(kgVals, i), reps: at(repsVals, i),
			rir: at(rirVals, i), note: at(noteVals, i),
		})
	}
	return rows, nil
}

// fillSheetFromRows devuelve a la hoja lo que el usuario tipeó. Se usa cuando
// una validación falla: perder 12 filas de series por un typo sería peor que
// el error.
func fillSheetFromRows(d *viewData, rows []sheetRow) {
	if d.Sheet == nil {
		return
	}
	byEx := make(map[int64][]sheetRow, len(rows))
	for _, row := range rows {
		byEx[row.exID] = append(byEx[row.exID], row)
	}
	for i := range d.Sheet.Exercises {
		ev := &d.Sheet.Exercises[i]
		ev.Rows = nil
		for n, row := range byEx[ev.ID] {
			ev.Rows = append(ev.Rows, setRowView{
				ExerciseID: ev.ID, ExerciseName: ev.Name, SetNumber: n + 1,
				Weight: strings.TrimSpace(row.kg), Reps: strings.TrimSpace(row.reps),
				RIR: strings.TrimSpace(row.rir), Note: strings.TrimSpace(row.note),
			})
		}
		ev.Rows = append(ev.Rows, setRowView{ExerciseID: ev.ID, ExerciseName: ev.Name})
		ev.HasSets = len(ev.Rows) > 1
	}
}

// rutinaHoja guarda la hoja completa de un día: las series de todos los
// ejercicios del día en una sola pasada, más la nota y la duración.
//
// Semántica igual a POST /api/workout: lo que llega es la lista final del día.
// Agregar, editar y borrar se hace sobre la hoja (borrar = dejar la fila
// vacía) y se guarda entera, así que la sesión nunca queda con huecos ni
// series duplicadas.
func (s *Server) rutinaHoja(w http.ResponseWriter, r *http.Request) {
	if !s.readForm(w, r) {
		return
	}
	day := s.formDate(r, store.Today(s.loc))
	dayKey := field(r, "day", 1)
	d, err := s.rutinaView(r, day, dayKey)
	if err != nil {
		s.fail(w, err)
		return
	}
	if d.Sheet == nil || d.Sheet.Key == "" {
		s.deny(w, http.StatusBadRequest, "La rutina no está sembrada.")
		return
	}
	if dayKey != "" && dayKey != d.Sheet.Key {
		d.Err = "Ese día no existe en la rutina."
		s.render(w, r, "rutina.html", d, http.StatusBadRequest)
		return
	}
	dayKey = d.Sheet.Key

	// Lo tipeado del pie de la hoja, para poder devolverlo si algo falla.
	notes := field(r, "day_notes", maxNotesLen)
	duration := int(fieldNum(r, "duration"))
	d.Sheet.Notes, d.Sheet.DurationMin = notes, duration

	rows, err := parseSheetRows(r)
	if err != nil {
		d.Err = err.Error()
		s.render(w, r, "rutina.html", d, http.StatusBadRequest)
		return
	}
	fillSheetFromRows(d, rows)

	if duration < 0 || duration > 600 {
		d.Err = "La duración tiene que estar entre 0 y 600 minutos."
		s.render(w, r, "rutina.html", d, http.StatusBadRequest)
		return
	}

	allowed := make(map[int64]bool, len(d.Sheet.Exercises))
	for _, e := range d.Sheet.Exercises {
		allowed[e.ID] = true
	}

	perEx := make(map[int64][]store.SetInput, len(allowed))
	for n, row := range rows {
		if !allowed[row.exID] {
			d.Err = fmt.Sprintf("La fila %d es de un ejercicio que no pertenece al día %s.", n+1, dayKey)
			s.render(w, r, "rutina.html", d, http.StatusBadRequest)
			return
		}
		kgRaw, repsRaw := strings.TrimSpace(row.kg), strings.TrimSpace(row.reps)
		rirRaw, note := strings.TrimSpace(row.rir), strings.TrimSpace(row.note)
		if kgRaw == "" && repsRaw == "" && rirRaw == "" && note == "" {
			continue // fila vacía: no es una serie
		}

		var kg *float64
		if kgRaw != "" {
			v, ok := number(kgRaw)
			if !ok || v < 0 || v > 1000 {
				d.Err = fmt.Sprintf("Fila %d: el peso tiene que estar entre 0 y 1000 kg.", n+1)
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			kg = &v
		}
		var repsInt *int
		if repsRaw != "" {
			v, ok := number(repsRaw)
			if !ok || v < 0 || v > 200 {
				d.Err = fmt.Sprintf("Fila %d: las reps tienen que estar entre 0 y 200.", n+1)
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			ri := int(v)
			repsInt = &ri
		}
		var rirInt *int
		if rirRaw != "" {
			v, ok := number(rirRaw)
			if !ok || v < 0 || v > 10 {
				d.Err = fmt.Sprintf("Fila %d: el RIR tiene que estar entre 0 y 10.", n+1)
				s.render(w, r, "rutina.html", d, http.StatusBadRequest)
				return
			}
			ri := int(v)
			rirInt = &ri
		}
		if utf8.RuneCountInString(note) > maxSetNote {
			d.Err = fmt.Sprintf("Fila %d: la nota es muy larga (máximo %d caracteres).", n+1, maxSetNote)
			s.render(w, r, "rutina.html", d, http.StatusBadRequest)
			return
		}
		if kg == nil && repsInt == nil {
			d.Err = fmt.Sprintf("Fila %d: cargá peso o reps (una nota sola no es una serie).", n+1)
			s.render(w, r, "rutina.html", d, http.StatusBadRequest)
			return
		}
		perEx[row.exID] = append(perEx[row.exID], store.SetInput{
			ExerciseID: row.exID, WeightKg: kg, Reps: repsInt, RIR: rirInt, Note: note,
		})
	}

	// Orden: ejercicios en el orden del plan, series 1..n dentro de cada uno.
	sets := make([]store.SetInput, 0, len(rows))
	for _, e := range d.Sheet.Exercises {
		for n, st := range perEx[e.ID] {
			st.SetNumber = n + 1
			sets = append(sets, st)
		}
	}

	if _, err := s.st.SaveWorkout(r.Context(), day, dayKey, notes, duration, sets); err != nil {
		slog.Error("webui: guardar hoja", "err", err, "day", dayKey)
		d.Err = "No pude guardar la hoja del día."
		s.render(w, r, "rutina.html", d, http.StatusInternalServerError)
		return
	}
	redirectWith(w, r, "/rutina", map[string]string{"date": day, "day": dayKey, "ok": "hoja"})
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