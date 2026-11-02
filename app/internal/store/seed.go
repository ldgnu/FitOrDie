package store

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/ldgnu/fitordie/data"
	"github.com/ldgnu/fitordie/internal/nutrition"
)

// seedVersion identifica el set de datos semilla. Cambiarlo fuerza re-seed
// cuando se agregan filas nuevas al plan o a la rutina.
const seedVersion = "2026-10-22.1"

// Seed siembra la base. Con force=true reimporta alimentos, plan y rutina
// (no toca diario ni pesos). Sin force solo completa lo que esté vacío, así el
// arranque normal es idempotente.
func (s *Store) Seed(ctx context.Context, force bool) error {
	return s.SeedFrom(ctx, force, nil)
}

// SeedFrom es Seed con un CSV alternativo (CSV_PATH). csv nil = usa el embebido.
func (s *Store) SeedFrom(ctx context.Context, force bool, csv []byte) error {
	if err := s.seedProfile(ctx); err != nil {
		return err
	}
	if err := s.seedFoods(ctx, force, csv); err != nil {
		return err
	}
	if err := s.seedPlan(ctx, force); err != nil {
		return err
	}
	if err := s.seedRoutine(ctx, force); err != nil {
		return err
	}
	if err := s.seedWeights(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES ('seed_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, seedVersion); err != nil {
		return fmt.Errorf("guardar seed_version: %w", err)
	}
	return nil
}

// seedProfile inserta el perfil de Javi si todavía no existe.
func (s *Store) seedProfile(ctx context.Context) error {
	const q = `INSERT OR IGNORE INTO profile
		(id, name, age, height_cm, weight_kg, target_weight_min, target_weight_max,
		 kcal_min, kcal_max, protein_min, protein_max, carb_target, fat_target, notes, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		"Javi", 38, 185.0, 87.5, 84.0, 85.0,
		2150.0, 2400.0, 155.0, 170.0, 220.0, 90.0,
		"Plan Fran. Altura 1,85 m (el plan asume 1,87). Objetivo 84-85 kg. "+
			"Sal normal, agua 2.5-3 L/día, pollo y carne se pesan crudos.",
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("sembrar perfil: %w", err)
	}
	return nil
}

// seedFoods importa data/alimentos.csv a la tabla foods.
func (s *Store) seedFoods(ctx context.Context, force bool, csvData []byte) error {
	n, err := s.CountFoods(ctx)
	if err != nil {
		return err
	}
	if n > 0 && !force {
		slog.Info("foods ya cargados, se omite import", "filas", n)
		return nil
	}

	if csvData == nil {
		csvData = data.AlimentosCSV
	}
	rows, skipped, err := parseFoodsCSV(csvData)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("el CSV de alimentos no produjo filas válidas (saltadas: %d)", skipped)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tx alimentos: %w", err)
	}
	defer tx.Rollback()

	const q = `INSERT INTO foods
		(name, kcal100, protein100, fat100, carbs100, fiber100, sugar100, sodium_mg100, source, source_detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			kcal100 = excluded.kcal100, protein100 = excluded.protein100,
			fat100 = excluded.fat100, carbs100 = excluded.carbs100,
			fiber100 = excluded.fiber100, sugar100 = excluded.sugar100,
			sodium_mg100 = excluded.sodium_mg100, source = excluded.source,
			source_detail = excluded.source_detail`

	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("preparar import: %w", err)
	}
	defer stmt.Close()

	for _, f := range rows {
		if _, err := stmt.ExecContext(ctx, f.Name, f.Per100.Kcal, f.Per100.Protein,
			f.Per100.Fat, f.Per100.Carbs, f.Per100.Fiber, f.Per100.Sugar,
			f.Per100.Sodium, f.Source, f.SourceDetail); err != nil {
			return fmt.Errorf("importar %q: %w", f.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}

	slog.Info("foods importados", "filas", len(rows), "saltadas", skipped)
	return nil
}

// parseFoodsCSV parsea el CSV de alimentos.
//
// El archivo original tenía un defecto de escape: las columnas sugar100 y
// sodium_mg100 quedaron dentro de source_detail como `...",""0"",""2"`, así que
// cada fila traía 8 campos en lugar de 10. El CSV del repo ya está normalizado,
// pero se tolera igual el formato viejo (sin tocar el archivo fuente): si una
// fila llega con 8 campos, el último se parte por `","` y se recuperan los dos
// valores faltantes.
func parseFoodsCSV(raw []byte) ([]Food, int, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("parsear CSV de alimentos: %w", err)
	}
	if len(records) < 2 {
		return nil, 0, fmt.Errorf("CSV de alimentos vacío")
	}

	var (
		out     []Food
		skipped int
	)
	for i, rec := range records[1:] {
		if len(rec) == 0 || strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue
		}
		rec = normalizeFoodRecord(rec)
		if len(rec) != 10 {
			slog.Warn("fila de alimentos con columnas inesperadas", "linea", i+2, "columnas", len(rec))
			skipped++
			continue
		}

		name := strings.TrimSpace(rec[0])
		if name == "" {
			skipped++
			continue
		}
		out = append(out, Food{
			Name:         name,
			Per100:       nutritionFrom(rec),
			Source:       strings.TrimSpace(rec[6]),
			SourceDetail: strings.TrimSpace(rec[7]),
		})
	}
	return out, skipped, nil
}

// normalizeFoodRecord recupera sugar/sodium cuando vienen embebidos en el
// séptimo campo (defecto de escape del CSV legacy).
func normalizeFoodRecord(rec []string) []string {
	if len(rec) == 8 {
		parts := strings.Split(rec[7], `","`)
		if len(parts) == 3 {
			fixed := make([]string, 0, 10)
			fixed = append(fixed, rec[:7]...)
			fixed = append(fixed, strings.Trim(parts[0], `"`), parts[1], parts[2])
			return fixed
		}
	}
	// Por las dudas, limpiar comillas residuales de cada campo.
	for i := range rec {
		rec[i] = strings.TrimSpace(strings.Trim(rec[i], `"`))
	}
	return rec
}

// nutritionFrom arma el Per100 desde columnas 1..5 y 8..9 del registro.
func nutritionFrom(rec []string) nutrition.Per100 {
	return nutrition.Per100{
		Kcal:    num(rec[1]),
		Protein: num(rec[2]),
		Fat:     num(rec[3]),
		Carbs:   num(rec[4]),
		Fiber:   num(rec[5]),
		Sugar:   num(rec[8]),
		Sodium:  num(rec[9]),
	}
}

func num(s string) float64 {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		return 0
	}
	return v
}

// seedPlan carga el Plan Fran: 5 comidas con opciones A/B/C, picoteo y reglas.
func (s *Store) seedPlan(ctx context.Context, force bool) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM meal_plan`).Scan(&n); err != nil {
		return fmt.Errorf("contar plan: %w", err)
	}
	if n > 0 && !force {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tx plan: %w", err)
	}
	defer tx.Rollback()

	if force {
		if _, err := tx.ExecContext(ctx, `DELETE FROM meal_plan`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM plan_notes`); err != nil {
			return err
		}
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO meal_plan (meal, meal_time, option, description, kcal, protein, ord)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, p := range planFran {
		if _, err := stmt.ExecContext(ctx, p.Meal, p.Time, p.Option, p.Description, p.Kcal, p.Protein, i); err != nil {
			return fmt.Errorf("insertar plan %s/%s: %w", p.Meal, p.Option, err)
		}
	}

	noteStmt, err := tx.PrepareContext(ctx, `INSERT INTO plan_notes (kind, text, ord) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer noteStmt.Close()

	for i, n := range planNotes {
		if _, err := noteStmt.ExecContext(ctx, n.Kind, n.Text, i); err != nil {
			return fmt.Errorf("insertar regla: %w", err)
		}
	}
	return tx.Commit()
}

// seedRoutine carga el Mesociclo 1 (días A/B/C).
func (s *Store) seedRoutine(ctx context.Context, force bool) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routine_exercises`).Scan(&n); err != nil {
		return fmt.Errorf("contar rutina: %w", err)
	}
	if n > 0 && !force {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tx rutina: %w", err)
	}
	defer tx.Rollback()

	if force {
		// Solo se limpia la rutina si no hay entrenamientos cargados: las series
		// referencian routine_exercises y perderíamos historial.
		var workouts int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workouts`).Scan(&workouts); err != nil {
			return err
		}
		if workouts > 0 {
			slog.Warn("rutina no se re-siembra: hay entrenamientos registrados", "workouts", workouts)
			return nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM routine_exercises`); err != nil {
			return err
		}
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO routine_exercises
		 (day, ord, name, sets, reps, rir, rest_sec, notes, youtube_query)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range routineMesociclo1 {
		if _, err := stmt.ExecContext(ctx, e.Day, e.Ord, e.Name, e.Sets, e.Reps,
			e.RIR, e.RestSec, e.Notes, e.YouTubeQuery); err != nil {
			return fmt.Errorf("insertar ejercicio %s/%d: %w", e.Day, e.Ord, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("rutina sembrada", "ejercicios", len(routineMesociclo1))
	return nil
}

// ---------------------------------------------------------------- datos seed

// seedWeightRow es una pesa histórica conocida.
type seedWeightRow struct {
	Day  string
	Kg   float64
	Note string
}

// seedWeights siembra las pesas reales del arranque del plan.
//
// Es idempotente y NO pisa lo que ya está cargado (INSERT OR IGNORE): si Javi
// ya corrigió una pesa, el seed no la revierte. Tampoco toca
// `profile.weight_kg`: de eso se encarga SaveWeight con la regla de fecha
// máxima.
var seedWeightsData = []seedWeightRow{
	// Pesa previa al inicio del plan: la fecha es estimada.
	{Day: "2026-10-04", Kg: 96.0, Note: "pesa previa; fecha estimada, confirmar"},
	// Primera pesa real del plan.
	{Day: "2026-10-05", Kg: 89.65, Note: "pesa real"},
}

// seedPendingNote marca como "a confirmar" la pesa suelta del 22/10 si existe
// sin nota. No se borra ni se cambia el kg: el dato quedó cargado antes de
// tener el histórico y hay que confirmarlo, no descartarlo.
const seedPendingNote = "a confirmar"

func (s *Store) seedWeights(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tx pesos seed: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, w := range seedWeightsData {
		// Si la fila ya existe no se reescribe el kg (dato del usuario): solo se
		// completa la nota si está vacía.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO weights (day, kg, note, created_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT(day) DO NOTHING`,
			w.Day, w.Kg, w.Note, now); err != nil {
			return fmt.Errorf("sembrar peso %s: %w", w.Day, err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE weights SET note = ? WHERE day = ? AND note = ''`,
			w.Note, w.Day); err != nil {
			return fmt.Errorf("completar nota de %s: %w", w.Day, err)
		}
	}

	// force no aplica acá: el seed de pesos nunca borra ni reescribe kg, solo
	// completa la nota de la pesa pendiente.
	if _, err := tx.ExecContext(ctx,
		`UPDATE weights SET note = ? WHERE day = ? AND kg = ? AND note = ''`,
		seedPendingNote, "2026-10-22", 87.5); err != nil {
		return fmt.Errorf("marcar pesa pendiente: %w", err)
	}
	return tx.Commit()
}

type planRow struct {
	Meal        string
	Time        string
	Option      string
	Description string
	Kcal        int
	Protein     int
}

// planFran es el Plan Fran tal como está en el brief (5 comidas + picoteo).
var planFran = []planRow{
	// Desayuno 7:30
	{"desayuno", "07:30", "A", "3 huevos + 40 g queso + café sin azúcar", 375, 25},
	{"desayuno", "07:30", "B", "1 scoop whey + 300 ml leche + banana + 20 g avena", 470, 28},
	{"desayuno", "07:30", "C", "Yogur griego 200 g + 30 g avena + 20 g frutos secos", 480, 24},

	// Media mañana 11:00
	{"media_manana", "11:00", "A", "Yogur griego 170 g + 20 g frutos secos", 326, 21},
	{"media_manana", "11:00", "B", "2 huevos duros + 1 fruta", 265, 14},
	{"media_manana", "11:00", "C", "40 g queso + 1 fruta + 15 g frutos secos", 390, 17},

	// Almuerzo 13:30 — proteína + carbos + ensalada + aceite
	{"almuerzo", "13:30", "A", "Pollo 250 g + fideos 110 g + ensalada + 10 g aceite de oliva", 780, 70},
	{"almuerzo", "13:30", "B", "Carne magra 200 g + arroz 70 g + ensalada + 10 g aceite de oliva", 780, 70},
	{"almuerzo", "13:30", "C", "Atún 2 latas (o 3 huevos) + papa 300 g + ensalada + 10 g aceite de oliva", 780, 70},

	// Merienda 16:30
	{"merienda", "16:30", "A", "1 banana + 40 g queso port salut", 237, 10},
	{"merienda", "16:30", "B", "1 scoop whey + agua o leche + 1 fruta", 250, 26},
	{"merienda", "16:30", "C", "Yogur griego 170 g + 20 g frutos secos", 326, 21},

	// Cena 21:00
	{"cena", "21:00", "A", "Carne magra 200 g + verduras al horno + 10 g aceite", 500, 45},
	{"cena", "21:00", "B", "Pollo 250 g + verduras + 10 g aceite", 455, 59},
	{"cena", "21:00", "C", "Tortilla de 3 huevos + verduras", 385, 24},

	// Picoteo (opciones sueltas para cargar rápido)
	{"picoteo", "", "A", "Pochoclo casero 25 g", 95, 3},
	{"picoteo", "", "B", "Aceitunas 50 g", 80, 1},
	{"picoteo", "", "C", "Pepino o zanahoria + 30 g queso crema", 90, 3},
	{"picoteo", "", "D", "1-2 huevos duros", 140, 12},
	{"picoteo", "", "E", "Gelatina sin azúcar", 40, 2},
	{"picoteo", "", "F", "Frutos secos 20 g", 126, 4},
	{"picoteo", "", "G", "Whey en agua (1 scoop)", 110, 24},
}

type noteRow struct{ Kind, Text string }

// planNotes son las reglas del plan y de la rutina que no son comidas.
var planNotes = []noteRow{
	{"plan", "Sal normal: no es una dieta sin sal."},
	{"plan", "Agua 2.5-3 L por día."},
	{"plan", "El pollo y la carne se pesan crudos."},
	{"plan", "Si la lactosa molesta, anotarlo y pasar a una opción sin lácteos."},
	{"plan", "Si baja más de 0.5 kg por semana, sumar 200 kcal."},
	{"rutina", "RIR 2-4 según el ejercicio; nunca al fallo."},
	{"rutina", "Exhalar en el esfuerzo; levantarse y sentarse lento; sentarse entre series."},
	{"rutina", "Cardio: caminar 20-30 min martes y jueves."},
	{"rutina", "Pendiente médico: ecocardiograma."},
}

// routineMesociclo1 es el Mesociclo 1 del brief (3 días/semana).
var routineMesociclo1 = []Exercise{
	// Día A
	{Day: "A", Ord: 1, Name: "Prensa de piernas", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
		Notes: "", YouTubeQuery: "prensa de piernas técnica correcta"},
	{Day: "A", Ord: 2, Name: "Press pecho máquina o mancuernas", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
		Notes: "Bajada controlada de 2 s", YouTubeQuery: "press pecho mancuernas técnica"},
	{Day: "A", Ord: 3, Name: "Remo máquina o mancuernas", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
		Notes: "", YouTubeQuery: "remo con mancuerna técnica correcta"},
	{Day: "A", Ord: 4, Name: "Elevaciones laterales", Sets: 2, Reps: "12-15", RIR: "2-3", RestSec: 60,
		Notes: "", YouTubeQuery: "elevaciones laterales técnica"},
	{Day: "A", Ord: 5, Name: "Curl bíceps", Sets: 2, Reps: "10-12", RIR: "2-3", RestSec: 60,
		Notes: "Codos pegados al torso, bajada de 2 s", YouTubeQuery: "curl bíceps mancuernas técnica"},

	// Día B
	{Day: "B", Ord: 1, Name: "Curl femoral máquina", Sets: 3, Reps: "12-15", RIR: "3-4", RestSec: 90,
		Notes: "", YouTubeQuery: "curl femoral máquina técnica"},
	{Day: "B", Ord: 2, Name: "Press militar sentado", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
		Notes: "", YouTubeQuery: "press militar sentado mancuernas técnica"},
	{Day: "B", Ord: 3, Name: "Jalón al pecho", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 90,
		Notes: "", YouTubeQuery: "jalón al pecho técnica correcta"},
	{Day: "B", Ord: 4, Name: "Extensión tríceps en polea", Sets: 2, Reps: "10-12", RIR: "2-3", RestSec: 60,
		Notes: "", YouTubeQuery: "extensión tríceps polea técnica"},

	// Día C
	{Day: "C", Ord: 1, Name: "Prensa de piernas", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
		Notes: "", YouTubeQuery: "prensa de piernas técnica correcta"},
	{Day: "C", Ord: 2, Name: "Press inclinado con mancuernas", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 120,
		Notes: "Banco a 30°", YouTubeQuery: "press inclinado mancuernas 30 grados técnica"},
	{Day: "C", Ord: 3, Name: "Remo unilateral con mancuerna", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 90,
		Notes: "", YouTubeQuery: "remo unilateral mancuerna técnica"},
	{Day: "C", Ord: 4, Name: "Remo en polea baja sentado", Sets: 3, Reps: "10-12", RIR: "3-4", RestSec: 90,
		Notes: "", YouTubeQuery: "remo polea baja sentado técnica"},
}
