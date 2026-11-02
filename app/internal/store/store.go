// Package store es la capa de persistencia de FitOrDie sobre SQLite
// (driver pure-Go modernc.org/sqlite, sin CGO).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ldgnu/fitordie/internal/nutrition"
)

// ErrNotFound se devuelve cuando el registro pedido no existe.
var ErrNotFound = errors.New("registro no encontrado")

// timeLayout es el formato de fecha usado en todas las columnas `day`.
const timeLayout = "2006-01-02"

// Meal describe una comida del día.
type Meal struct {
	Slug  string
	Label string
	Time  string
}

// Meals es el orden canónico de las comidas del Plan Fran.
var Meals = []Meal{
	{Slug: "desayuno", Label: "Desayuno", Time: "07:30"},
	{Slug: "media_manana", Label: "Media mañana", Time: "11:00"},
	{Slug: "almuerzo", Label: "Almuerzo", Time: "13:30"},
	{Slug: "merienda", Label: "Merienda", Time: "16:30"},
	{Slug: "cena", Label: "Cena", Time: "21:00"},
	{Slug: "picoteo", Label: "Picoteo", Time: ""},
}

// ValidMeal informa si el slug corresponde a una comida conocida.
func ValidMeal(slug string) bool {
	for _, m := range Meals {
		if m.Slug == slug {
			return true
		}
	}
	return false
}

// MealLabel devuelve la etiqueta legible de un slug (o el slug si no existe).
func MealLabel(slug string) string {
	for _, m := range Meals {
		if m.Slug == slug {
			return m.Label
		}
	}
	return slug
}

// mealOrder es el índice de orden de un slug.
func mealOrder(slug string) int {
	for i, m := range Meals {
		if m.Slug == slug {
			return i
		}
	}
	return len(Meals)
}

// Profile son los datos del usuario y sus objetivos diarios.
type Profile struct {
	ID              int64
	Name            string
	Age             int
	HeightCm        float64
	WeightKg        float64
	TargetWeightMin float64
	TargetWeightMax float64
	KcalMin         float64
	KcalMax         float64
	ProteinMin      float64
	ProteinMax      float64
	CarbTarget      float64
	FatTarget       float64
	Notes           string
	UpdatedAt       time.Time
}

// Targets convierte el perfil en los objetivos que consume el motor nutricional.
func (p Profile) Targets() nutrition.Targets {
	return nutrition.Targets{
		KcalMin:    p.KcalMin,
		KcalMax:    p.KcalMax,
		ProteinMin: p.ProteinMin,
		ProteinMax: p.ProteinMax,
		CarbTarget: p.CarbTarget,
		FatTarget:  p.FatTarget,
	}
}

// Food es un alimento de la tabla `foods` con sus valores por 100 g.
type Food struct {
	ID           int64
	Name         string
	Per100       nutrition.Per100
	Source       string
	SourceDetail string
}

// Entry es una línea del diario, con los macros congelados al momento de cargar.
type Entry struct {
	ID        int64
	Day       string
	Meal      string
	FoodID    *int64
	FoodName  string
	Grams     float64
	Macros    nutrition.Macros
	Note      string
	CreatedAt time.Time
}

// NewEntry es el payload de inserción de una línea del diario.
type NewEntry struct {
	Day      string
	Meal     string
	FoodID   *int64
	FoodName string
	Grams    float64
	Macros   nutrition.Macros
	Note     string
}

// Exercise es un ejercicio de la rutina.
type Exercise struct {
	ID           int64
	Day          string
	Ord          int
	Name         string
	Sets         int
	Reps         string
	RIR          string
	RestSec      int
	Notes        string
	YouTubeQuery string
}

// SetInput es una serie cargada de un entrenamiento.
type SetInput struct {
	ExerciseID int64
	SetNumber  int
	WeightKg   *float64
	Reps       *int
	RIR        *int
	// Note es la nota libre de la serie (\"última lenta\", \"fallo técnico\").
	Note string
}

// Workout es un entrenamiento registrado.
type Workout struct {
	ID          int64
	Day         string
	RoutineDay  string
	Notes       string
	DurationMin int
	CreatedAt   time.Time
	Sets        []SetInput
}

// WeightRow es un registro de peso. La nota es libre y sirve para marcar si la
// pesa es real, estimada o está pendiente de confirmar.
type WeightRow struct {
	Day  string  `json:"day"`
	Kg   float64 `json:"kg"`
	Note string  `json:"note"`
}

// ProfilePatch son los campos editables del perfil. Un puntero nil significa
// "no tocar este campo".
type ProfilePatch struct {
	HeightCm        *float64
	TargetWeightMin *float64
	TargetWeightMax *float64
	KcalMin         *float64
	KcalMax         *float64
	ProteinMin      *float64
	ProteinMax      *float64
}

// WorkoutSetDetail es una serie con el nombre del ejercicio y su volumen.
type WorkoutSetDetail struct {
	ExerciseID   int64
	ExerciseName string
	SetNumber    int
	WeightKg     *float64
	Reps         *int
	RIR          *int
	Note         string
	VolumeKg     float64 // peso * reps; 0 cuando falta alguno de los dos
}

// WorkoutDetail es un entrenamiento con sus series y el volumen agregado.
type WorkoutDetail struct {
	ID          int64
	Day         string
	RoutineDay  string
	Notes       string
	DurationMin int
	Sets        []WorkoutSetDetail
	VolumeKg    float64
}

// PlanItem es una opción del Plan Fran.
type PlanItem struct {
	Meal        string
	Time        string
	Option      string
	Description string
	Kcal        int
	Protein     int
}

// Store envuelve la conexión SQLite.
type Store struct {
	db *sql.DB
}

// Open abre (o crea) el archivo SQLite, aplica el schema y devuelve el Store.
func Open(path string) (*Store, error) {
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite %q: %w", path, err)
	}
	// Una sola conexión: SQLite serializa las escrituras y así evitamos
	// SQLITE_BUSY; con el tráfico de un usuario sobra y baja la memoria.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close cierra la conexión.
func (s *Store) Close() error { return s.db.Close() }

// DB expone la conexión para tests y tareas de mantenimiento.
func (s *Store) DB() *sql.DB { return s.db }

// migrate ejecuta el DDL sentencia por sentencia (el engine no acepta
// múltiples statements en un Exec de database/sql de forma portable) y después
// aplica las migraciones de columnas que el DDL no puede agregar solo.
func migrate(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schema, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migración falló en %q: %w", firstLine(stmt), err)
		}
	}
	// Bases creadas antes de la tanda de notas: `weights.note` no existe en el
	// DDL viejo, así que IF NOT EXISTS no la agrega. Idempotente por diseño.
	if err := ensureColumn(ctx, db, "weights", "note", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	// Nota por serie (hoja de rutina del día): las bases viejas no la tienen.
	if err := ensureColumn(ctx, db, "workout_sets", "note", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

// ensureColumn agrega `table`.`column` si todavía no existe.
//
// Las columnas se listan primero y se cierra el rows antes del ALTER: la
// conexión es única (MaxOpenConns = 1), así que ejecutar dentro del bucle
// quedaría esperando a que se libere esa misma conexión.
func ensureColumn(ctx context.Context, db *sql.DB, table, column, decl string) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return fmt.Errorf("leer columnas de %s: %w", table, err)
	}

	var names []string
	for rows.Next() {
		var (
			cid      int
			name     string
			typ      string
			notNull  int
			defaultV any
			pk       int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("leer columna de %s: %w", table, err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("leer columnas de %s: %w", table, err)
	}

	for _, n := range names {
		if strings.EqualFold(n, column) {
			return nil
		}
	}

	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("agregar columna %s.%s: %w", table, column, err)
	}
	slog.Info("migración: columna agregada", "tabla", table, "columna", column)
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// ---------------------------------------------------------------- perfil

// Profile devuelve el perfil único (id = 1).
func (s *Store) Profile(ctx context.Context) (Profile, error) {
	const q = `SELECT id, name, age, height_cm, weight_kg, target_weight_min,
		target_weight_max, kcal_min, kcal_max, protein_min, protein_max,
		carb_target, fat_target, notes, updated_at FROM profile WHERE id = 1`

	var p Profile
	var updated string
	err := s.db.QueryRowContext(ctx, q).Scan(
		&p.ID, &p.Name, &p.Age, &p.HeightCm, &p.WeightKg, &p.TargetWeightMin,
		&p.TargetWeightMax, &p.KcalMin, &p.KcalMax, &p.ProteinMin, &p.ProteinMax,
		&p.CarbTarget, &p.FatTarget, &p.Notes, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("leer perfil: %w", err)
	}
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return p, nil
}

// UpdateWeight actualiza el peso actual del perfil.
func (s *Store) UpdateWeight(ctx context.Context, kg float64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE profile SET weight_kg = ?, updated_at = ? WHERE id = 1`,
		kg, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("actualizar peso del perfil: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrInvalidProfile envuelve los errores de validación del perfil: la API los
// traduce a 400 en lugar de 500.
var ErrInvalidProfile = errors.New("perfil inválido")

// ValidateProfile valida los campos editables del perfil ya fusionados.
// Un valor 0 en objetivos significa "sin objetivo" y no se valida.
func ValidateProfile(p Profile) error {
	if p.HeightCm < 100 || p.HeightCm > 250 {
		return fmt.Errorf("%w: height_cm fuera de rango (100-250)", ErrInvalidProfile)
	}
	if err := validateRange("target_weight", p.TargetWeightMin, p.TargetWeightMax, 20, 400); err != nil {
		return err
	}
	if err := validateRange("kcal", p.KcalMin, p.KcalMax, 500, 10000); err != nil {
		return err
	}
	if err := validateRange("protein", p.ProteinMin, p.ProteinMax, 1, 600); err != nil {
		return err
	}
	return nil
}

// validateRange valida un par mínimo/máximo. min se valida siempre que no sea
// 0; max se valida cuando viene informado (0 = sin objetivo).
func validateRange(name string, min, max, lo, hi float64) error {
	if min < 0 || max < 0 {
		return fmt.Errorf("%w: %s no puede ser negativo", ErrInvalidProfile, name)
	}
	if min > 0 && (min < lo || min > hi) {
		return fmt.Errorf("%w: %s_min fuera de rango (%v-%v)", ErrInvalidProfile, name, lo, hi)
	}
	if max > 0 && (max < lo || max > hi) {
		return fmt.Errorf("%w: %s_max fuera de rango (%v-%v)", ErrInvalidProfile, name, lo, hi)
	}
	if min > 0 && max > 0 && min > max {
		return fmt.Errorf("%w: %s_min (%v) no puede ser mayor que %s_max (%v)", ErrInvalidProfile, name, min, name, max)
	}
	return nil
}

// UpdateProfile aplica un patch parcial sobre el perfil único y devuelve el
// perfil resultante. Valida el estado fusionado (no solo el patch), así un
// cambio de mínimo se compara contra el máximo vigente.
func (s *Store) UpdateProfile(ctx context.Context, patch ProfilePatch) (Profile, error) {
	cur, err := s.Profile(ctx)
	if err != nil {
		return Profile{}, err
	}

	apply := func(dst *float64, src *float64) {
		if src != nil {
			*dst = *src
		}
	}
	apply(&cur.HeightCm, patch.HeightCm)
	apply(&cur.TargetWeightMin, patch.TargetWeightMin)
	apply(&cur.TargetWeightMax, patch.TargetWeightMax)
	apply(&cur.KcalMin, patch.KcalMin)
	apply(&cur.KcalMax, patch.KcalMax)
	apply(&cur.ProteinMin, patch.ProteinMin)
	apply(&cur.ProteinMax, patch.ProteinMax)

	if err := ValidateProfile(cur); err != nil {
		return Profile{}, err
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE profile SET height_cm = ?, target_weight_min = ?, target_weight_max = ?,
		 kcal_min = ?, kcal_max = ?, protein_min = ?, protein_max = ?, updated_at = ?
		 WHERE id = 1`,
		cur.HeightCm, cur.TargetWeightMin, cur.TargetWeightMax,
		cur.KcalMin, cur.KcalMax, cur.ProteinMin, cur.ProteinMax,
		time.Now().UTC().Format(time.RFC3339)); err != nil {
		return Profile{}, fmt.Errorf("actualizar perfil: %w", err)
	}
	return s.Profile(ctx)
}

// ---------------------------------------------------------------- alimentos

// FoodByID busca un alimento por id.
func (s *Store) FoodByID(ctx context.Context, id int64) (Food, error) {
	const q = `SELECT id, name, kcal100, protein100, fat100, carbs100, fiber100,
		sugar100, sodium_mg100, source, source_detail FROM foods WHERE id = ?`
	return scanFood(s.db.QueryRowContext(ctx, q, id))
}

// SearchFoods busca alimentos por nombre (LIKE, sin distinguir mayúsculas).
func (s *Store) SearchFoods(ctx context.Context, query string, limit int) ([]Food, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := strings.TrimSpace(query)
	const base = `SELECT id, name, kcal100, protein100, fat100, carbs100, fiber100,
		sugar100, sodium_mg100, source, source_detail FROM foods `
	const tail = ` ORDER BY name LIMIT ?`

	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = s.db.QueryContext(ctx, base+tail, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, base+
			`WHERE name LIKE ? COLLATE NOCASE OR name LIKE ? COLLATE NOCASE`+tail,
			q+"%", "%"+q+"%", limit)
	}
	if err != nil {
		return nil, fmt.Errorf("buscar alimentos: %w", err)
	}
	defer rows.Close()

	out := make([]Food, 0, limit)
	for rows.Next() {
		f, err := scanFood(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// CountFoods cuenta los alimentos cargados.
func (s *Store) CountFoods(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM foods`).Scan(&n)
	return n, err
}

// AllFoods devuelve todos los alimentos ordenados por nombre. Lo usa el parser
// de texto libre, que necesita el catálogo completo (no una búsqueda paginada).
func (s *Store) AllFoods(ctx context.Context) ([]Food, error) {
	const q = `SELECT id, name, kcal100, protein100, fat100, carbs100, fiber100,
		sugar100, sodium_mg100, source, source_detail FROM foods ORDER BY name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("leer alimentos: %w", err)
	}
	defer rows.Close()

	out := make([]Food, 0, 64)
	for rows.Next() {
		f, err := scanFood(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanFood(sc scanner) (Food, error) {
	var f Food
	err := sc.Scan(&f.ID, &f.Name, &f.Per100.Kcal, &f.Per100.Protein, &f.Per100.Fat,
		&f.Per100.Carbs, &f.Per100.Fiber, &f.Per100.Sugar, &f.Per100.Sodium,
		&f.Source, &f.SourceDetail)
	if errors.Is(err, sql.ErrNoRows) {
		return Food{}, ErrNotFound
	}
	if err != nil {
		return Food{}, fmt.Errorf("leer alimento: %w", err)
	}
	return f, nil
}

// ---------------------------------------------------------------- diario

// InsertEntry agrega una línea al diario y devuelve la fila persistida.
func (s *Store) InsertEntry(ctx context.Context, e NewEntry) (Entry, error) {
	const q = `INSERT INTO diary_entries
		(day, meal, food_id, food_name, grams, kcal, protein, fat, carbs, fiber, sugar, sodium, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, created_at`

	m := e.Macros
	created := time.Now().UTC().Format(time.RFC3339)

	var id int64
	var createdAt string
	err := s.db.QueryRowContext(ctx, q,
		e.Day, e.Meal, e.FoodID, e.FoodName, e.Grams,
		m.Kcal, m.Protein, m.Fat, m.Carbs, m.Fiber, m.Sugar, m.Sodium,
		e.Note, created,
	).Scan(&id, &createdAt)
	if err != nil {
		return Entry{}, fmt.Errorf("insertar entrada: %w", err)
	}

	ts, _ := time.Parse(time.RFC3339, createdAt)
	return Entry{
		ID:        id,
		Day:       e.Day,
		Meal:      e.Meal,
		FoodID:    e.FoodID,
		FoodName:  e.FoodName,
		Grams:     e.Grams,
		Macros:    m,
		Note:      e.Note,
		CreatedAt: ts,
	}, nil
}

// DeleteEntry borra una línea del diario.
func (s *Store) DeleteEntry(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM diary_entries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("borrar entrada: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EntryByID lee una línea del diario.
func (s *Store) EntryByID(ctx context.Context, id int64) (Entry, error) {
	const q = `SELECT id, day, meal, food_id, food_name, grams, kcal, protein, fat,
		carbs, fiber, sugar, sodium, note, created_at FROM diary_entries WHERE id = ?`
	var e Entry
	var foodID sql.NullInt64
	var createdAt string
	err := s.db.QueryRowContext(ctx, q, id).Scan(&e.ID, &e.Day, &e.Meal, &foodID,
		&e.FoodName, &e.Grams, &e.Macros.Kcal, &e.Macros.Protein, &e.Macros.Fat,
		&e.Macros.Carbs, &e.Macros.Fiber, &e.Macros.Sugar, &e.Macros.Sodium,
		&e.Note, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, fmt.Errorf("leer entrada: %w", err)
	}
	if foodID.Valid {
		e.FoodID = &foodID.Int64
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return e, nil
}

// DayEntries devuelve las líneas de un día ordenadas por comida y hora de carga.
func (s *Store) DayEntries(ctx context.Context, day string) ([]Entry, error) {
	const q = `SELECT id, day, meal, food_id, food_name, grams, kcal, protein, fat,
		carbs, fiber, sugar, sodium, note, created_at
		FROM diary_entries WHERE day = ? ORDER BY created_at, id`

	rows, err := s.db.QueryContext(ctx, q, day)
	if err != nil {
		return nil, fmt.Errorf("leer diario de %s: %w", day, err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var foodID sql.NullInt64
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Day, &e.Meal, &foodID, &e.FoodName, &e.Grams,
			&e.Macros.Kcal, &e.Macros.Protein, &e.Macros.Fat, &e.Macros.Carbs,
			&e.Macros.Fiber, &e.Macros.Sugar, &e.Macros.Sodium, &e.Note, &createdAt); err != nil {
			return nil, fmt.Errorf("leer entrada: %w", err)
		}
		if foodID.Valid {
			v := foodID.Int64
			e.FoodID = &v
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		oi, oj := mealOrder(out[i].Meal), mealOrder(out[j].Meal)
		if oi != oj {
			return oi < oj
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

// DayTotals suma los macros del día usando el motor nutricional.
func (s *Store) DayTotals(ctx context.Context, day string) (nutrition.Macros, error) {
	entries, err := s.DayEntries(ctx, day)
	if err != nil {
		return nutrition.Zero, err
	}
	ms := make([]nutrition.Macros, 0, len(entries))
	for _, e := range entries {
		ms = append(ms, e.Macros)
	}
	return nutrition.Sum(ms...), nil
}

// LoggedDays lista los días con al menos una entrada desde `since` (inclusive),
// en orden descendente. Se usa para adherencia y racha.
func (s *Store) LoggedDays(ctx context.Context, since string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT day FROM diary_entries WHERE day >= ? ORDER BY day DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("leer días cargados: %w", err)
	}
	defer rows.Close()

	var days []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// ---------------------------------------------------------------- peso

// SaveWeight hace upsert del peso del día y, si esa fecha es la más reciente
// del histórico, actualiza el peso del perfil.
//
// La nota es opcional (variádica) para no romper a los llamadores que solo
// guardan kg. Cargar una pesa histórica NO pisa el peso actual del perfil:
// el perfil solo sigue a la fecha máxima de `weights`.
func (s *Store) SaveWeight(ctx context.Context, day string, kg float64, note ...string) error {
	n := ""
	if len(note) > 0 {
		n = strings.TrimSpace(note[0])
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tx peso: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO weights (day, kg, note, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(day) DO UPDATE SET kg = excluded.kg,
		   note = CASE WHEN excluded.note <> '' THEN excluded.note ELSE weights.note END,
		   created_at = excluded.created_at`,
		day, kg, n, now); err != nil {
		return fmt.Errorf("guardar peso: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE profile SET weight_kg = ?, updated_at = ?
		 WHERE id = 1 AND (SELECT MAX(day) FROM weights) = ?`, kg, now, day); err != nil {
		return fmt.Errorf("actualizar perfil: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Weights devuelve el historial completo (más reciente primero), hasta limit.
func (s *Store) Weights(ctx context.Context, limit int) ([]WeightRow, error) {
	return s.WeightsRange(ctx, "", "", limit)
}

// WeightsRange devuelve el historial entre dos fechas (inclusive). from/to
// vacíos no acotan ese extremo.
func (s *Store) WeightsRange(ctx context.Context, from, to string, limit int) ([]WeightRow, error) {
	if limit <= 0 {
		limit = 120
	}
	q := `SELECT day, kg, note FROM weights WHERE 1 = 1`
	args := make([]any, 0, 3)
	if from != "" {
		q += ` AND day >= ?`
		args = append(args, from)
	}
	if to != "" {
		q += ` AND day <= ?`
		args = append(args, to)
	}
	q += ` ORDER BY day DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("leer pesos: %w", err)
	}
	defer rows.Close()

	var out []WeightRow
	for rows.Next() {
		var w WeightRow
		if err := rows.Scan(&w.Day, &w.Kg, &w.Note); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// WeightNeighbors devuelve la pesa inmediatamente anterior y posterior a day.
// Sirve para calcular el delta correcto cuando se carga una fecha histórica.
func (s *Store) WeightNeighbors(ctx context.Context, day string) (prev, next *WeightRow, err error) {
	read := func(q string) (*WeightRow, error) {
		var w WeightRow
		err := s.db.QueryRowContext(ctx, q, day).Scan(&w.Day, &w.Kg, &w.Note)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("leer pesa vecina: %w", err)
		}
		return &w, nil
	}
	if prev, err = read(`SELECT day, kg, note FROM weights WHERE day < ? ORDER BY day DESC LIMIT 1`); err != nil {
		return nil, nil, err
	}
	if next, err = read(`SELECT day, kg, note FROM weights WHERE day > ? ORDER BY day ASC LIMIT 1`); err != nil {
		return nil, nil, err
	}
	return prev, next, nil
}

// ---------------------------------------------------------------- rutina

// Routine devuelve todos los ejercicios ordenados por día y orden.
func (s *Store) Routine(ctx context.Context) ([]Exercise, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, day, ord, name, sets, reps, rir, rest_sec, notes, youtube_query
		 FROM routine_exercises ORDER BY day, ord`)
	if err != nil {
		return nil, fmt.Errorf("leer rutina: %w", err)
	}
	defer rows.Close()

	var out []Exercise
	for rows.Next() {
		var e Exercise
		if err := rows.Scan(&e.ID, &e.Day, &e.Ord, &e.Name, &e.Sets, &e.Reps,
			&e.RIR, &e.RestSec, &e.Notes, &e.YouTubeQuery); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SaveWorkout registra (o reemplaza) el entrenamiento de un día y sus series.
func (s *Store) SaveWorkout(ctx context.Context, day, routineDay, notes string, durationMin int, sets []SetInput) (Workout, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Workout{}, fmt.Errorf("tx entrenamiento: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO workouts (day, routine_day, notes, duration_min, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(day) DO UPDATE SET routine_day = excluded.routine_day,
		   notes = excluded.notes, duration_min = excluded.duration_min
		 RETURNING id`,
		day, routineDay, notes, durationMin, now).Scan(&id); err != nil {
		return Workout{}, fmt.Errorf("guardar entrenamiento: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM workout_sets WHERE workout_id = ?`, id); err != nil {
		return Workout{}, fmt.Errorf("limpiar series: %w", err)
	}
	for _, st := range sets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO workout_sets (workout_id, exercise_id, set_number, weight_kg, reps, rir, note, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, st.ExerciseID, st.SetNumber, st.WeightKg, st.Reps, st.RIR, st.Note, now); err != nil {
			return Workout{}, fmt.Errorf("guardar serie %d: %w", st.SetNumber, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Workout{}, err
	}

	return Workout{
		ID: id, Day: day, RoutineDay: routineDay, Notes: notes,
		DurationMin: durationMin, Sets: sets,
	}, nil
}

// WorkoutByDay devuelve el entrenamiento de un día (con series), si existe.
func (s *Store) WorkoutByDay(ctx context.Context, day string) (Workout, error) {
	var w Workout
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, day, routine_day, notes, duration_min, created_at FROM workouts WHERE day = ?`, day,
	).Scan(&w.ID, &w.Day, &w.RoutineDay, &w.Notes, &w.DurationMin, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Workout{}, ErrNotFound
	}
	if err != nil {
		return Workout{}, fmt.Errorf("leer entrenamiento: %w", err)
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	rows, err := s.db.QueryContext(ctx,
		`SELECT exercise_id, set_number, weight_kg, reps, rir, note FROM workout_sets
		 WHERE workout_id = ? ORDER BY exercise_id, set_number`, w.ID)
	if err != nil {
		return Workout{}, fmt.Errorf("leer series: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var st SetInput
		var weight sql.NullFloat64
		var reps, rir sql.NullInt64
		if err := rows.Scan(&st.ExerciseID, &st.SetNumber, &weight, &reps, &rir, &st.Note); err != nil {
			return Workout{}, err
		}
		if weight.Valid {
			v := weight.Float64
			st.WeightKg = &v
		}
		if reps.Valid {
			v := int(reps.Int64)
			st.Reps = &v
		}
		if rir.Valid {
			v := int(rir.Int64)
			st.RIR = &v
		}
		w.Sets = append(w.Sets, st)
	}
	return w, rows.Err()
}

// WorkoutsRange devuelve los entrenamientos entre dos fechas (inclusive) con
// sus series, el nombre del ejercicio y el volumen (kg * reps) por serie.
//
// `limit` acota la cantidad de entrenamientos (no de filas del join): el día
// sin series también se devuelve, porque el entrenamiento existe igual.
func (s *Store) WorkoutsRange(ctx context.Context, from, to string, limit int) ([]WorkoutDetail, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	sub := `SELECT id FROM workouts WHERE 1 = 1`
	args := make([]any, 0, 3)
	if from != "" {
		sub += ` AND day >= ?`
		args = append(args, from)
	}
	if to != "" {
		sub += ` AND day <= ?`
		args = append(args, to)
	}
	sub += ` ORDER BY day DESC LIMIT ?`
	args = append(args, limit)

	q := `SELECT w.id, w.day, w.routine_day, w.notes, w.duration_min,
	             s.exercise_id, s.set_number, s.weight_kg, s.reps, s.rir, COALESCE(s.note, ''),
	             COALESCE(e.name, ''), COALESCE(e.ord, 0)
	      FROM workouts w
	      LEFT JOIN workout_sets s ON s.workout_id = w.id
	      LEFT JOIN routine_exercises e ON e.id = s.exercise_id
	      WHERE w.id IN (` + sub + `)
	      ORDER BY w.day DESC, COALESCE(e.ord, 0), s.set_number`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("leer entrenamientos: %w", err)
	}
	defer rows.Close()

	out := make([]WorkoutDetail, 0, limit)
	index := make(map[int64]int, limit)

	for rows.Next() {
		var (
			id, durationMin int64
			day, routineDay string
			notes           string
			exerciseID      sql.NullInt64
			setNumber       sql.NullInt64
			weight          sql.NullFloat64
			reps, rir       sql.NullInt64
			setNote         string
			exerciseName    string
			ord             int64
		)
		if err := rows.Scan(&id, &day, &routineDay, &notes, &durationMin,
			&exerciseID, &setNumber, &weight, &reps, &rir, &setNote, &exerciseName, &ord); err != nil {
			return nil, fmt.Errorf("leer entrenamiento: %w", err)
		}

		pos, ok := index[id]
		if !ok {
			out = append(out, WorkoutDetail{
				ID: id, Day: day, RoutineDay: routineDay, Notes: notes,
				DurationMin: int(durationMin), Sets: []WorkoutSetDetail{},
			})
			pos = len(out) - 1
			index[id] = pos
		}
		if !exerciseID.Valid {
			continue // entrenamiento sin series
		}

		st := WorkoutSetDetail{
			ExerciseID:   exerciseID.Int64,
			ExerciseName: exerciseName,
			SetNumber:    int(setNumber.Int64),
			Note:         setNote,
		}
		if weight.Valid {
			v := weight.Float64
			st.WeightKg = &v
		}
		if reps.Valid {
			v := int(reps.Int64)
			st.Reps = &v
		}
		if rir.Valid {
			v := int(rir.Int64)
			st.RIR = &v
		}
		if st.WeightKg != nil && st.Reps != nil {
			st.VolumeKg = round1(*st.WeightKg * float64(*st.Reps))
		}
		out[pos].Sets = append(out[pos].Sets, st)
		out[pos].VolumeKg = round1(out[pos].VolumeKg + st.VolumeKg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// round1 redondea a un decimal (volúmenes y deltas de peso).
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// MealPlan devuelve las opciones del Plan Fran.
func (s *Store) MealPlan(ctx context.Context) ([]PlanItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT meal, meal_time, option, description, kcal, protein
		 FROM meal_plan ORDER BY ord, option`)
	if err != nil {
		return nil, fmt.Errorf("leer plan: %w", err)
	}
	defer rows.Close()

	var out []PlanItem
	for rows.Next() {
		var p PlanItem
		if err := rows.Scan(&p.Meal, &p.Time, &p.Option, &p.Description, &p.Kcal, &p.Protein); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlanNotes devuelve las reglas del plan (agua, sal, pesaje, ajustes).
func (s *Store) PlanNotes(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT text FROM plan_notes ORDER BY ord`)
	if err != nil {
		return nil, fmt.Errorf("leer reglas: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- utilidades

// Today devuelve la fecha de hoy en la zona configurada, formato YYYY-MM-DD.
func Today(loc *time.Location) string {
	return time.Now().In(loc).Format(timeLayout)
}

// ParseDay valida y normaliza una fecha YYYY-MM-DD.
func ParseDay(s string) (string, error) {
	t, err := time.Parse(timeLayout, strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("fecha inválida %q (formato YYYY-MM-DD)", s)
	}
	return t.Format(timeLayout), nil
}
