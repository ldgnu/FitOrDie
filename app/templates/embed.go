// Package templates embebe los templates HTML del servidor.
//
// Los templates implementan el sistema de diseño de app/design con el contrato
// de datos real del backend (internal/store + internal/nutrition).
package templates

import "embed"

// FS contiene todos los .html del directorio.
//
//go:embed *.html
var FS embed.FS
