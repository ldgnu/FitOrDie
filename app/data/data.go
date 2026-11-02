// Package data embebe los datos semilla de FitOrDie en el binario, para que el
// deploy sea un único ejecutable sin dependencias externas.
package data

import _ "embed"

// AlimentosCSV es una copia de ../../data/alimentos.csv (misma ruta que usa el
// proyecto legacy). Se embebe porque go:embed no puede leer directorios padre.
// Para resincronizar: scripts/sync-data.sh
//
//go:embed alimentos.csv
var AlimentosCSV []byte
