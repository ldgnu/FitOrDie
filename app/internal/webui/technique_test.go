package webui

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/ldgnu/fitordie/internal/store"
	"github.com/ldgnu/fitordie/static"
	"github.com/ldgnu/fitordie/templates"
)

// maxImageBytes es el techo por foto: el modal abre desde el celular en el
// gimnasio, muchas veces con datos móviles.
const maxImageBytes = 120 * 1024

// TestRoutineTechniques cuida el contenido curado de los 12 ejercicios únicos:
// que exista, que tenga la cantidad pedida de claves y errores, y sobre todo
// que la imagen referenciada exista de verdad dentro del binario. Un slug mal
// escrito deja el modal con la foto rota y nadie se entera hasta el gimnasio.
func TestRoutineTechniques(t *testing.T) {
	slugs := make(map[string]string, len(routineNames))
	for _, name := range routineNames {
		g := technique(name)
		if g == nil {
			t.Errorf("%q no tiene guía de técnica", name)
			continue
		}
		if n := len(g.Keys); n < 4 || n > 6 {
			t.Errorf("%q tiene %d claves, tiene que tener 4-6", name, n)
		}
		if n := len(g.Errors); n < 2 || n > 3 {
			t.Errorf("%q tiene %d errores, tiene que tener 2-3", name, n)
		}
		for _, k := range g.Keys {
			if strings.TrimSpace(k) == "" {
				t.Errorf("%q tiene una clave vacía", name)
			}
		}
		for _, e := range g.Errors {
			if strings.TrimSpace(e) == "" {
				t.Errorf("%q tiene un error vacío", name)
			}
		}
		if strings.TrimSpace(g.Credit) == "" {
			t.Errorf("%q no tiene crédito de imagen", name)
		}
		if prev, dup := slugs[g.Slug]; dup {
			t.Errorf("%q y %q comparten el slug %q", prev, name, g.Slug)
		}
		slugs[g.Slug] = name

		path := techniqueImagePath(name)
		if path == "" {
			t.Errorf("%q no resuelve ruta de imagen", name)
			continue
		}
		b, err := static.FS.ReadFile(path)
		if err != nil {
			t.Errorf("%q apunta a una imagen que no está embebida: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("%q tiene la imagen vacía (%s)", name, path)
		}
		if len(b) > maxImageBytes {
			t.Errorf("%q tiene la imagen en %d KB (tope %d KB)", name, len(b)/1024, maxImageBytes/1024)
		}
	}

	if got, want := len(routineTechniques), len(routineNames); got != want {
		t.Errorf("el mapa tiene %d entradas, la rutina tiene %d nombres únicos", got, want)
	}
	if technique("Ejercicio que no existe") != nil {
		t.Error("un ejercicio desconocido debe devolver nil")
	}
	if got := techniqueImagePath("Ejercicio que no existe"); got != "" {
		t.Errorf("un ejercicio desconocido no debe tener imagen, devolvió %q", got)
	}
}

// TestRutinaTecnicaModal verifica que el template enrole el botón "Técnica",
// el panel oculto con el contenido y el <dialog> que lo muestra. Si algo de
// eso falta, el botón no hace nada o el modal queda vacío.
func TestRutinaTecnicaModal(t *testing.T) {
	tmpl, err := template.New("rutina.html").Funcs(templateFuncs()).
		ParseFS(templates.FS, "layout.html", "rutina.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	d := &viewData{
		Page: "rutina", Title: "Rutina", AssetVer: "abc123", CSRF: "tok",
		Tabs: []dayTabView{{Key: "A", Name: routineDayNames["A"], Active: true}},
		Sheet: &daySheetView{
			Key: "A", Name: routineDayNames["A"],
			Exercises: []exerciseSheetView{
				{Exercise: store.Exercise{ID: 7, Name: "Prensa de piernas", Sets: 3, Reps: "10-12",
					RIR: "3-4", RestSec: 120, YouTubeQuery: "prensa de piernas técnica correcta"}},
				// Sin guía: no debe aparecer el botón (y el resto sigue andando).
				{Exercise: store.Exercise{ID: 8, Name: "Ejercicio sin ficha", Sets: 3, Reps: "10-12",
					RIR: "3-4", RestSec: 90, YouTubeQuery: "ejercicio sin ficha"}},
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", d); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`class="btn js-tech"`,
		`data-tech="tec-7"`,
		`>Técnica</button>`,
		`id="tec-7"`,
		`/static/img/exercises/prensa-piernas.jpg?v=abc123`,
		`Claves de técnica`,
		`Errores comunes`,
		`id="tech-modal"`,
		`id="tech-body"`,
		`/static/js/technique.js?v=abc123`,
		imageCredit,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("el HTML no contiene %q", want)
		}
	}

	if got := strings.Count(out, `class="btn js-tech"`); got != 1 {
		t.Errorf("esperaba un solo botón Técnica (el otro ejercicio no tiene ficha), hay %d", got)
	}
	if got := strings.Count(out, `id="tec-`); got != 1 {
		t.Errorf("esperaba un solo panel de técnica, hay %d", got)
	}
	// El panel va oculto: sin `hidden` el contenido aparecería en la tarjeta.
	if !strings.Contains(out, `<div class="tech" id="tec-7" hidden>`) {
		t.Error("el panel de técnica tiene que ir oculto")
	}
	// Nada de hotlinks: la foto sale del binario, no de un banco externo.
	if strings.Contains(out, "raw.githubusercontent.com") {
		t.Error("el template no debe apuntar a imágenes externas")
	}
}
