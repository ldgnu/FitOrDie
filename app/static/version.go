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
	"io/fs"
)

// assetDirs son los directorios que entran en el hash: si cambia el CSS, el JS
// o una foto de ejercicio, cambia la URL y el borde vuelve a pedir el archivo.
// embed.FS lista ordenado, así que el hash es estable entre builds.
var assetDirs = []string{"css", "js", "img"}

// Version se calcula una sola vez al arrancar (init del paquete).
var Version = func() string {
	h := sha256.New()
	for _, dir := range assetDirs {
		_ = fs.WalkDir(FS, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if b, err := FS.ReadFile(path); err == nil {
				// La ruta también entra: mover un archivo cambia la versión.
				h.Write([]byte(path))
				h.Write(b)
			}
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}()
