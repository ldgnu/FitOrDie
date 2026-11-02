// Package static embebe CSS, iconos y manifest para servirlos desde el
// binario (sin nginx ni volúmenes).
package static

import "embed"

// FS contiene los assets del frontend.
//
//go:embed css icons js manifest.webmanifest
var FS embed.FS
