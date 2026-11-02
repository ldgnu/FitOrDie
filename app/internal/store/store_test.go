package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ldgnu/fitordie/internal/nutrition"
	"github.com/ldgnu/fitordie/internal/store"
)

// openTest abre una base temporal ya sembrada (alimentos, plan, rutina).
func openTest(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir base: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	if err := st.Seed(context.Background(), false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return st
}

func TestSeedCargaAlimentosPerfilPlanYRutina(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	n, err := st.CountFoods(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 49 {
		t.Fatalf("alimentos: esperaba 49, hay %d", n)
	}

	p, err := st.Profile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Javi" || p.HeightCm != 185 || p.KcalMin != 2150 || p.ProteinMin != 155 {
		t.Fatalf("perfil inesperado: %+v", p)
	}

	plan, err := st.MealPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatal("plan Fran vacío")
	}

	routine, err := st.Routine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(routine) != 13 {
		t.Fatalf("rutina: esperaba 13 ejercicios (5+4+4), hay %d", len(routine))
	}
}

func TestSeedEsIdempotente(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	before, _ := st.CountFoods(ctx)
	if err := st.Seed(ctx, false); err != nil {
		t.Fatalf("segundo seed: %v", err)
	}
	after, _ := st.CountFoods(ctx)
	if before != after {
		t.Fatalf("el seed duplicó alimentos: %d -> %d", before, after)
	}
	routine, _ := st.Routine(ctx)
	if len(routine) != 13 {
		t.Fatalf("el seed duplicó rutina: %d ejercicios", len(routine))
	}
}

// TestCargarComidaYVerTotalesDelDia cubre el criterio de aceptación
// "se puede cargar una comida y ver totales del día".
func TestCargarComidaYVerTotalesDelDia(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	day := "2026-10-22"

	pollo, err := st.SearchFoods(ctx, "pollo", 5)
	if err != nil || len(pollo) == 0 {
		t.Fatalf("buscar pollo: %v (%d resultados)", err, len(pollo))
	}
	f := pollo[0]
	if f.Name != "Pollo pechuga" {
		t.Fatalf("primer resultado inesperado: %s", f.Name)
	}

	// El escalado lo hace el motor nutricional; el store solo persiste.
	macros := nutrition.Scale(f.Per100, 250)
	id := f.ID
	entry, err := st.InsertEntry(ctx, store.NewEntry{
		Day: day, Meal: "almuerzo", FoodID: &id, FoodName: f.Name,
		Grams: 250, Macros: macros,
	})
	if err != nil {
		t.Fatalf("insertar entrada: %v", err)
	}
	if entry.ID == 0 {
		t.Fatal("entrada sin id")
	}

	totals, err := st.DayTotals(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	// Pollo pechuga: 165 kcal / 31 P por 100 g -> 250 g = 412,5 kcal / 77,5 P
	if diff := totals.Kcal - 412.5; diff > 0.01 || diff < -0.01 {
		t.Fatalf("kcal del día: esperaba 412.5, obtuve %v", totals.Kcal)
	}
	if diff := totals.Protein - 77.5; diff > 0.01 || diff < -0.01 {
		t.Fatalf("proteína del día: esperaba 77.5, obtuve %v", totals.Protein)
	}

	// La entrada aparece en la comida correcta.
	entries, err := st.DayEntries(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Meal != "almuerzo" || entries[0].Grams != 250 {
		t.Fatalf("entradas del día inesperadas: %+v", entries)
	}

	// Borrar deja el día en cero.
	if err := st.DeleteEntry(ctx, entry.ID); err != nil {
		t.Fatalf("borrar: %v", err)
	}
	if err := st.DeleteEntry(ctx, entry.ID); err != store.ErrNotFound {
		t.Fatalf("borrar dos veces: esperaba ErrNotFound, obtuve %v", err)
	}
	totals, _ = st.DayTotals(ctx, day)
	if totals.Kcal != 0 {
		t.Fatalf("totales post-borrado: %v", totals.Kcal)
	}
}

func TestPesoYEntrenamiento(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.SaveWeight(ctx, "2026-10-20", 89.9); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveWeight(ctx, "2026-10-21", 89.65); err != nil {
		t.Fatal(err)
	}
	// Mismo día: upsert, no duplica.
	if err := st.SaveWeight(ctx, "2026-10-21", 89.4); err != nil {
		t.Fatal(err)
	}
	weights, err := st.WeightsRange(ctx, "2026-10-20", "2026-10-21", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(weights) != 2 {
		t.Fatalf("pesos: esperaba 2 filas, hay %d", len(weights))
	}
	if weights[0].Day != "2026-10-21" || weights[0].Kg != 89.4 {
		t.Fatalf("orden/upsert de pesos: %+v", weights[0])
	}

	routine, _ := st.Routine(ctx)
	var sets []store.SetInput
	reps := 12
	rir := 3
	kg := 80.0
	for i, ex := range routine {
		if ex.Day != "A" {
			continue
		}
		sets = append(sets, store.SetInput{
			ExerciseID: ex.ID, SetNumber: 1,
			WeightKg: &kg, Reps: &reps, RIR: &rir,
		})
		if i > 10 {
			break
		}
	}
	w, err := st.SaveWorkout(ctx, "2026-10-21", "A", "prueba", 45, sets)
	if err != nil {
		t.Fatalf("guardar entrenamiento: %v", err)
	}
	if len(w.Sets) != 5 {
		t.Fatalf("series guardadas: esperaba 5 (día A), hay %d", len(w.Sets))
	}
}

func TestFechas(t *testing.T) {
	loc, err := time.LoadLocation("America/Argentina/Cordoba")
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Today(loc); len(got) != 10 {
		t.Fatalf("Today() = %q, esperaba YYYY-MM-DD", got)
	}
	if _, err := store.ParseDay("22/10/2026"); err == nil {
		t.Fatal("ParseDay debía rechazar 22/10/2026")
	}
	if got, err := store.ParseDay("2026-10-22"); err != nil || got != "2026-10-22" {
		t.Fatalf("ParseDay válido: %q %v", got, err)
	}
}

// oldWeightsDB crea una base con el schema viejo de `weights` (sin note) y la
// pesa suelta del 22/10, tal como estaba en el volumen de producción.
func oldWeightsDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vieja.db")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("abrir base vieja: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE weights (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			day TEXT NOT NULL UNIQUE,
			kg REAL NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO weights (day, kg, created_at) VALUES ('2026-10-22', 87.5, '2026-10-22T23:14:37Z')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("preparar base vieja: %v", err)
		}
	}
	return path
}

// TestMigracionNoteEsIdempotenteYConservaDatos cubre la migración sobre una
// base ya existente: agrega la columna, no pierde filas y se puede repetir.
func TestMigracionNoteEsIdempotenteYConservaDatos(t *testing.T) {
	path := oldWeightsDB(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		st, err := store.Open(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		rows, err := st.Weights(ctx, 10)
		if err != nil {
			t.Fatalf("leer pesos: %v", err)
		}
		if len(rows) != 1 || rows[0].Day != "2026-10-22" || rows[0].Kg != 87.5 {
			t.Fatalf("la migración perdió datos: %+v", rows)
		}
		if rows[0].Note != "" {
			t.Fatalf("la nota arranca vacía en la base vieja: %q", rows[0].Note)
		}
		st.Close()
	}
}

// TestSeedPesosMarcaPendienteYNoBorra: el seed sobre la base real conserva el
// 87,5 del 22/10 (con nota "a confirmar") y agrega las dos pesas históricas.
func TestSeedPesosMarcaPendienteYNoBorra(t *testing.T) {
	path := oldWeightsDB(t)
	ctx := context.Background()

	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Perfil ya existente: el seed es INSERT OR IGNORE y no debe pisarlo.
	if _, err := st.DB().ExecContext(ctx, `INSERT INTO profile
		(id, name, age, height_cm, weight_kg, target_weight_min, target_weight_max,
		 kcal_min, kcal_max, protein_min, protein_max, carb_target, fat_target, notes, updated_at)
		VALUES (1, 'Javi', 38, 185, 89.65, 84, 85, 2150, 2400, 155, 170, 220, 90, '', '2026-10-22T00:00:00Z')`); err != nil {
		t.Fatalf("perfil previo: %v", err)
	}
	if err := st.Seed(ctx, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rows, err := st.Weights(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("esperaba 3 pesas (04, 05 y 22), hay %d: %+v", len(rows), rows)
	}
	byDay := map[string]store.WeightRow{}
	for _, r := range rows {
		byDay[r.Day] = r
	}
	if got := byDay["2026-10-22"]; got.Kg != 87.5 || got.Note != "a confirmar" {
		t.Fatalf("la pesa del 22/10 debe quedar con kg 87,5 y nota 'a confirmar': %+v", got)
	}
	if got := byDay["2026-10-04"]; got.Kg != 96.0 || got.Note == "" {
		t.Fatalf("pesa previa: %+v", got)
	}
	if got := byDay["2026-10-05"]; got.Kg != 89.65 || got.Note != "pesa real" {
		t.Fatalf("pesa real: %+v", got)
	}

	// El seed no toca el perfil (el peso actual sigue la fecha máxima).
	p, err := st.Profile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.WeightKg != 89.65 {
		t.Fatalf("el seed no debe reescribir el peso del perfil: %v", p.WeightKg)
	}

	// Segunda corrida: no duplica.
	if err := st.Seed(ctx, false); err != nil {
		t.Fatalf("segundo seed: %v", err)
	}
	rows, _ = st.Weights(ctx, 10)
	if len(rows) != 3 {
		t.Fatalf("el seed duplicó pesas: %d", len(rows))
	}
}

// TestSaveWeightConservaNotaYActualizaPerfilSegunFecha cubre la regla de la
// pesa histórica a nivel store.
func TestSaveWeightConservaNotaYActualizaPerfilSegunFecha(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.SaveWeight(ctx, "2026-10-21", 88.5, "pesa real"); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Profile(ctx)
	if p.WeightKg != 88.5 {
		t.Fatalf("perfil tras la pesa más reciente: esperaba 88.5, obtuve %v", p.WeightKg)
	}

	// Nota vacía no borra la nota existente.
	if err := st.SaveWeight(ctx, "2026-10-21", 88.3); err != nil {
		t.Fatal(err)
	}
	rows, _ := st.WeightsRange(ctx, "2026-10-21", "2026-10-21", 5)
	if len(rows) != 1 || rows[0].Kg != 88.3 || rows[0].Note != "pesa real" {
		t.Fatalf("upsert de nota: %+v", rows)
	}

	// Pesa anterior: entra al historial sin pisar el perfil.
	if err := st.SaveWeight(ctx, "2026-09-01", 95.0, "arranque"); err != nil {
		t.Fatal(err)
	}
	p, _ = st.Profile(ctx)
	if p.WeightKg != 88.3 {
		t.Fatalf("una pesa histórica pisó el perfil: %v", p.WeightKg)
	}
	rows, _ = st.WeightsRange(ctx, "2026-09-01", "2026-10-21", 10)
	// Incluye las pesas del seed (04/10 y 05/10): lo que importa es el orden
	// descendente y que la pesa del 01/09 quedó al final con su nota.
	if len(rows) != 4 {
		t.Fatalf("histórico: esperaba 4 filas (2 del test + 2 del seed), hay %d: %+v", len(rows), rows)
	}
	if rows[0].Day != "2026-10-21" || rows[0].Note != "pesa real" {
		t.Fatalf("primera fila: %+v", rows[0])
	}
	if last := rows[len(rows)-1]; last.Day != "2026-09-01" || last.Note != "arranque" {
		t.Fatalf("última fila: %+v", last)
	}
}

func TestUpdateProfileValidaRangos(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	min, max := 82.0, 84.5
	p, err := st.UpdateProfile(ctx, store.ProfilePatch{
		TargetWeightMin: &min, TargetWeightMax: &max,
	})
	if err != nil {
		t.Fatalf("patch válido: %v", err)
	}
	if p.TargetWeightMin != 82 || p.TargetWeightMax != 84.5 {
		t.Fatalf("perfil no actualizado: %+v", p)
	}
	if p.KcalMin != 2150 {
		t.Fatalf("campo no patcheado se perdió: %v", p.KcalMin)
	}

	bad := 10.0
	if _, err := st.UpdateProfile(ctx, store.ProfilePatch{TargetWeightMin: &bad}); err == nil {
		t.Fatal("target_weight_min=10 debía fallar")
	}
	badMax := 80.0
	if _, err := st.UpdateProfile(ctx, store.ProfilePatch{TargetWeightMax: &badMax}); err == nil {
		t.Fatal("max 80 con min 82 debía fallar (min<=max)")
	}
	huge := 450.0
	if _, err := st.UpdateProfile(ctx, store.ProfilePatch{TargetWeightMax: &huge}); err == nil {
		t.Fatal("target_weight_max=450 debía fallar (rango 20-400)")
	}
	// 400 es el límite superior inclusive: tiene que pasar.
	top := 400.0
	if _, err := st.UpdateProfile(ctx, store.ProfilePatch{TargetWeightMax: &top}); err != nil {
		t.Fatalf("target_weight_max=400 debería ser válido: %v", err)
	}
}

// TestAddEntryValidaYCalculaMacros cubre el camino único de carga: la API y la
// web escriben por acá, así que las reglas no pueden divergir.
func TestAddEntryValidaYCalculaMacros(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	foods, err := st.SearchFoods(ctx, "pollo", 1)
	if err != nil || len(foods) == 0 {
		t.Fatalf("buscar pollo: %v", err)
	}
	food := foods[0]

	// Con food_id: los macros salen de la base, no del cliente.
	e, err := st.AddEntry(ctx, store.EntryInput{
		Day: "2026-11-01", Meal: "almuerzo", FoodID: &food.ID, Grams: 200,
	})
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	want := food.Per100.Kcal * 2
	if e.Macros.Kcal < want-0.6 || e.Macros.Kcal > want+0.6 {
		t.Errorf("kcal = %v, esperaba ~%v", e.Macros.Kcal, want)
	}
	if e.FoodName != food.Name {
		t.Errorf("nombre = %q, esperaba %q", e.FoodName, food.Name)
	}

	// Carga manual: nombre + per100.
	per := nutrition.Per100{Kcal: 200, Protein: 10, Fat: 8, Carbs: 20}
	e, err = st.AddEntry(ctx, store.EntryInput{
		Day: "2026-11-01", Meal: "cena", FoodName: "Casero", Grams: 150, Per100: &per,
	})
	if err != nil {
		t.Fatalf("carga manual: %v", err)
	}
	if e.Macros.Kcal < 299 || e.Macros.Kcal > 301 {
		t.Errorf("kcal manual = %v, esperaba 300", e.Macros.Kcal)
	}

	// Errores de validación: todos envueltos en ErrInvalidInput (400 en la API).
	cases := []struct {
		name string
		in   store.EntryInput
	}{
		{"comida inválida", store.EntryInput{Day: "2026-11-01", Meal: "brunch", FoodID: &food.ID, Grams: 100}},
		{"gramos cero", store.EntryInput{Day: "2026-11-01", Meal: "cena", FoodID: &food.ID, Grams: 0}},
		{"gramos enormes", store.EntryInput{Day: "2026-11-01", Meal: "cena", FoodID: &food.ID, Grams: 6000}},
		{"sin fuente", store.EntryInput{Day: "2026-11-01", Meal: "cena", Grams: 100}},
		{"per100 fuera de rango", store.EntryInput{Day: "2026-11-01", Meal: "cena", FoodName: "X", Grams: 100,
			Per100: &nutrition.Per100{Kcal: 5000}}},
		{"fecha inválida", store.EntryInput{Day: "01/11/2026", Meal: "cena", FoodID: &food.ID, Grams: 100}},
	}
	for _, c := range cases {
		if _, err := st.AddEntry(ctx, c.in); !errors.Is(err, store.ErrInvalidInput) {
			t.Errorf("%s: err = %v, esperaba ErrInvalidInput", c.name, err)
		}
	}

	// food_id inexistente devuelve ErrNotFound (404 en la API).
	missing := int64(999999)
	if _, err := st.AddEntry(ctx, store.EntryInput{
		Day: "2026-11-01", Meal: "cena", FoodID: &missing, Grams: 100,
	}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("food_id inexistente: err = %v, esperaba ErrNotFound", err)
	}
}

// TestDeleteWeightSincronizaPerfil: al borrar la pesa más reciente, el peso del
// perfil vuelve a la fecha máxima que queda.
func TestDeleteWeightSincronizaPerfil(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.SaveWeight(ctx, "2026-11-01", 88.0, "nueva"); err != nil {
		t.Fatalf("guardar 01/11: %v", err)
	}
	if err := st.SaveWeight(ctx, "2026-10-20", 90.0, "vieja"); err != nil {
		t.Fatalf("guardar 20/10: %v", err)
	}
	p, err := st.Profile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.WeightKg != 88 {
		t.Fatalf("perfil = %v, esperaba 88", p.WeightKg)
	}

	if err := st.DeleteWeight(ctx, "2026-11-01"); err != nil {
		t.Fatalf("borrar 01/11: %v", err)
	}
	p, _ = st.Profile(ctx)
	if p.WeightKg != 90 {
		t.Errorf("perfil tras borrar = %v, esperaba 90 (la pesa del 20/10)", p.WeightKg)
	}

	if err := st.DeleteWeight(ctx, "2026-11-01"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("borrar dos veces: err = %v, esperaba ErrNotFound", err)
	}
	if err := st.DeleteWeight(ctx, "no-es-fecha"); !errors.Is(err, store.ErrInvalidInput) {
		t.Errorf("fecha inválida: err = %v, esperaba ErrInvalidInput", err)
	}
}
