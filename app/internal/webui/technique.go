package webui

// Guía de técnica por ejercicio: las claves que importan, los errores que
// lesionan y la foto local que abre el modal "Técnica" de /rutina.
//
// El contenido vive en Go (igual que routineVideoIDs) y no en la base: es
// texto de referencia que se versiona con el código y no se edita en runtime.
// La clave del mapa es el nombre EXACTO del ejercicio en
// internal/store/seed.go; si alguien renombra un ejercicio, el test
// TestRoutineTechniques lo caza antes que el gimnasio.

// imageCredit es la atribución obligatoria de las fotos. free-exercise-db se
// publica bajo Unlicense (dominio público), así que no exige crédito, pero
// ponerlo es la forma honesta de respetar el trabajo ajeno.
const imageCredit = "Foto: free-exercise-db (Unlicense)"

// techniqueGuide es lo que consume el template: 4-6 claves, 2-3 errores, una
// imagen local y el crédito.
type techniqueGuide struct {
	// Slug es el nombre del archivo en static/img/exercises/<slug>.jpg.
	Slug string
	// Keys son las claves de técnica perfecta, en orden de ejecución.
	Keys []string
	// Errors son los errores comunes que hay que evitar.
	Errors []string
	// Credit es la atribución de la imagen (se completa sola).
	Credit string
}

// techniqueImageDir es la carpeta embebida donde viven las fotos.
const techniqueImageDir = "img/exercises/"

// routineTechniques es la guía curada de los 12 ejercicios únicos del
// Mesociclo 1 (Prensa de piernas se repite en los días A y C).
var routineTechniques = map[string]techniqueGuide{
	"Prensa de piernas": {
		Slug: "prensa-piernas",
		Keys: []string{
			"Apoyá espalda y cadera completas contra el respaldo: la cola no se despega nunca.",
			"Pies al ancho de hombros, en el centro de la plataforma y con el talón bien apoyado.",
			"Bajá controlado hasta unos 90° de rodilla, hasta donde la cadera siga pegada al asiento.",
			"Empujá con el talón y el mediopie, no con la punta, para no castigar la rodilla.",
			"Arriba no trabes las rodillas: dejá una microflexión y pausá un segundo.",
			"Respiración: bajás inspirando, empujás exhalando.",
		},
		Errors: []string{
			"Rodillas que se meten hacia adentro al empujar.",
			"Bajar tanto que la cadera se levanta del asiento y la lumbar se redondea.",
			"Empujar con la punta del pie: la rodilla se va hacia adelante y aparece el dolor.",
		},
	},

	"Press pecho máquina o mancuernas": {
		Slug: "press-pecho",
		Keys: []string{
			"Escápulas juntas y pegadas al respaldo o al banco, hombros abajo lejos de las orejas.",
			"Codos a 45-60° del torso, no abiertos en cruz.",
			"Bajada de 2 segundos hasta sentir el estiramiento en el pecho.",
			"Empujá sin bloquear del todo los codos y apretá el pecho arriba.",
			"Muñecas firmes, alineadas con el antebrazo.",
			"Pies firmes en el piso: dan estabilidad y evitan que el pecho se hunda.",
		},
		Errors: []string{
			"Rebotar la barra o las mancuernas contra el pecho usando impulso.",
			"Abrir los codos a 90°: sobrecarga el hombro y saca el trabajo del pecho.",
			"Despegar la cadera del banco para empujar más peso.",
		},
	},

	"Remo máquina o mancuernas": {
		Slug: "remo-maquina-mancuernas",
		Keys: []string{
			"Pecho arriba y espalda neutra; la cadera va atrás (bisagra de cadera), no se encorva.",
			"Tirá con los codos, no con las manos: los brazos son ganchos.",
			"Llevá el codo hacia la cadera y juntá la escápula al final del recorrido.",
			"Pausá 1 segundo apretando el dorsal y bajá en 2 segundos.",
			"Cuello relajado, mirada al piso un metro adelante.",
		},
		Errors: []string{
			"Usar el torso como palanca: el balanceo le saca el trabajo a la espalda.",
			"Encorvar la lumbar al tirar.",
			"Tirar con los brazos flexionando el codo antes que el hombro.",
		},
	},

	"Elevaciones laterales": {
		Slug: "elevaciones-laterales",
		Keys: []string{
			"Mancuernas al costado del cuerpo con una microflexión fija de codo.",
			"Subí hasta la altura del hombro, no más: la mano no pasa el codo.",
			"Guiá con el codo, como si derramaras agua de una jarra.",
			"Bajada lenta de 2-3 segundos, sin dejar caer el peso.",
			"Peso liviano: acá manda la técnica, no el ego.",
		},
		Errors: []string{
			"Encogimiento de hombros y balanceo de cadera para lanzar la mancuerna.",
			"Subir por encima del hombro: pinza el supraespinoso.",
			"Bloquear el aire arriba: se respira durante todo el recorrido.",
		},
	},

	"Curl bíceps": {
		Slug: "curl-biceps",
		Keys: []string{
			"Codos pegados al torso y quietos durante todo el movimiento.",
			"Subí contrayendo el bíceps, sin adelantar los hombros.",
			"Apretá 1 segundo arriba antes de bajar.",
			"Bajada de 2 segundos: la fase excéntrica es la que hace crecer.",
			"Muñeca neutra, sin quebrarla hacia atrás.",
		},
		Errors: []string{
			"Balanceo del torso para subir el peso.",
			"Codos que se van adelante: el curl se convierte en press.",
			"Soltar de golpe en la bajada (se pierde la mitad del estímulo).",
		},
	},

	"Curl femoral máquina": {
		Slug: "curl-femoral",
		Keys: []string{
			"Rodillas alineadas con el eje de la máquina y cadera bien pegada al pad.",
			"Llevá el talón hacia la cola sin despegar la cadera del asiento.",
			"Apretá los isquios 1 segundo arriba.",
			"Bajada de 3 segundos, frenando el peso todo el camino.",
			"Rango completo, pero sin dolor detrás de la rodilla.",
		},
		Errors: []string{
			"Levantar la cadera para poder subir: termina tirando la lumbar.",
			"Soltar el peso de golpe: la bajada es donde más se gana.",
			"Poner los tobillos mal en el rodillo (palanca corta y molestia en el tendón).",
		},
	},

	"Press militar sentado": {
		Slug: "press-militar",
		Keys: []string{
			"Espalda apoyada en el respaldo, abdomen firme y costillas abajo.",
			"Mancuernas a la altura de las orejas, codos algo adelantados.",
			"Empujá hacia arriba y un poco atrás, sobre la cabeza.",
			"Sin arquear la lumbar: si no sube, bajá el peso.",
			"Bajada controlada hasta la altura de la barbilla o las orejas.",
		},
		Errors: []string{
			"Arquear la espalda y empujar con el pecho.",
			"Bajar solo hasta la frente: rango corto y menos estímulo.",
			"Bloquear los codos de golpe arriba.",
		},
	},

	"Jalón al pecho": {
		Slug: "jalon-al-pecho",
		Keys: []string{
			"Agarre al ancho de hombros o un poco más, pulgar cerrado.",
			"Pecho arriba y una inclinación atrás de 10-15°, sin balanceo.",
			"Bajá los codos hacia las costillas: no tirés con las manos.",
			"Barra al pecho, nunca detrás de la nuca.",
			"Subida controlada de 2-3 segundos dejando estirar el dorsal.",
		},
		Errors: []string{
			"Tirar con impulso del torso (balanceo hacia atrás).",
			"Bajar la barra detrás de la cabeza: compromete el hombro.",
			"Usar un agarre tan ancho que el recorrido se acorta y molesta el hombro.",
		},
	},

	"Extensión tríceps en polea": {
		Slug: "extension-triceps-polea",
		Keys: []string{
			"Codos pegados al torso y fijos: solo se mueve el antebrazo.",
			"Empujá hasta extender del todo y apretá el tríceps 1 segundo.",
			"Torso levemente inclinado, sin usar el peso del cuerpo.",
			"Muñecas firmes; con cuerda o barra, el mismo control.",
			"Volvé controlado hasta unos 90° de codo, sin abrirlos.",
		},
		Errors: []string{
			"Mover los codos y empujar con hombro y pecho.",
			"Colgarse del peso inclinando el torso hacia adelante.",
			"Extender de golpe sin control en la vuelta.",
		},
	},

	"Press inclinado con mancuernas": {
		Slug: "press-inclinado-mancuernas",
		Keys: []string{
			"Banco a 30°: más inclinación lo convierte en press militar.",
			"Escápulas juntas, hombros atrás y abajo.",
			"Codos a 45-60°, bajada de 2 segundos hasta el pecho alto.",
			"Empujá en diagonal, sin chocar las mancuernas arriba.",
			"Pies firmes; cadera y cabeza siempre apoyadas en el banco.",
		},
		Errors: []string{
			"Abrir los codos a 90° y bajar hasta el esternón.",
			"Despegar la cadera del banco para empujar más.",
			"Usar 45° o más de inclinación: el deltoides se come el trabajo del pecho.",
		},
	},

	"Remo unilateral con mancuerna": {
		Slug: "remo-unilateral-mancuerna",
		Keys: []string{
			"Una mano y una rodilla en el banco; espalda paralela al piso.",
			"Columna neutra y cuello largo: la mirada va al piso, no al frente.",
			"Tirá el codo hacia la cadera, pegadito al torso.",
			"Pausá 1 segundo arriba y bajá en 2 segundos hasta estirar el dorsal.",
			"Cadera quieta: no rotes el torso para subir más.",
		},
		Errors: []string{
			"Rotar el torso para completar la repetición.",
			"Elegir un peso que obliga a encorvar la espalda.",
			"Tirar con el bíceps en vez de llevar el codo atrás.",
		},
	},

	"Remo en polea baja sentado": {
		Slug: "remo-polea-baja",
		Keys: []string{
			"Pies en la plataforma, rodillas algo flexionadas, pecho arriba.",
			"Espalda recta: la bisagra la hace la cadera, sin encorvarse.",
			"Tirá los codos hacia atrás y juntá las escápulas al final.",
			"Pausá 1 segundo y volvé controlado dejando estirar el dorsal.",
			"Hombros abajo y cuello relajado.",
		},
		Errors: []string{
			"Balanceo de torso adelante y atrás para mover más peso.",
			"Encorvar la espalda al final del recorrido.",
			"Tirar del mango con las manos sin llevar los codos atrás.",
		},
	},
}

// technique devuelve la guía del ejercicio, o nil si no hay contenido curado.
// Devuelve una copia con el crédito completo para que el template sólo tenga
// que imprimir campos.
func technique(name string) *techniqueGuide {
	g, ok := routineTechniques[name]
	if !ok {
		return nil
	}
	g.Credit = imageCredit
	return &g
}

// techniqueImagePath es la ruta servida de la foto del ejercicio (vacía si no
// hay guía). Sirve para los tests y para el template.
func techniqueImagePath(name string) string {
	g := technique(name)
	if g == nil {
		return ""
	}
	return techniqueImageDir + g.Slug + ".jpg"
}
