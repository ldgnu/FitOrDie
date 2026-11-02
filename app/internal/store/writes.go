// Escrituras validadas que comparten la API y el panel web.
//
// Antes cada superficie repetía las mismas reglas (comida válida, gramos en
// rango, resolución de food_id o carga manual). Viven acá para que el bot, la
// API JSON y los formularios de la web no puedan divergir.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ldgnu/fitordie/internal/nutrition"
)

// ErrInvalidInput envuelve los errores de validación de una carga: la API y la
// web los traducen a 400 con el mensaje tal cual.
var ErrInvalidInput = errors.New("entrada inválida")

// maxFoodName limita el nombre libre de un alimento cargado a mano.
const maxFoodName = 120

// EntryInput es una línea de diario a insertar. O viene FoodID (se leen los
// valores por 100 g de `foods`) o viene FoodName + Per100 (carga manual).
type EntryInput struct {
	Day      string
	Meal     string
	FoodID   *int64
	FoodName string
	Grams    float64
	Per100   *nutrition.Per100
	Note     string
}

// AddEntry valida y persiste una línea del diario, con los macros calculados en
// el servidor. Nunca confía en macros que mande el cliente.
func (s *Store) AddEntry(ctx context.Context, in EntryInput) (Entry, error) {
	day, err := ParseDay(in.Day)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if !ValidMeal(in.Meal) {
		return Entry{}, fmt.Errorf("%w: comida inválida: usá %s", ErrInvalidInput, mealSlugs())
	}
	if in.Grams <= 0 || in.Grams > 5000 {
		return Entry{}, fmt.Errorf("%w: gramos fuera de rango (0 < g <= 5000)", ErrInvalidInput)
	}

	entry := NewEntry{
		Day:   day,
		Meal:  in.Meal,
		Grams: in.Grams,
		Note:  strings.TrimSpace(in.Note),
	}
	if utf8.RuneCountInString(entry.Note) > 200 {
		return Entry{}, fmt.Errorf("%w: nota demasiado larga (máx 200 caracteres)", ErrInvalidInput)
	}

	switch {
	case in.FoodID != nil && *in.FoodID > 0:
		f, err := s.FoodByID(ctx, *in.FoodID)
		if err != nil {
			return Entry{}, err
		}
		id := f.ID
		entry.FoodID = &id
		entry.FoodName = f.Name
		entry.Macros = nutrition.Scale(f.Per100, in.Grams)

	case in.Per100 != nil && strings.TrimSpace(in.FoodName) != "":
		name := strings.TrimSpace(in.FoodName)
		if utf8.RuneCountInString(name) > maxFoodName {
			return Entry{}, fmt.Errorf("%w: nombre demasiado largo (máx %d caracteres)", ErrInvalidInput, maxFoodName)
		}
		if err := nutrition.ValidatePer100(*in.Per100); err != nil {
			return Entry{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		entry.FoodName = name
		entry.Macros = nutrition.Scale(*in.Per100, in.Grams)

	default:
		return Entry{}, fmt.Errorf("%w: falta food_id, o food_name + per100 para carga manual", ErrInvalidInput)
	}

	return s.InsertEntry(ctx, entry)
}

// DeleteWeight borra la pesa de un día. Si esa pesa era la más reciente, el
// perfil vuelve al peso de la nueva fecha máxima (o queda como está si no
// queda ninguna).
func (s *Store) DeleteWeight(ctx context.Context, day string) error {
	d, err := ParseDay(day)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tx peso: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM weights WHERE day = ?`, d)
	if err != nil {
		return fmt.Errorf("borrar peso: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	// El perfil sigue a la fecha máxima: recalcular después del borrado evita
	// dejar el peso actual apuntando a una pesa que ya no existe.
	if _, err := tx.ExecContext(ctx,
		`UPDATE profile SET weight_kg = (SELECT kg FROM weights ORDER BY day DESC LIMIT 1),
		 updated_at = ? WHERE id = 1 AND EXISTS (SELECT 1 FROM weights)`,
		nowRFC3339()); err != nil {
		return fmt.Errorf("actualizar perfil tras borrar peso: %w", err)
	}

	return tx.Commit()
}

// mealSlugs lista las comidas válidas para el mensaje de error.
func mealSlugs() string {
	parts := make([]string, 0, len(Meals))
	for _, m := range Meals {
		parts = append(parts, m.Slug)
	}
	return strings.Join(parts, ", ")
}

// nowRFC3339 es el timestamp que usan las columnas created_at/updated_at.
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
