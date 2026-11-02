// Version devuelve un hash corto de los assets embebidos (CSS + JS).
//
// Va como ?v= en las URLs de los assets para que un deploy nuevo no quede
// tapado por la caché de Cloudflare, que guarda .css/.js bastante más tiempo
// que el origin. Vive en este paquete porque es el dueño de los assets: lo
// usan el panel (webui) y el login (auth) para no hardcodear la versión.
package static

import (
	"crypto/sha256"
	"encoding/hex"
)

// assets son los archivos que entran en el hash: si cambia el CSS o el JS,
// cambia la URL y el borde vuelve a pedir el archivo.
var assets = []string{"css/app.css", "js/video.js", "js/sheet.js"}

// Version se calcula una sola vez al arrancar (init del paquete).
var Version = func() string {
	h := sha256.New()
	for _, name := range assets {
		if b, err := FS.ReadFile(name); err == nil {
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}()
