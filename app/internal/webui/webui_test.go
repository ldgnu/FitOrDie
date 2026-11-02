package webui

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ldgnu/fitordie/internal/store"
	"github.com/ldgnu/fitordie/templates"
)

// Los 13 ejercicios del Mesociclo 1 (Prensa de piernas se repite en A y C):
// 12 nombres únicos, uno por video.
var routineNames = []string{
	"Prensa de piernas",
	"Press pecho máquina o mancuernas",
	"Remo máquina o mancuernas",
	"Elevaciones laterales",
	"Curl bíceps",
	"Curl femoral máquina",
	"Press militar sentado",
	"Jalón al pecho",
	"Extensión tríceps en polea",
	"Press inclinado con mancuernas",
	"Remo unilateral con mancuerna",
	"Remo en polea baja sentado",
}

var youTubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// TestRoutineVideoIDs cuida lo que más se rompe al copiar y pegar videos:
// un ID mal escrito (o un nombre con el acento cambiado) deja el ejercicio sin
// video y nadie se entera hasta que estás en el gimnasio.
func TestRoutineVideoIDs(t *testing.T) {
	seen := make(map[string]string, len(routineVideoIDs))
	for _, name := range routineNames {
		id := routineVideoID(name)
		if id == "" {
			t.Errorf("%q no tiene video curado", name)
			continue
		}
		if !youTubeID.MatchString(id) {
			t.Errorf("%q tiene un ID que no parece de YouTube: %q", name, id)
		}
		if prev, dup := seen[id]; dup {
			t.Errorf("%q y %q comparten el video %s", prev, name, id)
		}
		seen[id] = name
	}
	if got, want := len(routineVideoIDs), len(routineNames); got != want {
		t.Errorf("el mapa tiene %d entradas, la rutina tiene %d nombres únicos", got, want)
	}
	if id := routineVideoID("Ejercicio que no existe"); id != "" {
		t.Errorf("un ejercicio desconocido debe devolver vacío, devolvió %q", id)
	}
}

// TestRutinaVideoFallback verifica las dos ramas del template: el botón que
// abre el modal cuando hay ID y el link de búsqueda cuando no lo hay.
func TestRutinaVideoFallback(t *testing.T) {
	tmpl, err := template.New("rutina.html").Funcs(templateFuncs()).
		ParseFS(templates.FS, "layout.html", "rutina.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	d := &viewData{
		Page:  "rutina",
		Title: "Rutina",
		Tabs: []dayTabView{
			{Key: "A", Name: routineDayNames["A"], Active: true, Planned: 6},
			{Key: "B", Name: routineDayNames["B"], Planned: 11},
		},
		Sheet: &daySheetView{
			Key: "A", Name: routineDayNames["A"], Summary: "2 ejercicios · 6 series planificadas",
			Planned: 6,
			Exercises: []exerciseSheetView{
				{
					Exercise: store.Exercise{ID: 1, Name: "Prensa de piernas", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
						YouTubeQuery: "prensa de piernas técnica correcta"},
					Rows: []setRowView{{ExerciseID: 1, ExerciseName: "Prensa de piernas", SetNumber: 1, Weight: "80", Reps: "10"}},
				},
				{
					Exercise: store.Exercise{ID: 2, Name: "Ejercicio sin video", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 90,
						YouTubeQuery: "ejercicio sin video"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", d); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `data-video="`+routineVideoID("Prensa de piernas")+`"`) {
		t.Error("el ejercicio con video debe traer el botón con data-video")
	}
	if !strings.Contains(out, `data-title="Prensa de piernas"`) {
		t.Error("el botón debe llevar el nombre del ejercicio como título")
	}
	// html/template escapa el "+" del query como &#43;: inofensivo, el browser
	// lo decodifica. Alcanza con validar la URL y el texto del link.
	if !strings.Contains(out, "https://www.youtube.com/results?search_query=ejercicio") {
		t.Error("el ejercicio sin video debe caer al link de búsqueda")
	}
	if !strings.Contains(out, ">Buscar video</a>") {
		t.Error("el fallback debe decir 'Buscar video'")
	}
	if strings.Count(out, "data-video=") != 1 {
		t.Errorf("esperaba un solo botón de video, hay %d", strings.Count(out, "data-video="))
	}
	if !strings.Contains(out, `id="video-frame"`) {
		t.Error("falta el modal de video")
	}
}

// TestProgresoPesosYReps cuida la agregación de la sección "Pesos y reps":
// las series por día tal cual se cargaron y, por semana, la mejor serie con el
// delta contra la semana calendario anterior.
func TestProgresoPesosYReps(t *testing.T) {
	loc := time.FixedZone("ART", -3*60*60)
	kg := func(v float64) *float64 { return &v }
	num := func(v int) *int { return &v }

	set := func(ex int64, name string, n int, w float64, r int, rir int) store.WorkoutSetDetail {
		st := store.WorkoutSetDetail{
			ExerciseID: ex, ExerciseName: name, SetNumber: n,
			WeightKg: kg(w), Reps: num(r), RIR: num(rir),
		}
		if w > 0 {
			st.VolumeKg = w * float64(r)
		}
		return st
	}

	// Más nuevo primero, igual que WorkoutsRange.
	rows := []store.WorkoutDetail{
		{ID: 3, Day: "2026-10-26", RoutineDay: "A", VolumeKg: 251, Sets: []store.WorkoutSetDetail{
			set(1, "Press", 1, 10, 8, 2),
			set(1, "Press", 2, 8, 12, 3),
			set(2, "Remo", 1, 5, 15, 2),
			set(3, "Prensa", 1, 0, 12, 4),
		}},
		{ID: 2, Day: "2026-10-19", RoutineDay: "A", VolumeKg: 180, Sets: []store.WorkoutSetDetail{
			set(1, "Press", 1, 8, 15, 3),
			set(2, "Remo", 1, 5, 12, 3),
		}},
		{ID: 1, Day: "2026-10-12", RoutineDay: "A", VolumeKg: 50, Sets: []store.WorkoutSetDetail{
			set(2, "Remo", 1, 5, 10, 4),
		}},
	}

	days := buildProgressDays(rows)
	if len(days) != 3 {
		t.Fatalf("esperaba 3 días, hay %d", len(days))
	}
	d := days[0]
	if d.Date != "26/10" || d.Weekday != "lun" || d.RoutineDay != "A" {
		t.Errorf("día mal formado: %+v", d)
	}
	if d.SetCount != 4 || d.Volume != 251 {
		t.Errorf("día 26/10: %d series / %v kg, esperaba 4 / 251", d.SetCount, d.Volume)
	}
	if len(d.Exercises) != 3 {
		t.Fatalf("esperaba 3 ejercicios, hay %d", len(d.Exercises))
	}
	// Peso 0 = serie sin carga: se lee como reps, no como "0 kg × 12".
	if got := d.Exercises[2].Sets[0].Text; got != "12 reps" {
		t.Errorf("serie sin peso: %q, esperaba %q", got, "12 reps")
	}
	if got := d.Exercises[0].Sets[0].RIR; got != "RIR 2" {
		t.Errorf("RIR: %q, esperaba %q", got, "RIR 2")
	}

	weeks := buildExerciseWeeks(rows, loc)
	byName := make(map[string]exerciseProgress, len(weeks))
	for _, ep := range weeks {
		byName[ep.Name] = ep
	}

	press := byName["Press"]
	if len(press.Weeks) != 2 {
		t.Fatalf("Press: esperaba 2 semanas, hay %d", len(press.Weeks))
	}
	if w := press.Weeks[0]; w.Best != "10 kg × 8" || w.Sets != 2 || w.Volume != 176 {
		t.Errorf("Press semana actual: %+v", w)
	}
	if w := press.Weeks[0]; w.Delta != "+2 kg" || w.DeltaKind != "ok" {
		t.Errorf("Press delta esperaba +2 kg (ok), dio %q (%q)", w.Delta, w.DeltaKind)
	}
	// La semana anterior no tiene Press: sin delta, no inventamos tendencia.
	if w := press.Weeks[1]; w.Delta != "" {
		t.Errorf("Press sin semana previa no debe tener delta, dio %q", w.Delta)
	}

	remo := byName["Remo"]
	if len(remo.Weeks) != 3 {
		t.Fatalf("Remo: esperaba 3 semanas, hay %d", len(remo.Weeks))
	}
	if w := remo.Weeks[0]; w.Delta != "+3 reps" || w.DeltaKind != "ok" {
		t.Errorf("Remo delta esperaba +3 reps (ok), dio %q (%q)", w.Delta, w.DeltaKind)
	}

	if prensa := byName["Prensa"]; len(prensa.Weeks) != 1 || prensa.Weeks[0].Best != "12 reps" {
		t.Errorf("Prensa: %+v", prensa)
	}

	// Sin filas no se arma nada: la vista cae al empty state.
	if got := buildProgressDays(nil); len(got) != 0 {
		t.Errorf("sin datos esperaba 0 días, dio %d", len(got))
	}
	if got := buildExerciseWeeks(nil, loc); len(got) != 0 {
		t.Errorf("sin datos esperaba 0 ejercicios, dio %d", len(got))
	}

	// El template tiene que ejecutar en las dos ramas (con y sin series).
	tmpl, err := template.New("progreso.html").Funcs(templateFuncs()).
		ParseFS(templates.FS, "layout.html", "progreso.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	dv := &viewData{
		Page: "progreso", Title: "Progreso",
		HasProgress: true, ProgressDays: days,
		HasExerciseWeeks: true, ExerciseWeeks: weeks,
		ProgressSpan: "21/09 – 01/11", ProgressWeekCount: progressWeekCount,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", dv); err != nil {
		t.Fatalf("render con datos: %v", err)
	}
	html := buf.String()
	// html/template escapa el "+" como &#43; en el texto, así que el delta se
	// valida en el modelo (arriba) y acá alcanza con el badge de mejora.
	for _, want := range []string{"Pesos y reps", "Por día", "Por semana", "10 kg × 8", "12 reps", "badge badge--ok"} {
		if !strings.Contains(html, want) {
			t.Errorf("el HTML no contiene %q", want)
		}
	}

	buf.Reset()
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", &viewData{Page: "progreso", Title: "Progreso"}); err != nil {
		t.Fatalf("render vacío: %v", err)
	}
	if !strings.Contains(buf.String(), "Todavía no hay series cargadas") {
		t.Error("sin series el template debe mostrar el empty state")
	}
}
