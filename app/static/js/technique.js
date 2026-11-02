/* Modal de técnica de la rutina.
 *
 * Un solo <dialog> para todos los ejercicios del día. El contenido de cada
 * ejercicio vive en un panel oculto (#tec-<id>) que el servidor ya renderizó;
 * al abrir se clona dentro del modal. Así no hay ningún request extra: la foto
 * es local (/static/img/exercises/...) y no se pide hasta que abrís el modal.
 *
 * Cierra por botón, por ESC (cancel nativo) y por click en el backdrop.
 * Bloquea el scroll del body mientras está abierto (iOS incluido).
 * Sin dependencias, sin jQuery, sin build step.
 */
(function () {
  'use strict';

  var dlg = document.getElementById('tech-modal');
  var body = document.getElementById('tech-body');
  var titleEl = document.getElementById('tech-modal-title');
  var link = document.getElementById('tech-link');
  if (!dlg || !body || !titleEl || !link) return;

  var canModal = typeof dlg.showModal === 'function';
  var lastFocus = null;

  function lock(on) {
    document.documentElement.classList.toggle('is-locked', on);
    document.body.classList.toggle('is-locked', on);
  }

  function clearBody() {
    while (body.firstChild) body.removeChild(body.firstChild);
  }

  // Clonar en vez de mover: el panel original queda intacto y el modal se
  // puede abrir tantas veces como quieras.
  function fill(panel) {
    clearBody();
    var kids = panel.children;
    for (var i = 0; i < kids.length; i++) body.appendChild(kids[i].cloneNode(true));
  }

  function setLink(btn) {
    var id = btn.getAttribute('data-tech-video') || '';
    var url = id
      ? 'https://www.youtube.com/watch?v=' + encodeURIComponent(id)
      : (btn.getAttribute('data-tech-search') || '');
    if (!url) {
      link.hidden = true;
      link.removeAttribute('href');
      return;
    }
    link.hidden = false;
    link.href = url;
    link.textContent = id ? 'Ver el video de este ejercicio' : 'Buscar videos de este ejercicio';
  }

  function open(btn) {
    var id = btn.getAttribute('data-tech') || '';
    var panel = id ? document.getElementById(id) : null;
    if (!panel) return;

    titleEl.textContent = btn.getAttribute('data-title') || 'Técnica';
    setLink(btn);
    fill(panel);
    body.scrollTop = 0;

    if (canModal) {
      dlg.showModal();
    } else {
      // Fallback para navegadores sin <dialog> modal: mismo look, ESC manual.
      dlg.setAttribute('open', '');
      dlg.classList.add('is-fallback');
      document.addEventListener('keydown', legacyEscape);
    }
    lock(true);
  }

  function close() {
    if (!dlg.open && !dlg.hasAttribute('open')) return;
    if (canModal) {
      dlg.close();
    } else {
      dlg.removeAttribute('open');
      dlg.classList.remove('is-fallback');
      document.removeEventListener('keydown', legacyEscape);
      teardown();
    }
  }

  function teardown() {
    clearBody();
    lock(false);
    if (lastFocus && typeof lastFocus.focus === 'function') lastFocus.focus();
    lastFocus = null;
  }

  function legacyEscape(ev) {
    if (ev.key === 'Escape' || ev.key === 'Esc') close();
  }

  // Abrir: delegación, así funciona con todos los botones sin listeners propios.
  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest && ev.target.closest('[data-tech]');
    if (!btn) return;
    ev.preventDefault();
    lastFocus = btn;
    open(btn);
  });

  dlg.addEventListener('click', function (ev) {
    if (ev.target === dlg) close(); // click en el backdrop
  });

  var closeBtn = dlg.querySelector('[data-tech-close]');
  if (closeBtn) {
    closeBtn.addEventListener('click', function (ev) {
      ev.preventDefault();
      close();
    });
  }

  // ESC en <dialog> dispara cancel: el cierre lo hace el navegador, acá
  // limpiamos. Si algún navegador cancela sin cerrar, forzamos.
  dlg.addEventListener('cancel', function (ev) {
    if (!canModal) return;
    ev.preventDefault();
    close();
  });

  dlg.addEventListener('close', teardown);
})();
