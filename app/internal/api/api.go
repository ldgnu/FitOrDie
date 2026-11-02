// Package api expone la API REST mínima de FitOrDie en JSON.
//
// Los handlers no confían en los macros que manda el cliente: si viene food_id,
// los valores por 100 g se leen de la base y el servidor calcula la porción con
// internal/nutrition.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ldgnu/fitordie/internal/nutrition"
	"github.com/ldgnu/fitordie/internal/store"
)

// maxBody limita el tamaño de los cuerpos JSON aceptados.
const maxBody = 1 << 20 // 1 MiB

// Options configura lo opcional de la API (hoy: el LLM para estimar comidas).
// Con todo vacío la API funciona igual, solo con el parser local determinista.
type Options struct {
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
	// LLMClient permite inyectar un cliente HTTP en tests. nil = default.
	LLMClient *http.Client
}

// API agrupa los handlers REST.
type API struct {
	st  *store.Store
	loc *time.Location
	opt Options
}

// New construye la API sin LLM (parser local determinista).
func New(st *store.Store, loc *time.Location) *API {
	return NewWithOptions(st, loc, Options{})
}

// NewWithOptions construye la API con la configuración opcional.
func NewWithOptions(st *store.Store, loc *time.Location, opt Options) *API {
	if loc == nil {
		loc = time.UTC
	}
	return &API{st: st, loc: loc, opt: opt}
}

// Routes registra las rutas (Go 1.22 pattern routing).
func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("GET /api/profile", a.getProfile)
	// POST se acepta como alias de PATCH: hay clientes que no mandan PATCH.
	mux.HandleFunc("PATCH /api/profile", a.patchProfile)
	mux.HandleFunc("POST /api/profile", a.patchProfile)
	mux.HandleFunc("GET /api/day", a.getDay)
	mux.HandleFunc("POST /api/log", a.postLog)
	mux.HandleFunc("DELETE /api/log/{id}", a.deleteLog)
	mux.HandleFunc("GET /api/weight", a.getWeights)
	mux.HandleFunc("POST /api/weight", a.postWeight)
	mux.HandleFunc("DELETE /api/weight/{day}", a.deleteWeight)
	mux.HandleFunc("GET /api/routine", a.getRoutine)
	mux.HandleFunc("GET /api/workouts", a.getWorkouts)
	mux.HandleFunc("POST /api/workout", a.postWorkout)
	mux.HandleFunc("POST /api/estimate", a.postEstimate)
	mux.HandleFunc("GET /api/foods", a.searchFoods)
	mux.HandleFunc("GET /api/plan", a.getPlan)
}

// ---------------------------------------------------------------- payloads

type entryJSON struct {
	ID       int64            `json:"id"`
	Day      string           `json:"day"`
	Meal     string           `json:"meal"`
	FoodID   *int64           `json:"food_id,omitempty"`
	FoodName string           `json:"food_name"`
	Grams    float64          `json:"grams"`
	Macros   nutrition.Macros `json:"macros"`
	Note     string           `json:"note,omitempty"`
}

type mealJSON struct {
	Key     string           `json:"key"`
	Label   string           `json:"label"`
	Time    string           `json:"time"`
	Entries []entryJSON      `json:"entries"`
	Totals  nutrition.Macros `json:"totals"`
}

type profileJSON struct {
	Name            string  `json:"name"`
	Age             int     `json:"age"`
	HeightCm        float64 `json:"height_cm"`
	WeightKg        float64 `json:"weight_kg"`
	TargetWeightMin float64 `json:"target_weight_min"`
	TargetWeightMax float64 `json:"target_weight_max"`
	KcalMin         float64 `json:"kcal_min"`
	KcalMax         float64 `json:"kcal_max"`
	ProteinMin      float64 `json:"protein_min"`
	ProteinMax      float64 `json:"protein_max"`
	CarbTarget      float64 `json:"carb_target"`
	FatTarget       float64 `json:"fat_target"`
}

// toProfileJSON aplana el perfil del store al JSON de la API.
func toProfileJSON(p store.Profile) profileJSON {
	return profileJSON{
		Name: p.Name, Age: p.Age, HeightCm: p.HeightCm, WeightKg: p.WeightKg,
		TargetWeightMin: p.TargetWeightMin, TargetWeightMax: p.TargetWeightMax,
		KcalMin: p.KcalMin, KcalMax: p.KcalMax,
		ProteinMin: p.ProteinMin, ProteinMax: p.ProteinMax,
		CarbTarget: p.CarbTarget, FatTarget: p.FatTarget,
	}
}

type dayJSON struct {
	Date          string            `json:"date"`
	Meals         []mealJSON        `json:"meals"`
	Totals        nutrition.Macros  `json:"totals"`
	Targets       nutrition.Targets `json:"targets"`
	KcalPct       float64           `json:"kcal_pct"`
	ProteinPct    float64           `json:"protein_pct"`
	KcalStatus    nutrition.Status  `json:"kcal_status"`
	ProteinState  nutrition.Status  `json:"protein_status"`
	RemainingKcal float64           `json:"remaining_kcal"`
	Profile       profileJSON       `json:"profile"`
}

type logRequest struct {
	Date     string            `json:"date"`
	Meal     string            `json:"meal"`
	FoodID   *int64            `json:"food_id"`
	FoodName string            `json:"food_name"`
	Grams    float64           `json:"grams"`
	Note     string            `json:"note"`
	Per100   *nutrition.Per100 `json:"per100"`
}

type weightRequest struct {
	Date string  `json:"date"`
	Kg   float64 `json:"kg"`
	Note string  `json:"note"`
}

type profileRequest struct {
	HeightCm        *float64 `json:"height_cm"`
	TargetWeightMin *float64 `json:"target_weight_min"`
	TargetWeightMax *float64 `json:"target_weight_max"`
	KcalMin         *float64 `json:"kcal_min"`
	KcalMax         *float64 `json:"kcal_max"`
	ProteinMin      *float64 `json:"protein_min"`
	ProteinMax      *float64 `json:"protein_max"`
}

// empty informa si el patch no trae ningún campo (no hay nada para aplicar).
func (p profileRequest) empty() bool {
	return p.HeightCm == nil && p.TargetWeightMin == nil && p.TargetWeightMax == nil &&
		p.KcalMin == nil && p.KcalMax == nil && p.ProteinMin == nil && p.ProteinMax == nil
}

type workoutSetJSON struct {
	ExerciseID   int64    `json:"exercise_id"`
	ExerciseName string   `json:"exercise_name"`
	SetNumber    int      `json:"set_number"`
	WeightKg     *float64 `json:"weight_kg"`
	Reps         *int     `json:"reps"`
	RIR          *int     `json:"rir"`
	VolumeKg     float64  `json:"volume_kg"`
}

type exerciseVolumeJSON struct {
	ExerciseID   int64   `json:"exercise_id"`
	ExerciseName string  `json:"exercise_name"`
	Sets         int     `json:"sets"`
	Reps         int     `json:"reps"`
	VolumeKg     float64 `json:"volume_kg"`
	TopWeightKg  float64 `json:"top_weight_kg"`
}

type workoutJSON struct {
	ID          int64                `json:"id"`
	Date        string               `json:"date"`
	RoutineDay  string               `json:"routine_day"`
	DurationMin int                  `json:"duration_min"`
	Notes       string               `json:"notes"`
	Sets        []workoutSetJSON     `json:"sets"`
	ByExercise  []exerciseVolumeJSON `json:"by_exercise"`
	VolumeKg    float64              `json:"volume_kg"`
	TotalReps   int                  `json:"total_reps"`
}

type setJSON struct {
	ExerciseID int64    `json:"exercise_id"`
	SetNumber  int      `json:"set_number"`
	WeightKg   *float64 `json:"weight_kg,omitempty"`
	Reps       *int     `json:"reps,omitempty"`
	RIR        *int     `json:"rir,omitempty"`
}

type workoutRequest struct {
	Date        string    `json:"date"`
	RoutineDay  string    `json:"routine_day"`
	Notes       string    `json:"notes"`
	DurationMin int       `json:"duration_min"`
	Sets        []setJSON `json:"sets"`
}

type exerciseJSON struct {
	ID           int64  `json:"id"`
	Day          string `json:"day"`
	Ord          int    `json:"ord"`
	Name         string `json:"name"`
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"`
	RIR          string `json:"rir"`
	RestSec      int    `json:"rest_sec"`
	Notes        string `json:"notes,omitempty"`
	YouTubeQuery string `json:"youtube_query"`
}

// ---------------------------------------------------------------- handlers

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	n, err := a.st.CountFoods(r.Context())
	if err != nil {
		a.fail(w, http.StatusInternalServerError, "base no disponible")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "foods": n})
}

func (a *API) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.Profile(r.Context())
	if err != nil {
		a.failStore(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileJSON(p))
}

// patchProfile edita el perfil por patch parcial (PATCH o POST).
func (a *API) patchProfile(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.empty() {
		a.fail(w, http.StatusBadRequest,
			"nada que actualizar: mandá target_weight_min/max, height_cm, kcal_min/max o protein_min/max")
		return
	}

	p, err := a.st.UpdateProfile(r.Context(), store.ProfilePatch{
		HeightCm:        req.HeightCm,
		TargetWeightMin: req.TargetWeightMin,
		TargetWeightMax: req.TargetWeightMax,
		KcalMin:         req.KcalMin,
		KcalMax:         req.KcalMax,
		ProteinMin:      req.ProteinMin,
		ProteinMax:      req.ProteinMax,
	})
	if err != nil {
		if errors.Is(err, store.ErrInvalidProfile) {
			a.fail(w, http.StatusBadRequest,
				strings.TrimPrefix(err.Error(), store.ErrInvalidProfile.Error()+": "))
			return
		}
		a.failStore(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": toProfileJSON(p)})
}

func (a *API) getDay(w http.ResponseWriter, r *http.Request) {
	day, ok := a.dayParam(w, r)
	if !ok {
		return
	}
	out, err := a.buildDay(r, day)
	if err != nil {
		a.failStore(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// buildDay arma la vista JSON del día (comidas, totales, objetivos, estado).
func (a *API) buildDay(r *http.Request, day string) (dayJSON, error) {
	ctx := r.Context()

	p, err := a.st.Profile(ctx)
	if err != nil {
		return dayJSON{}, err
	}
	entries, err := a.st.DayEntries(ctx, day)
	if err != nil {
		return dayJSON{}, err
	}

	byMeal := make(map[string][]entryJSON, len(store.Meals))
	for _, e := range entries {
		byMeal[e.Meal] = append(byMeal[e.Meal], entryJSON{
			ID: e.ID, Day: e.Day, Meal: e.Meal, FoodID: e.FoodID,
			FoodName: e.FoodName, Grams: e.Grams,
			Macros: e.Macros.Rounded(), Note: e.Note,
		})
	}

	meals := make([]mealJSON, 0, len(store.Meals))
	all := make([]nutrition.Macros, 0, len(entries))
	for _, m := range store.Meals {
		es := byMeal[m.Slug]
		ms := make([]nutrition.Macros, 0, len(es))
		for _, e := range entries {
			if e.Meal == m.Slug {
				ms = append(ms, e.Macros)
			}
		}
		if es == nil {
			es = []entryJSON{}
		}
		meals = append(meals, mealJSON{
			Key: m.Slug, Label: m.Label, Time: m.Time,
			Entries: es, Totals: nutrition.Sum(ms...).Rounded(),
		})
		all = append(all, ms...)
	}

	totals := nutrition.Sum(all...)
	targets := p.Targets()

	return dayJSON{
		Date:          day,
		Meals:         meals,
		Totals:        totals.Rounded(),
		Targets:       targets,
		KcalPct:       round1(nutrition.Pct(totals.Kcal, targets.KcalMin)),
		ProteinPct:    round1(nutrition.Pct(totals.Protein, targets.ProteinMin)),
		KcalStatus:    targets.KcalStatus(totals.Kcal),
		ProteinState:  targets.ProteinStatus(totals.Protein),
		RemainingKcal: round1(targets.Remaining(totals.Kcal)),
		Profile:       toProfileJSON(p),
	}, nil
}

func (a *API) postLog(w http.ResponseWriter, r *http.Request) {
	var req logRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	day, err := a.resolveDay(req.Date)
	if err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}

	// Toda la validación (comida, gramos, food_id vs. carga manual) vive en el
	// store: la web carga por el mismo camino, así no hay dos reglas distintas.
	saved, err := a.st.AddEntry(r.Context(), store.EntryInput{
		Day:      day,
		Meal:     req.Meal,
		FoodID:   req.FoodID,
		FoodName: req.FoodName,
		Grams:    req.Grams,
		Per100:   req.Per100,
		Note:     req.Note,
	})
	if err != nil {
		if errors.Is(err, store.ErrInvalidInput) {
			a.fail(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), store.ErrInvalidInput.Error()+": "))
			return
		}
		a.failStore(w, err)
		return
	}

	out, err := a.buildDay(r, day)
	if err != nil {
		a.failStore(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"entry": entryJSON{
			ID: saved.ID, Day: saved.Day, Meal: saved.Meal, FoodID: saved.FoodID,
			FoodName: saved.FoodName, Grams: saved.Grams,
			Macros: saved.Macros.Rounded(), Note: saved.Note,
		},
		"day": out,
	})
}

func (a *API) deleteLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		a.fail(w, http.StatusBadRequest, "id inválido")
		return
	}
	if err := a.st.DeleteEntry(r.Context(), id); err != nil {
		a.failStore(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (a *API) getWeights(w http.ResponseWriter, r *http.Request) {
	limit := 60
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	from, ok := a.rangeParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := a.rangeParam(w, r, "to")
	if !ok {
		return
	}

	rows, err := a.st.WeightsRange(r.Context(), from, to, limit)
	if err != nil {
		a.failStore(w, err)
		return
	}
	if rows == nil {
		rows = []store.WeightRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"weights": rows,
		"from":    from,
		"to":      to,
		"count":   len(rows),
	})
}

func (a *API) postWeight(w http.ResponseWriter, r *http.Request) {
	var req weightRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	day, err := a.resolveDay(req.Date)
	if err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Kg < 20 || req.Kg > 400 {
		a.fail(w, http.StatusBadRequest, "peso fuera de rango (20-400 kg)")
		return
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 200 {
		a.fail(w, http.StatusBadRequest, "nota demasiado larga (máx 200 caracteres)")
		return
	}

	// El delta se calcula contra la pesa anterior a esa fecha, no contra el
	// último registro: cargar un histórico tiene que compararse con su vecino.
	prev, _, err := a.st.WeightNeighbors(r.Context(), day)
	if err != nil {
		a.failStore(w, err)
		return
	}

	if err := a.st.SaveWeight(r.Context(), day, req.Kg, note); err != nil {
		a.failStore(w, err)
		return
	}

	var delta *float64
	if prev != nil {
		d := round1(req.Kg - prev.Kg)
		delta = &d
	}

	// El perfil solo se actualiza si esta pesa es la más reciente.
	profileUpdated := false
	if latest, err := a.st.Weights(r.Context(), 1); err == nil && len(latest) == 1 {
		profileUpdated = latest[0].Day == day
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"date": day, "kg": req.Kg, "note": note, "delta_kg": delta,
		"previous_kg":     prevKg(prev),
		"profile_updated": profileUpdated,
	})
}

// prevKg devuelve el kg de una pesa vecina o nil.
func prevKg(w *store.WeightRow) *float64 {
	if w == nil {
		return nil
	}
	kg := w.Kg
	return &kg
}

// deleteWeight borra la pesa de un día (DELETE /api/weight/{day}).
func (a *API) deleteWeight(w http.ResponseWriter, r *http.Request) {
	day, err := store.ParseDay(r.PathValue("day"))
	if err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.st.DeleteWeight(r.Context(), day); err != nil {
		if errors.Is(err, store.ErrInvalidInput) {
			a.fail(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), store.ErrInvalidInput.Error()+": "))
			return
		}
		a.failStore(w, err)
		return
	}
	// El perfil puede haber cambiado: se devuelve el peso vigente.
	var current *float64
	if latest, err := a.st.Weights(r.Context(), 1); err == nil && len(latest) == 1 {
		kg := latest[0].Kg
		current = &kg
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": day, "current_kg": current})
}

// getWorkouts devuelve los entrenamientos del rango pedido con series, nombre
// de ejercicio, volumen por ejercicio y volumen total.
func (a *API) getWorkouts(w http.ResponseWriter, r *http.Request) {
	from, ok := a.rangeParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := a.rangeParam(w, r, "to")
	if !ok {
		return
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	rows, err := a.st.WorkoutsRange(r.Context(), from, to, limit)
	if err != nil {
		a.failStore(w, err)
		return
	}

	out := make([]workoutJSON, 0, len(rows))
	totalVolume := 0.0
	for _, wo := range rows {
		sets := make([]workoutSetJSON, 0, len(wo.Sets))
		byEx := make(map[int64]*exerciseVolumeJSON, 4)
		order := make([]int64, 0, 4)
		totalReps := 0

		for _, st := range wo.Sets {
			sets = append(sets, workoutSetJSON{
				ExerciseID: st.ExerciseID, ExerciseName: st.ExerciseName,
				SetNumber: st.SetNumber, WeightKg: st.WeightKg, Reps: st.Reps,
				RIR: st.RIR, VolumeKg: st.VolumeKg,
			})
			agg, ok := byEx[st.ExerciseID]
			if !ok {
				agg = &exerciseVolumeJSON{
					ExerciseID: st.ExerciseID, ExerciseName: st.ExerciseName,
				}
				byEx[st.ExerciseID] = agg
				order = append(order, st.ExerciseID)
			}
			agg.Sets++
			agg.VolumeKg = round1(agg.VolumeKg + st.VolumeKg)
			if st.Reps != nil {
				agg.Reps += *st.Reps
				totalReps += *st.Reps
			}
			if st.WeightKg != nil && *st.WeightKg > agg.TopWeightKg {
				agg.TopWeightKg = *st.WeightKg
			}
		}

		perEx := make([]exerciseVolumeJSON, 0, len(order))
		for _, id := range order {
			perEx = append(perEx, *byEx[id])
		}

		out = append(out, workoutJSON{
			ID: wo.ID, Date: wo.Day, RoutineDay: wo.RoutineDay,
			DurationMin: wo.DurationMin, Notes: wo.Notes,
			Sets: sets, ByExercise: perEx,
			VolumeKg: round1(wo.VolumeKg), TotalReps: totalReps,
		})
		totalVolume = round1(totalVolume + wo.VolumeKg)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workouts":   out,
		"from":       from,
		"to":         to,
		"count":      len(out),
		"volume_kg":  totalVolume,
		"total_sets": totalSets(out),
	})
}

func totalSets(ws []workoutJSON) int {
	n := 0
	for _, w := range ws {
		n += len(w.Sets)
	}
	return n
}

func (a *API) getRoutine(w http.ResponseWriter, r *http.Request) {
	ex, err := a.st.Routine(r.Context())
	if err != nil {
		a.failStore(w, err)
		return
	}
	days := []map[string]any{}
	order := []string{"A", "B", "C"}
	for _, d := range order {
		list := []exerciseJSON{}
		for _, e := range ex {
			if e.Day != d {
				continue
			}
			list = append(list, exerciseJSON{
				ID: e.ID, Day: e.Day, Ord: e.Ord, Name: e.Name, Sets: e.Sets,
				Reps: e.Reps, RIR: e.RIR, RestSec: e.RestSec, Notes: e.Notes,
				YouTubeQuery: e.YouTubeQuery,
			})
		}
		if len(list) == 0 {
			continue
		}
		days = append(days, map[string]any{"day": "Día " + d, "key": d, "exercises": list})
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days})
}

func (a *API) postWorkout(w http.ResponseWriter, r *http.Request) {
	var req workoutRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	day, err := a.resolveDay(req.Date)
	if err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validRoutineDay(req.RoutineDay) {
		a.fail(w, http.StatusBadRequest, "routine_day debe ser A, B o C")
		return
	}
	if len(req.Sets) > 100 {
		a.fail(w, http.StatusBadRequest, "demasiadas series (máx 100)")
		return
	}

	// El ejercicio tiene que existir y pertenecer al día de rutina declarado.
	ex, err := a.st.Routine(r.Context())
	if err != nil {
		a.failStore(w, err)
		return
	}
	belongs := make(map[int64]string, len(ex))
	for _, e := range ex {
		belongs[e.ID] = e.Day
	}

	sets := make([]store.SetInput, 0, len(req.Sets))
	for i, s := range req.Sets {
		exDay, ok := belongs[s.ExerciseID]
		if !ok {
			a.fail(w, http.StatusBadRequest, fmt.Sprintf("serie %d: exercise_id %d no existe", i+1, s.ExerciseID))
			return
		}
		if exDay != req.RoutineDay {
			a.fail(w, http.StatusBadRequest, fmt.Sprintf("serie %d: el ejercicio es del día %s, no %s", i+1, exDay, req.RoutineDay))
			return
		}
		if s.SetNumber < 1 || s.SetNumber > 30 {
			a.fail(w, http.StatusBadRequest, fmt.Sprintf("serie %d: set_number fuera de rango (1-30)", i+1))
			return
		}
		if s.Reps != nil && (*s.Reps < 0 || *s.Reps > 200) {
			a.fail(w, http.StatusBadRequest, fmt.Sprintf("serie %d: reps fuera de rango (0-200)", i+1))
			return
		}
		if s.WeightKg != nil && (*s.WeightKg < 0 || *s.WeightKg > 1000) {
			a.fail(w, http.StatusBadRequest, fmt.Sprintf("serie %d: peso fuera de rango (0-1000 kg)", i+1))
			return
		}
		if s.RIR != nil && (*s.RIR < 0 || *s.RIR > 10) {
			a.fail(w, http.StatusBadRequest, fmt.Sprintf("serie %d: RIR fuera de rango (0-10)", i+1))
			return
		}
		sets = append(sets, store.SetInput{
			ExerciseID: s.ExerciseID, SetNumber: s.SetNumber,
			WeightKg: s.WeightKg, Reps: s.Reps, RIR: s.RIR,
		})
	}

	wo, err := a.st.SaveWorkout(r.Context(), day, req.RoutineDay,
		strings.TrimSpace(req.Notes), req.DurationMin, sets)
	if err != nil {
		a.failStore(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"workout": map[string]any{
			"id": wo.ID, "date": wo.Day, "routine_day": wo.RoutineDay,
			"notes": wo.Notes, "duration_min": wo.DurationMin, "sets": len(wo.Sets),
		},
	})
}

func (a *API) searchFoods(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	rows, err := a.st.SearchFoods(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		a.failStore(w, err)
		return
	}
	type foodJSON struct {
		ID     int64            `json:"id"`
		Name   string           `json:"name"`
		Per100 nutrition.Per100 `json:"per100"`
	}
	out := make([]foodJSON, 0, len(rows))
	for _, f := range rows {
		out = append(out, foodJSON{ID: f.ID, Name: f.Name, Per100: f.Per100})
	}
	writeJSON(w, http.StatusOK, map[string]any{"foods": out})
}

func (a *API) getPlan(w http.ResponseWriter, r *http.Request) {
	items, err := a.st.MealPlan(r.Context())
	if err != nil {
		a.failStore(w, err)
		return
	}
	notes, err := a.st.PlanNotes(r.Context())
	if err != nil {
		a.failStore(w, err)
		return
	}
	if items == nil {
		items = []store.PlanItem{}
	}
	if notes == nil {
		notes = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": items, "notes": notes})
}

// ---------------------------------------------------------------- helpers

func (a *API) dayParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	day, err := a.resolveDay(r.URL.Query().Get("date"))
	if err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return day, true
}

func (a *API) resolveDay(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return store.Today(a.loc), nil
	}
	return store.ParseDay(raw)
}

// rangeParam lee y valida un parámetro de fecha opcional (from/to).
// Vacío devuelve "" (sin límite) y es válido.
func (a *API) rangeParam(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return "", true
	}
	day, err := store.ParseDay(raw)
	if err != nil {
		a.fail(w, http.StatusBadRequest, name+": "+err.Error())
		return "", false
	}
	return day, true
}

func (a *API) failStore(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		a.fail(w, http.StatusNotFound, "no encontrado")
		return
	}
	slog.Error("error de store", "err", err)
	a.fail(w, http.StatusInternalServerError, "error interno")
}

func (a *API) fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("escribir JSON", "err", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
		return false
	}
	return true
}

func validRoutineDay(d string) bool {
	switch strings.ToUpper(strings.TrimSpace(d)) {
	case "A", "B", "C":
		return true
	}
	return false
}

func mealSlugs() string {
	parts := make([]string, 0, len(store.Meals))
	for _, m := range store.Meals {
		parts = append(parts, m.Slug)
	}
	return strings.Join(parts, ", ")
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
