// Package webui sirve el panel de FitOrDie.
//
// Reglas de esta capa:
//   - La lógica de negocio (macros, targets, persistencia, validación) vive en
//     internal/nutrition, internal/estimate y internal/store.
//   - Las vistas son GET server-rendered. Las cargas (comidas, peso, series)
//     entran por POST /hoy/*, /progreso/* y /rutina/* — formularios HTML con
//     sesión + CSRF y patrón PRG (redirect después de escribir).
//   - JavaScript solo para el modal de video de /rutina: el resto funciona con
//     HTML y un único CSS. Así la app anda igual en un celular viejo.
package webui

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ldgnu/fitordie/internal/estimate"
	"github.com/ldgnu/fitordie/internal/nutrition"
	"github.com/ldgnu/fitordie/internal/store"
	"github.com/ldgnu/fitordie/static"
	"github.com/ldgnu/fitordie/templates"
)

// pages son las tres únicas secciones del panel (más el layout compartido).
var pages = []string{"hoy.html", "rutina.html", "progreso.html"}

// routineDayNames describe cada día del mesociclo (presentación, no negocio).
var routineDayNames = map[string]string{
	"A": "Empuje + piernas",
	"B": "Femoral + hombro + tirón",
	"C": "Piernas + espalda",
}

// routineVideoIDs mapea el nombre exacto del ejercicio (tal cual está en
// internal/store/seed.go) al video de YouTube curado, en español y con
// técnica clara. Si un ejercicio no aparece acá, el template cae al link de
// búsqueda de siempre: mejor mandar a buscar que mostrar un video equivocado.
//
// Cada ID se verificó a mano: existe (oembed 200) y acepta reproducción
// embebida en un iframe (sin error 101/150 del player).
var routineVideoIDs = map[string]string{
	"Prensa de piernas":                "KdT2g0iSdG0",
	"Press pecho máquina o mancuernas": "ADpFuBjvSvA",
	"Remo máquina o mancuernas":        "rGeBwjJC8nU",
	"Elevaciones laterales":            "aVa9ce3SlSA",
	"Curl bíceps":                      "wE1Ng4nRqEU",
	"Curl femoral máquina":             "CBCPBnMzsMI",
	"Press militar sentado":            "o5M9RZ-vWrc",
	"Jalón al pecho":                   "YhUPIm4scdA",
	"Extensión tríceps en polea":       "Fv7v6IAHc0I",
	"Press inclinado con mancuernas":   "o_jrAnrSkUc",
	"Remo unilateral con mancuerna":    "eyOJAKZ3Jvw",
	"Remo en polea baja sentado":       "ZGqDOyGyX20",
}

// assetVer es el hash corto de los assets embebidos (CSS + JS), calculado en
// el paquete static. Va como ?v= en las URLs de los assets para que un deploy
// nuevo no quede tapado por la caché de Cloudflare.
var assetVer = static.Version

// routineVideoID devuelve el ID de YouTube del ejercicio, o "" si todavía no
// hay video curado para ese nombre.
func routineVideoID(name string) string {
	return routineVideoIDs[name]
}

// CSRF es el guardián de formularios (internal/auth lo implementa). Se declara
// como interfaz para que el panel no dependa de auth y los tests usen un stub.
type CSRF interface {
	// CSRFToken devuelve el token del formulario, emitiendo la cookie si falta.
	CSRFToken(w http.ResponseWriter, r *http.Request) string
	// ValidCSRFToken valida el token que vino en el POST.
	ValidCSRFToken(r *http.Request) bool
}

// Option configura dependencias del panel.
type Option func(*Server)

// WithCSRF conecta el guard de sesión/CSRF. Sin esto los POST responden 403:
// la web falla cerrado.
func WithCSRF(c CSRF) Option { return func(s *Server) { s.csrf = c } }

// WithEstimator conecta el analizador de comidas (con LLM si está configurado).
func WithEstimator(e *estimate.Estimator) Option { return func(s *Server) { s.est = e } }

// Server renderiza las páginas.
type Server struct {
	st    *store.Store
	loc   *time.Location
	est   *estimate.Estimator
	csrf  CSRF
	pages map[string]*template.Template
}

// New parsea los templates (uno por página, compartiendo el layout).
func New(st *store.Store, loc *time.Location, opts ...Option) (*Server, error) {
	if loc == nil {
		loc = time.UTC
	}
	funcs := templateFuncs()

	s := &Server{st: st, loc: loc, pages: make(map[string]*template.Template, len(pages))}
	for _, opt := range opts {
		opt(s)
	}
	// Sin estimador explícito igual funciona: parser local determinista.
	if s.est == nil && st != nil {
		s.est = estimate.New(st, estimate.Options{})
	}
	for _, p := range pages {
		t, err := template.New(p).Funcs(funcs).ParseFS(templates.FS, "layout.html", p)
		if err != nil {
			return nil, err
		}
		s.pages[p] = t
	}
	return s, nil
}

// templateFuncs son los helpers que usan los templates. Vive aparte de New
// para que los tests puedan renderizar una página sin abrir la base.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"num": fmtNum,
		"dg":  fmtGrams,
		"kg":  fmtKg,
		// pct se usa para anchos de barra: siempre 0-100.
		"pct": func(v float64) string {
			return strconv.FormatFloat(math.Min(math.Max(v, 0), 100), 'f', 0, 64)
		},
		"dmy": func(day string) string {
			if t, err := time.Parse("2006-01-02", day); err == nil {
				return t.Format("02/01")
			}
			return day
		},
		"rest":     restLabel,
		"urlquery": url.QueryEscape,
		"videoID":  routineVideoID,
	}
}

// Routes registra las vistas, los formularios POST y los assets embebidos.
// Los POST exigen sesión (guard.Protect) y CSRF (se valida en cada handler).
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", s.hoy)
	mux.HandleFunc("GET /rutina", s.rutina)
	mux.HandleFunc("GET /progreso", s.progreso)

	// Hoy: diario.
	mux.HandleFunc("POST /hoy/analizar", s.hoyAnalyze)
	mux.HandleFunc("POST /hoy/confirmar", s.hoyConfirm)
	mux.HandleFunc("POST /hoy/manual", s.hoyManual)
	mux.HandleFunc("POST /hoy/alimento", s.hoyFood)
	mux.HandleFunc("POST /hoy/borrar", s.hoyDelete)

	// Progreso: peso.
	mux.HandleFunc("POST /progreso/peso", s.progresoPeso)
	mux.HandleFunc("POST /progreso/peso/borrar", s.progresoPesoBorrar)

	// Rutina: series y nota del día (una hoja por día, se guarda completa).
	mux.HandleFunc("POST /rutina/hoja", s.rutinaHoja)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.FS)))
	mux.HandleFunc("GET /manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		b, err := static.FS.ReadFile("manifest.webmanifest")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write(b)
	})
}

// ---------------------------------------------------------------- vistas

type mealView struct {
	Key     string
	Label   string
	Time    string
	Entries []store.Entry
	Totals  nutrition.Macros
}

// dayTabView es una pestaña de la rutina: un día del mesociclo.
type dayTabView struct {
	Key    string
	Name   string
	Active bool
	// Planned son las series planificadas del día (del plan, no del registro).
	Planned int
	// Sets son las series efectivamente cargadas ese día.
	Sets int
}

// setRowView es una serie editable dentro de la hoja del día. SetNumber 0 es
// la fila vacía que espera la próxima serie.
type setRowView struct {
	ExerciseID   int64
	ExerciseName string
	SetNumber    int
	Weight       string
	Reps         string
	RIR          string
	Note         string
}

// exerciseSheetView es un ejercicio del plan con sus series del día.
type exerciseSheetView struct {
	store.Exercise
	Rows    []setRowView
	Last    *lastSetView
	Volume  float64
	HasSets bool
}

// daySheetView es la hoja del día elegido: todos los ejercicios del plan con
// los inputs de sus series, más la nota y la duración de la sesión.
type daySheetView struct {
	Key         string
	Name        string
	Summary     string
	Planned     int
	Exercises   []exerciseSheetView
	SetCount    int
	Volume      float64
	Notes       string
	DurationMin int
	Exists      bool
}

// lastSetView es la última serie registrada de un ejercicio en la semana:
// "80 kg × 10" + día (dd/mm).
type lastSetView struct {
	Text string
	Day  string
}

// histExercise resume un ejercicio dentro de un entrenamiento.
type histExercise struct {
	Name   string
	Sets   int
	Volume float64
}

// historyWorkout es un entrenamiento de la semana en curso.
type historyWorkout struct {
	Date        string
	RoutineDay  string
	DayName     string
	DurationMin int
	Notes       string
	Exercises   []histExercise
	Volume      float64
	TotalReps   int
	SetCount    int
}

// weightWeek agrupa las pesas de una semana (lunes a domingo).
type weightWeek struct {
	Label   string
	StartKg float64
	EndKg   float64
	Delta   float64
	Rows    []store.WeightRow
}

// progressSet es una serie tal como se lee en el historial: "8 kg × 15", más
// el RIR cuando el registro lo tiene.
type progressSet struct {
	Text string
	RIR  string
}

// progressExercise es un ejercicio con sus series dentro de un entrenamiento.
type progressExercise struct {
	Name string
	Sets []progressSet
}

// progressDay es un entrenamiento del historial reciente (uno por día).
type progressDay struct {
	Date       string
	Weekday    string
	RoutineDay string
	DayName    string
	SetCount   int
	Volume     float64
	Exercises  []progressExercise
}

// progressWeek es la mejor serie de un ejercicio en una semana, con el volumen
// acumulado y el delta contra la semana anterior (vacío si no hay con qué
// comparar).
type progressWeek struct {
	Label     string
	Best      string
	BestKg    float64
	BestReps  int
	Sets      int
	Volume    float64
	Delta     string
	DeltaKind string // "ok", "warn" o "" (sin cambio)
}

// exerciseProgress agrupa las semanas de un ejercicio, de la más nueva a la
// más vieja.
type exerciseProgress struct {
	Name  string
	Weeks []progressWeek
}

type viewData struct {
	Page  string
	Title string
	// AssetVer es el hash de CSS/JS; va en la URL de los assets (?v=).
	AssetVer string
	// CSRF es el token que llevan todos los formularios de la página.
	CSRF string
	// Msg es el aviso de éxito (viene del redirect PRG) y Err el de error.
	Msg string
	Err string

	Today    string
	Date     string
	Prev     string
	Next     string
	IsToday  bool
	DayLabel string

	Profile     store.Profile
	NowKg       float64
	Targets     nutrition.Targets
	Totals      nutrition.Macros
	KcalBarPct  float64
	KcalOver    bool
	KcalOverBy  float64
	Remaining   float64
	ProteinPct  float64
	CarbPct     float64
	FatPct      float64
	ProteinOver bool

	Meals            []mealView
	MealsWithEntries []mealView

	// Peso / tendencia (compartido por Hoy y Progreso).
	Weights     []store.WeightRow
	HasWeights  bool
	WeightWeeks []weightWeek
	HasSpark    bool
	SparkLine   string
	SparkArea   string
	SparkFirst  string
	SparkLast   string
	SparkMin    string
	SparkMax    string
	SparkDate0  string
	SparkDateN  string
	HasWeek     bool
	WeekDelta   float64
	ToGoal      float64
	GoalPct     float64
	Streak      int
	LoggedCount int

	// Rutina: pestañas por día y la hoja del día elegido.
	Tabs         []dayTabView
	Sheet        *daySheetView
	RoutineNotes []string
	LastByEx     map[int64]*lastSetView
	WeekLabel    string
	History      []historyWorkout
	HasHistory   bool
	WeekSessions int
	WeekSets     int
	WeekReps     int
	WeekVolume   float64

	// Pesos y reps (Progreso): historial por día y mejor serie por semana.
	ProgressDays      []progressDay
	HasProgress       bool
	ExerciseWeeks     []exerciseProgress
	HasExerciseWeeks  bool
	ProgressSpan      string
	ProgressWeekCount int

	// Formularios (cargas desde la web).
	Analyze    *analyzeFormView
	Manual     *manualFormView
	Preview    *previewView
	WeightForm *weightFormView
	Foods      []store.Food
	Query      string
}

// analyzeFormView conserva el texto y la comida del analizador.
type analyzeFormView struct {
	Meal string
	Text string
}

// base carga los datos comunes a las tres vistas. La fecha se resuelve en este
// orden: la explícita (POST o ?date=), la del query y, si no hay, hoy.
func (s *Server) base(r *http.Request, date string) (*viewData, error) {
	p, err := s.st.Profile(r.Context())
	if err != nil {
		return nil, err
	}
	today := store.Today(s.loc)

	d := &viewData{
		AssetVer: assetVer,
		Today:    today,
		Date:     today,
		Profile:  p,
		Targets:  p.Targets(),
		LastByEx: map[int64]*lastSetView{},
	}
	raw := date
	if raw == "" {
		raw = r.URL.Query().Get("date")
	}
	if day, err := store.ParseDay(raw); err == nil {
		d.Date = day
	}
	if t, err := time.Parse("2006-01-02", d.Date); err == nil {
		d.Prev = t.AddDate(0, 0, -1).Format("2006-01-02")
		d.Next = t.AddDate(0, 0, 1).Format("2006-01-02")
	}
	d.IsToday = d.Date == today
	d.DayLabel = longDate(d.Date)
	return d, nil
}

// fillDay agrega entradas, totales y barras del día (solo lectura).
func (s *Server) fillDay(r *http.Request, d *viewData) error {
	entries, err := s.st.DayEntries(r.Context(), d.Date)
	if err != nil {
		return err
	}
	all := make([]nutrition.Macros, 0, len(entries))
	d.Meals = make([]mealView, 0, len(store.Meals))
	for _, m := range store.Meals {
		mv := mealView{Key: m.Slug, Label: m.Label, Time: m.Time, Entries: []store.Entry{}}
		ms := make([]nutrition.Macros, 0, 4)
		for _, e := range entries {
			if e.Meal != m.Slug {
				continue
			}
			mv.Entries = append(mv.Entries, e)
			ms = append(ms, e.Macros)
			all = append(all, e.Macros)
		}
		mv.Totals = nutrition.Sum(ms...).Rounded()
		d.Meals = append(d.Meals, mv)
		if len(mv.Entries) > 0 {
			d.MealsWithEntries = append(d.MealsWithEntries, mv)
		}
	}

	d.Totals = nutrition.Sum(all...).Rounded()
	t := d.Targets
	d.KcalBarPct = nutrition.Pct(d.Totals.Kcal, t.KcalMin)
	d.ProteinPct = nutrition.Pct(d.Totals.Protein, t.ProteinMax)
	d.CarbPct = nutrition.Pct(d.Totals.Carbs, t.CarbTarget)
	d.FatPct = nutrition.Pct(d.Totals.Fat, t.FatTarget)
	d.Remaining = t.Remaining(d.Totals.Kcal)
	d.ProteinOver = t.ProteinMax > 0 && d.Totals.Protein > t.ProteinMax
	if t.KcalMax > 0 && d.Totals.Kcal > t.KcalMax {
		d.KcalOver = true
		d.KcalOverBy = d.Totals.Kcal - t.KcalMax
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, d *viewData, status int) {
	t, ok := s.pages[page]
	if !ok {
		http.Error(w, "template no encontrado", http.StatusInternalServerError)
		return
	}
	// Token de formularios: se emite (o reusa) la cookie CSRF en cada render.
	if s.csrf != nil {
		d.CSRF = s.csrf.CSRFToken(w, r)
	}
	if d.Msg == "" {
		d.Msg = flash(r.URL.Query())
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// El HTML cambia con cada carga: que Safari no muestre una versión vieja.
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout.html", d); err != nil {
		slog.Error("render template", "page", page, "err", err)
	}
}

// ---------------------------------------------------------------- vistas

// hoyView arma la vista del diario para una fecha (vacía = query u hoy).
func (s *Server) hoyView(r *http.Request, date string) (*viewData, error) {
	d, err := s.base(r, date)
	if err != nil {
		return nil, err
	}
	d.Page, d.Title = "hoy", "Hoy"
	if err := s.fillDay(r, d); err != nil {
		return nil, err
	}

	// Racha y peso: si fallan, la vista sigue mostrando el diario.
	if days, err := s.st.LoggedDays(r.Context(), time.Now().In(s.loc).AddDate(0, 0, -90).Format("2006-01-02")); err == nil {
		d.Streak = streak(days, d.Today)
	}
	rows, err := s.st.Weights(r.Context(), 30)
	if err != nil {
		return nil, err
	}
	s.fillWeight(d, rows)

	// Búsqueda de alimentos para la carga rápida (?q=).
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" && len([]rune(q)) <= maxQueryLen {
		foods, err := s.st.SearchFoods(r.Context(), q, 12)
		if err != nil {
			return nil, err
		}
		d.Query = q
		d.Foods = foods
	}
	return d, nil
}

// rutinaView arma la vista de rutina: pestañas por día, la hoja del día
// elegido, la semana en curso y las reglas del plan.
func (s *Server) rutinaView(r *http.Request, date, day string) (*viewData, error) {
	d, err := s.base(r, date)
	if err != nil {
		return nil, err
	}
	d.Page, d.Title = "rutina", "Rutina"
	if err := s.fillRutina(r, d, day); err != nil {
		return nil, err
	}
	return d, nil
}

// fillRutina completa la vista de rutina: pestañas, hoja del día (ejercicios
// del plan + series cargadas), semana en curso y reglas.
//
// El día activo se resuelve así: el pedido explícito (?day= o el POST), si no
// el del entrenamiento ya cargado para esa fecha y, si no hay nada, el que
// toca por día de la semana (lun A, mar B, mié C, jue A…).
func (s *Server) fillRutina(r *http.Request, d *viewData, day string) error {
	ex, err := s.st.Routine(r.Context())
	if err != nil {
		return err
	}
	byDay := make(map[string][]store.Exercise, 3)
	planned := make(map[string]int, 3)
	for _, e := range ex {
		byDay[e.Day] = append(byDay[e.Day], e)
		planned[e.Day] += e.Sets
	}

	active := s.resolveRoutineDay(r, d, day, byDay)
	if active == "" {
		// Rutina sin sembrar: la vista muestra el empty state.
		d.Sheet = &daySheetView{}
		return nil
	}

	notes, err := s.st.PlanNotes(r.Context())
	if err != nil {
		return err
	}
	d.RoutineNotes = displayNotes(notes)

	// Semana en curso: mismo rango y misma agregación que GET /api/workouts.
	week := s.weekOf(d.Today)
	d.WeekLabel = week.StartDay[8:10] + "/" + week.StartDay[5:7] + " – " + week.EndDay[8:10] + "/" + week.EndDay[5:7]

	rows, err := s.st.WorkoutsRange(r.Context(), week.StartDay, week.EndDay, 200)
	if err != nil {
		return err
	}
	d.History = buildHistory(rows)
	d.HasHistory = len(d.History) > 0
	for _, h := range d.History {
		d.WeekSessions++
		d.WeekSets += h.SetCount
		d.WeekReps += h.TotalReps
		d.WeekVolume += h.Volume
	}
	d.WeekVolume = round1f(d.WeekVolume)

	// "Último registro" de cada ejercicio: la mejor serie de la última vez que
	// se hizo ANTES (o el mismo día) de la fecha que se está editando. Una
	// ventana de 8 semanas alcanza para no mostrar un recuerdo de hace meses.
	lastFrom := d.Date
	if t, err := time.Parse("2006-01-02", d.Date); err == nil {
		lastFrom = t.AddDate(0, 0, -56).Format("2006-01-02")
	}
	lastRows, err := s.st.WorkoutsRange(r.Context(), lastFrom, d.Date, 200)
	if err != nil {
		return err
	}
	d.LastByEx = lastSets(lastRows)

	// Series del día elegido, agrupadas por ejercicio.
	wo, err := s.st.WorkoutByDay(r.Context(), d.Date)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	setsByEx := make(map[int64][]store.SetInput, len(wo.Sets))
	for _, st := range wo.Sets {
		setsByEx[st.ExerciseID] = append(setsByEx[st.ExerciseID], st)
	}

	sheet := &daySheetView{
		Key: active, Name: routineDayNames[active],
		Notes: wo.Notes, DurationMin: wo.DurationMin,
		Exists: err == nil, Planned: planned[active],
	}
	for _, e := range byDay[active] {
		ev := exerciseSheetView{Exercise: e, Last: d.LastByEx[e.ID]}
		for _, st := range setsByEx[e.ID] {
			ev.Rows = append(ev.Rows, setRowView{
				ExerciseID: e.ID, ExerciseName: e.Name, SetNumber: st.SetNumber,
				Weight: inputNumPtr(st.WeightKg), Reps: inputIntPtr(st.Reps),
				RIR: inputIntPtr(st.RIR), Note: st.Note,
			})
			if st.WeightKg != nil && st.Reps != nil {
				ev.Volume = round1f(ev.Volume + *st.WeightKg*float64(*st.Reps))
			}
			sheet.SetCount++
		}
		ev.HasSets = len(ev.Rows) > 0
		// Fila vacía para agregar una serie: con JS se clona, sin JS alcanza
		// para cargar una serie más y guardar.
		ev.Rows = append(ev.Rows, setRowView{ExerciseID: e.ID, ExerciseName: e.Name})
		sheet.Volume = round1f(sheet.Volume + ev.Volume)
		sheet.Exercises = append(sheet.Exercises, ev)
	}
	sheet.Summary = fmt.Sprintf("%d ejercicios · %d series planificadas", len(sheet.Exercises), sheet.Planned)
	d.Sheet = sheet

	// Pestañas: solo los días que existen en el plan.
	for _, key := range []string{"A", "B", "C"} {
		if len(byDay[key]) == 0 {
			continue
		}
		t := dayTabView{Key: key, Name: routineDayNames[key], Planned: planned[key], Active: key == active}
		if key == active {
			t.Sets = sheet.SetCount
		}
		d.Tabs = append(d.Tabs, t)
	}
	return nil
}

// resolveRoutineDay decide qué día mostrar. Prioridad: lo pedido explícito,
// lo ya cargado para esa fecha, el día de la semana y, al final, el primero
// que exista en el plan.
func (s *Server) resolveRoutineDay(r *http.Request, d *viewData, day string, byDay map[string][]store.Exercise) string {
	valid := func(k string) bool {
		return (k == "A" || k == "B" || k == "C") && len(byDay[k]) > 0
	}
	if valid(day) {
		return day
	}
	if q := strings.TrimSpace(r.URL.Query().Get("day")); valid(q) {
		return q
	}
	if wo, err := s.st.WorkoutByDay(r.Context(), d.Date); err == nil && valid(wo.RoutineDay) {
		return wo.RoutineDay
	}
	// Rotación A/B/C por día de la semana (lunes = A).
	if t, err := time.ParseInLocation("2006-01-02", d.Date, s.loc); err == nil {
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		if k := string(rune('A' + (wd-1)%3)); valid(k) {
			return k
		}
	}
	for _, k := range []string{"A", "B", "C"} {
		if valid(k) {
			return k
		}
	}
	return ""
}

// progresoView arma la vista de progreso (peso, tendencia y series).
func (s *Server) progresoView(r *http.Request, date string) (*viewData, error) {
	d, err := s.base(r, date)
	if err != nil {
		return nil, err
	}
	d.Page, d.Title = "progreso", "Progreso"

	weights, err := s.st.Weights(r.Context(), 120)
	if err != nil {
		return nil, err
	}
	s.fillWeight(d, weights)

	since := time.Now().In(s.loc).AddDate(0, 0, -90).Format("2006-01-02")
	days, err := s.st.LoggedDays(r.Context(), since)
	if err != nil {
		return nil, err
	}
	d.LoggedCount = len(days)
	d.Streak = streak(days, d.Today)

	// Pesos y reps: una sola query para toda la ventana (8 semanas). El
	// historial por día se queda con los últimos entrenamientos y la vista
	// semanal con las últimas progressWeekCount semanas.
	week := s.weekOf(d.Today)
	from := week.Start.AddDate(0, 0, -7*(progressWeekCount+1))
	rows, err := s.st.WorkoutsRange(r.Context(), from.Format("2006-01-02"), week.EndDay, 200)
	if err != nil {
		return nil, err
	}
	d.ProgressDays = buildProgressDays(rows)
	d.HasProgress = len(d.ProgressDays) > 0
	d.ExerciseWeeks = buildExerciseWeeks(rows, s.loc)
	d.HasExerciseWeeks = len(d.ExerciseWeeks) > 0
	d.ProgressWeekCount = progressWeekCount
	spanStart := week.Start.AddDate(0, 0, -7*(progressWeekCount-1))
	d.ProgressSpan = spanStart.Format("02/01") + " – " + week.End.Format("02/01")
	return d, nil
}

// ---------------------------------------------------------------- handlers

func (s *Server) hoy(w http.ResponseWriter, r *http.Request) {
	d, err := s.hoyView(r, "")
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "hoy.html", d, http.StatusOK)
}

func (s *Server) rutina(w http.ResponseWriter, r *http.Request) {
	d, err := s.rutinaView(r, "", "")
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "rutina.html", d, http.StatusOK)
}

func (s *Server) progreso(w http.ResponseWriter, r *http.Request) {
	d, err := s.progresoView(r, "")
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "progreso.html", d, http.StatusOK)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	slog.Error("webui", "err", err)
	http.Error(w, "error interno", http.StatusInternalServerError)
}

// ---------------------------------------------------------------- peso

// fillWeight completa peso actual, semanas, tendencia y avance al objetivo.
func (s *Server) fillWeight(d *viewData, rows []store.WeightRow) {
	d.Weights = rows
	d.HasWeights = len(rows) > 0
	d.NowKg = d.Profile.WeightKg
	if d.HasWeights {
		d.NowKg = rows[0].Kg
		d.WeightWeeks = groupWeeks(rows, s.loc)
		d.WeekDelta, d.HasWeek = weekDelta(rows, d.Today)
		if len(rows) > 1 {
			start := rows[len(rows)-1].Kg
			goal := d.Profile.TargetWeightMax
			if start > goal {
				d.GoalPct = math.Min(100, math.Max(0, (start-d.NowKg)/(start-goal)*100))
			}
			d.ToGoal = math.Max(0, d.NowKg-goal)
		}
	}
	s.fillSpark(d, rows)
}

// weekDelta compara el último peso con el registro de hace >= 7 días.
func weekDelta(rows []store.WeightRow, today string) (float64, bool) {
	if len(rows) == 0 {
		return 0, false
	}
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0, false
	}
	for _, w := range rows[1:] {
		day, err := time.Parse("2006-01-02", w.Day)
		if err != nil {
			continue
		}
		if t.Sub(day) >= 7*24*time.Hour {
			return round1f(rows[0].Kg - w.Kg), true
		}
	}
	return 0, false
}

// fillSpark genera la ruta del sparkline SVG (más reciente primero). Sin al
// menos 2 puntos no hay gráfico: preferimos no dibujar antes que inventar.
func (s *Server) fillSpark(d *viewData, rows []store.WeightRow) {
	if len(rows) < 2 {
		return
	}
	const (
		w, h   = 320.0, 140.0
		padL   = 34.0
		padT   = 24.0
		padR   = 10.0
		padB   = 28.0
		maxPts = 30
	)
	pts := rows
	if len(pts) > maxPts {
		pts = pts[:maxPts]
	}
	chrono := make([]store.WeightRow, len(pts))
	for i, r := range pts {
		chrono[len(pts)-1-i] = r
	}
	min, max := chrono[0].Kg, chrono[0].Kg
	for _, r := range chrono {
		if r.Kg < min {
			min = r.Kg
		}
		if r.Kg > max {
			max = r.Kg
		}
	}
	span := max - min
	if span < 0.2 {
		span = 0.2
	}
	x := func(i int) float64 {
		if len(chrono) == 1 {
			return padL
		}
		return padL + float64(i)*(w-padL-padR)/float64(len(chrono)-1)
	}
	y := func(kg float64) float64 {
		return padT + (1-(kg-min)/span)*(h-padT-padB)
	}
	var line strings.Builder
	for i, r := range chrono {
		if i > 0 {
			line.WriteString(" L ")
		} else {
			line.WriteString("M ")
		}
		fmt.Fprintf(&line, "%.1f %.1f", x(i), y(r.Kg))
	}
	area := line.String() + fmt.Sprintf(" L %.1f %.1f L %.1f %.1f Z", x(len(chrono)-1), h-padB, x(0), h-padB)

	d.HasSpark = true
	d.SparkLine = line.String()
	d.SparkArea = area
	d.SparkFirst = fmtKg(chrono[0].Kg)
	d.SparkLast = fmtKg(chrono[len(chrono)-1].Kg)
	d.SparkMin = fmtKg(min)
	d.SparkMax = fmtKg(max)
	if t, err := time.Parse("2006-01-02", chrono[0].Day); err == nil {
		d.SparkDate0 = t.Format("02/01")
	}
	if t, err := time.Parse("2006-01-02", chrono[len(chrono)-1].Day); err == nil {
		d.SparkDateN = t.Format("02/01")
	}
}

// groupWeeks agrupa las pesas por semana. rows viene del más nuevo al más
// viejo, así que el primer registro de cada semana es el cierre y el último
// el inicio.
func groupWeeks(rows []store.WeightRow, loc *time.Location) []weightWeek {
	out := make([]weightWeek, 0, len(rows))
	idx := make(map[string]int, len(rows))
	for _, r := range rows {
		t, err := time.ParseInLocation("2006-01-02", r.Day, loc)
		if err != nil {
			continue
		}
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		start := t.AddDate(0, 0, -(wd - 1))
		key := start.Format("2006-01-02")
		p, ok := idx[key]
		if !ok {
			end := start.AddDate(0, 0, 6)
			out = append(out, weightWeek{
				Label:   start.Format("02/01") + " – " + end.Format("02/01"),
				StartKg: r.Kg, EndKg: r.Kg,
			})
			p = len(out) - 1
			idx[key] = p
		}
		w := &out[p]
		w.Rows = append(w.Rows, r)
		w.EndKg = r.Kg
		w.StartKg = w.Rows[0].Kg
	}
	for i := range out {
		out[i].Delta = round1f(out[i].EndKg - out[i].StartKg)
	}
	return out
}

// ---------------------------------------------------------------- rutina

// buildHistory arma la vista del historial: un bloque por entrenamiento con
// los ejercicios agrupados y el volumen de cada uno.
func buildHistory(rows []store.WorkoutDetail) []historyWorkout {
	out := make([]historyWorkout, 0, len(rows))
	for _, wo := range rows {
		h := historyWorkout{
			Date: dmyShort(wo.Day), RoutineDay: wo.RoutineDay,
			DayName: routineDayNames[wo.RoutineDay], DurationMin: wo.DurationMin,
			Notes: wo.Notes, Volume: round1f(wo.VolumeKg),
		}
		idx := make(map[int64]int, len(wo.Sets))
		for _, st := range wo.Sets {
			p, ok := idx[st.ExerciseID]
			if !ok {
				h.Exercises = append(h.Exercises, histExercise{Name: st.ExerciseName})
				p = len(h.Exercises) - 1
				idx[st.ExerciseID] = p
			}
			ex := &h.Exercises[p]
			ex.Sets++
			h.SetCount++
			if st.Reps != nil {
				h.TotalReps += *st.Reps
			}
			ex.Volume = round1f(ex.Volume + st.VolumeKg)
		}
		out = append(out, h)
	}
	return out
}

// lastSets elige, por ejercicio, la mejor serie del entrenamiento más
// reciente de la semana (rows viene del más nuevo al más viejo).
func lastSets(rows []store.WorkoutDetail) map[int64]*lastSetView {
	out := make(map[int64]*lastSetView, len(rows)*3)
	for _, wo := range rows {
		best := make(map[int64]store.WorkoutSetDetail, len(wo.Sets))
		order := make([]int64, 0, len(wo.Sets))
		for _, st := range wo.Sets {
			if _, done := out[st.ExerciseID]; done {
				continue
			}
			cur, seen := best[st.ExerciseID]
			if !seen {
				best[st.ExerciseID] = st
				order = append(order, st.ExerciseID)
				continue
			}
			if betterSet(st, cur) {
				best[st.ExerciseID] = st
			}
		}
		for _, id := range order {
			st := best[id]
			// fmtSetLoad: un peso 0 es "12 reps", no "0 kg × 12" (serie sin carga).
			text := fmtSetLoad(st.WeightKg, st.Reps)
			if text == "" {
				continue
			}
			out[id] = &lastSetView{Text: text, Day: dmyShort(wo.Day)}
		}
	}
	return out
}

func betterSet(a, b store.WorkoutSetDetail) bool {
	aw, bw := 0.0, 0.0
	if a.WeightKg != nil {
		aw = *a.WeightKg
	}
	if b.WeightKg != nil {
		bw = *b.WeightKg
	}
	if aw != bw {
		return aw > bw
	}
	ar, br := 0, 0
	if a.Reps != nil {
		ar = *a.Reps
	}
	if b.Reps != nil {
		br = *b.Reps
	}
	return ar > br
}

// ---------------------------------------------------------------- pesos y reps

const (
	// progressDayCount es cuántos entrenamientos se listan uno por uno.
	progressDayCount = 8
	// progressWeekCount es cuántas semanas entran en la vista semanal.
	progressWeekCount = 6
)

// buildProgressDays arma el historial día por día: los últimos
// progressDayCount entrenamientos con las series de cada ejercicio, en el
// orden de la rutina. rows viene del más nuevo al más viejo.
func buildProgressDays(rows []store.WorkoutDetail) []progressDay {
	out := make([]progressDay, 0, progressDayCount)
	for _, wo := range rows {
		if len(out) == progressDayCount {
			break
		}
		day := progressDay{
			Date:       dmyShort(wo.Day),
			Weekday:    weekdayShort(wo.Day),
			RoutineDay: wo.RoutineDay,
			DayName:    routineDayNames[wo.RoutineDay],
			Volume:     round1f(wo.VolumeKg),
		}
		idx := make(map[int64]int, len(wo.Sets))
		for _, st := range wo.Sets {
			text := fmtSetLoad(st.WeightKg, st.Reps)
			if text == "" {
				continue
			}
			p, ok := idx[st.ExerciseID]
			if !ok {
				day.Exercises = append(day.Exercises, progressExercise{Name: st.ExerciseName})
				p = len(day.Exercises) - 1
				idx[st.ExerciseID] = p
			}
			ps := progressSet{Text: text}
			if st.RIR != nil {
				ps.RIR = "RIR " + strconv.Itoa(*st.RIR)
			}
			day.Exercises[p].Sets = append(day.Exercises[p].Sets, ps)
			day.SetCount++
		}
		if day.SetCount == 0 {
			continue // entrenamiento sin series: no hay nada que mostrar
		}
		out = append(out, day)
	}
	return out
}

// exWeekAgg acumula un ejercicio dentro de una semana.
type exWeekAgg struct {
	best   store.WorkoutSetDetail
	sets   int
	volume float64
}

// buildExerciseWeeks devuelve, por ejercicio, la mejor serie y el volumen de
// cada una de las últimas progressWeekCount semanas. El delta compara con la
// semana calendario anterior: si esa semana no tiene registro del ejercicio,
// no hay delta (mejor no mostrar que inventar una tendencia vieja).
func buildExerciseWeeks(rows []store.WorkoutDetail, loc *time.Location) []exerciseProgress {
	byWeek := make(map[string]map[int64]*exWeekAgg, progressWeekCount+2)
	keys := make([]string, 0, progressWeekCount+2)
	order := make([]int64, 0, 16) // orden de aparición = orden de la rutina
	names := make(map[int64]string, 16)

	for _, wo := range rows {
		start, ok := weekStart(wo.Day, loc)
		if !ok {
			continue
		}
		key := start.Format("2006-01-02")
		week, seen := byWeek[key]
		if !seen {
			week = make(map[int64]*exWeekAgg, len(wo.Sets))
			byWeek[key] = week
			keys = append(keys, key)
		}
		for _, st := range wo.Sets {
			agg, ok := week[st.ExerciseID]
			if !ok {
				agg = &exWeekAgg{best: st}
				week[st.ExerciseID] = agg
				if _, known := names[st.ExerciseID]; !known {
					names[st.ExerciseID] = st.ExerciseName
					order = append(order, st.ExerciseID)
				}
			} else if betterSet(st, agg.best) {
				agg.best = st
			}
			agg.sets++
			agg.volume = round1f(agg.volume + st.VolumeKg)
		}
	}
	if len(keys) == 0 {
		return nil
	}

	// Las fechas ISO ordenan igual que el calendario; nos quedamos con la
	// ventana que se muestra.
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	if len(keys) > progressWeekCount {
		keys = keys[:progressWeekCount]
	}

	out := make([]exerciseProgress, 0, len(order))
	for _, id := range order {
		ep := exerciseProgress{Name: names[id]}
		for _, key := range keys {
			agg, ok := byWeek[key][id]
			if !ok {
				continue
			}
			w := progressWeek{
				Label:    weekRangeLabel(key),
				Best:     fmtSetLoad(agg.best.WeightKg, agg.best.Reps),
				BestKg:   weightOf(agg.best.WeightKg),
				BestReps: repsOf(agg.best.Reps),
				Sets:     agg.sets,
				Volume:   agg.volume,
			}
			// Se compara con la semana calendario anterior, no con la fila
			// anterior de la lista: si el ejercicio no se hizo esa semana, no
			// hay delta.
			if prev, ok := byWeek[previousWeekKey(key)][id]; ok {
				w.Delta, w.DeltaKind = setDelta(w.BestKg, w.BestReps,
					weightOf(prev.best.WeightKg), repsOf(prev.best.Reps))
			}
			ep.Weeks = append(ep.Weeks, w)
		}
		if len(ep.Weeks) > 0 {
			out = append(out, ep)
		}
	}
	return out
}

// setDelta compara la mejor serie de dos semanas. Manda el peso; con el mismo
// peso manda las reps. Sin cambios devuelve "=" (neutro, sin badge de color).
func setDelta(kg float64, reps int, prevKg float64, prevReps int) (string, string) {
	if kg != prevKg {
		d := round1f(kg - prevKg)
		text := fmtKg(d) + " kg"
		if d > 0 {
			return "+" + text, "ok"
		}
		return text, "warn"
	}
	if reps != prevReps {
		text := strconv.Itoa(reps-prevReps) + " reps"
		if reps > prevReps {
			return "+" + text, "ok"
		}
		return text, "warn"
	}
	return "=", ""
}

// weekStart normaliza un día a la fecha del lunes de su semana.
func weekStart(day string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return time.Time{}, false
	}
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1)), true
}

// weekRangeLabel pasa la fecha de un lunes a "20/10 – 26/10".
func weekRangeLabel(startKey string) string {
	t, err := time.Parse("2006-01-02", startKey)
	if err != nil {
		return startKey
	}
	return t.Format("02/01") + " – " + t.AddDate(0, 0, 6).Format("02/01")
}

// previousWeekKey devuelve el lunes de la semana anterior a startKey.
func previousWeekKey(startKey string) string {
	t, err := time.Parse("2006-01-02", startKey)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -7).Format("2006-01-02")
}

// weightOf y repsOf leen los punteros opcionales como cero.
func weightOf(w *float64) float64 {
	if w == nil {
		return 0
	}
	return *w
}

func repsOf(r *int) int {
	if r == nil {
		return 0
	}
	return *r
}

// ---------------------------------------------------------------- semanas

// weekWindow es una semana lunes-domingo, con las fechas ya formateadas.
type weekWindow struct {
	Start, End       time.Time
	StartDay, EndDay string
}

// weekOf normaliza un día a la semana (lunes) que lo contiene.
func (s *Server) weekOf(raw string) weekWindow {
	day := raw
	if _, err := store.ParseDay(day); err != nil {
		day = store.Today(s.loc)
	}
	t, err := time.ParseInLocation("2006-01-02", day, s.loc)
	if err != nil {
		t, _ = time.ParseInLocation("2006-01-02", store.Today(s.loc), s.loc)
	}
	wd := int(t.Weekday()) // domingo = 0
	if wd == 0 {
		wd = 7
	}
	start := t.AddDate(0, 0, -(wd - 1))
	end := start.AddDate(0, 0, 6)
	return weekWindow{
		Start: start, End: end,
		StartDay: start.Format("2006-01-02"), EndDay: end.Format("2006-01-02"),
	}
}

// ---------------------------------------------------------------- helpers

// displayNotes normaliza notas viejas del plan que quedaron sembradas con texto
// desactualizado: la rutina mezcla RIR 3-4 y 2-3, así que la regla correcta es
// el rango 2-4 (sin llegar al fallo).
func displayNotes(notes []string) []string {
	out := make([]string, 0, len(notes))
	seenRIR := false
	for _, n := range notes {
		if strings.Contains(strings.ToUpper(n), "RIR") {
			if seenRIR {
				continue
			}
			seenRIR = true
			out = append(out, "RIR 2-4 según el ejercicio; nunca al fallo.")
			continue
		}
		out = append(out, n)
	}
	return out
}

// fmtNum formatea enteros con separador de miles (1.569).
func fmtNum(v float64) string {
	n := int64(math.Round(v))
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out strings.Builder
	if neg {
		out.WriteByte('-')
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(c)
	}
	return out.String()
}

// fmtGrams muestra gramos con 1 decimal y sin ceros colgados (130 · 130,5).
func fmtGrams(v float64) string {
	s := strconv.FormatFloat(math.Round(v*10)/10, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1)
}

// inputNum formatea un número para un <input type="number"> (punto decimal:
// el navegador no acepta coma como separador de miles/decimales).
func inputNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// inputNumPtr e inputIntPtr muestran vacío cuando la serie no trae el dato.
func inputNumPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return inputNum(*v)
}

func inputIntPtr(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// fmtKg muestra kilos con hasta 2 decimales, sin ceros de relleno (89,65 · 90).
func fmtKg(v float64) string {
	s := strconv.FormatFloat(math.Round(v*100)/100, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return strings.Replace(s, ".", ",", 1)
}

// longDate devuelve "jueves 22 de octubre" para el kicker.
func longDate(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	weekdays := [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
	months := [...]string{"", "enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
		"agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	return fmt.Sprintf("%s %d de %s", weekdays[int(t.Weekday())], t.Day(), months[int(t.Month())])
}

// streak cuenta días consecutivos cargados terminando hoy (o ayer si hoy aún
// no tiene registros: el día en curso no debería cortar la racha).
func streak(days []string, today string) int {
	set := make(map[string]bool, len(days))
	for _, d := range days {
		set[d] = true
	}
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	if !set[today] {
		t = t.AddDate(0, 0, -1)
	}
	n := 0
	for set[t.Format("2006-01-02")] {
		n++
		t = t.AddDate(0, 0, -1)
	}
	return n
}

// restLabel formatea el descanso planificado en lenguaje humano.
func restLabel(sec int) string {
	switch {
	case sec <= 0:
		return "—"
	case sec < 60:
		return strconv.Itoa(sec) + " s"
	case sec%60 == 0:
		return strconv.Itoa(sec/60) + " min"
	default:
		return strconv.Itoa(sec/60) + " min " + strconv.Itoa(sec%60) + " s"
	}
}

// fmtSetLoad escribe una serie del historial de entrenamientos. Un peso 0 no
// es "0 kg": fue una serie sin carga (prensa con molestia), así que se lee
// como reps sueltas.
func fmtSetLoad(weight *float64, reps *int) string {
	if weight == nil || *weight <= 0 {
		if reps == nil {
			return ""
		}
		return strconv.Itoa(*reps) + " reps"
	}
	if reps == nil {
		return fmtKg(*weight) + " kg"
	}
	return fmtKg(*weight) + " kg × " + strconv.Itoa(*reps)
}

// dmyShort pasa 2026-10-22 a 22/10.
func dmyShort(day string) string {
	if t, err := time.Parse("2006-01-02", day); err == nil {
		return t.Format("02/01")
	}
	return day
}

// weekdayShort pasa 2026-10-22 a "jue".
func weekdayShort(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return ""
	}
	return [...]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}[int(t.Weekday())]
}

func round1f(v float64) float64 {
	return math.Round(v*10) / 10
}
