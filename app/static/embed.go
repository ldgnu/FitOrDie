// Package static embebe CSS, iconos, imágenes y manifest para servirlos desde
// el binario (sin nginx ni volúmenes).
package static

import "embed"

// FS contiene los assets del frontend. `img` son las fotos de los ejercicios
// (locales, sin hotlinks a bancos externos).
//
//go:embed css icons img js manifest.webmanifest
var FS embed.FS
