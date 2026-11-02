/* ==========================================================================
   FitOrDie — service worker
   Estrategia:
     - navegación (HTML): red primero, caché como respaldo (offline lectura)
     - estáticos (css/js/iconos): caché primero, refresco en segundo plano
     - /api/*: siempre red, sin caché (datos personales y mutaciones)
   Nota: no se cachea nada cross-origin ni requests que no sean GET.
   ========================================================================== */

const VERSION = "fitordie-v2";
const SHELL = [
  "./",
  "./index.html",
  "./comidas.html",
  "./rutina.html",
  "./progreso.html",
  "./perfil.html",
  "./css/app.css",
  "./css/tokens.css",
  "./css/base.css",
  "./css/components.css",
  "./js/demo.js",
  "./manifest.webmanifest",
  "./icons/icon.svg",
  "./icons/icon-192.png",
  "./icons/icon-512.png",
  "./icons/apple-touch-icon-180.png"
];

self.addEventListener("install", (event) => {
  /* Cachear de a uno: si un archivo falta (rutas distintas en Go, por ejemplo),
     el service worker igual se instala con lo que pudo guardar. */
  event.waitUntil(
    caches
      .open(VERSION)
      .then((cache) =>
        Promise.all(
          SHELL.map((url) =>
            cache.add(url).catch(() => {
              /* archivo ausente: se ignora, no rompe la instalación */
            })
          )
        )
      )
      .then(() => self.skipWaiting())
      .catch(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;

  // API: ni caché ni mentiras. Si no hay red, se avisa.
  if (url.pathname.startsWith("/api/")) {
    event.respondWith(
      fetch(req).catch(
        () =>
          new Response(JSON.stringify({ error: "offline", detail: "Sin red: no se guardó nada." }), {
            status: 503,
            headers: { "Content-Type": "application/json; charset=utf-8" }
          })
      )
    );
    return;
  }

  // Navegación: red primero para no servir datos viejos como si fueran de hoy.
  if (req.mode === "navigate") {
    event.respondWith(
      fetch(req)
        .then((res) => {
          const copy = res.clone();
          caches.open(VERSION).then((cache) => cache.put(req, copy));
          return res;
        })
        .catch(() =>
          caches.match(req).then((hit) => hit || caches.match("./index.html")).then(
            (hit) =>
              hit ||
              new Response(
                "<!doctype html><meta charset=utf-8><title>Sin red</title>" +
                  "<body style=\"font:16px system-ui;background:#0a0f0e;color:#eaf2ef;padding:2rem\">" +
                  "<h1>Sin red</h1><p>No hay conexión y esta pantalla no está guardada. " +
                  "Probá de nuevo cuando vuelva la red.</p>",
                { headers: { "Content-Type": "text/html; charset=utf-8" } }
              )
          )
        )
    );
    return;
  }

  // Estáticos: red primero (un deploy no queda tapado por el caché viejo),
  // caché como respaldo cuando no hay red. Sin hashes en los nombres, esta es
  // la opción honesta; si algún día se agregan hashes, se puede pasar a
  // stale-while-revalidate.
  event.respondWith(
    fetch(req)
      .then((res) => {
        if (res && res.ok) {
          const copy = res.clone();
          caches.open(VERSION).then((cache) => cache.put(req, copy));
        }
        return res;
      })
      .catch(() => caches.match(req).then((hit) => hit || Response.error()))
  );
});
