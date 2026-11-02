/* Modal de video de la rutina.
 *
 * Un solo <dialog> para los 13 ejercicios y el iframe se crea recién al click:
 * si no, la página cargaría 13 players de YouTube (pesado en datos móviles) y
 * haría requests externos apenas abrís /rutina. Al cerrar se destruye el
 * iframe, que es la única forma prolija de cortar la reproducción.
 *
 * Cierra por botón, por ESC (cancel nativo) y por click en el backdrop.
 * Bloquea el scroll del body mientras está abierto (iOS incluido).
 * Sin dependencias, sin jQuery, sin build step.
 */
(function () {
  'use strict';

  var dlg = document.getElementById('video-modal');
  var frame = document.getElementById('video-frame');
  var titleEl = document.getElementById('video-modal-title');
  var openLink = document.getElementById('video-open');
  if (!dlg || !frame || !titleEl || !openLink) return;

  var canModal = typeof dlg.showModal === 'function';
  var lastFocus = null;
  var currentID = '';

  function lock(on) {
    document.documentElement.classList.toggle('is-locked', on);
    document.body.classList.toggle('is-locked', on);
  }

  function mount(id, title) {
    var f = document.createElement('iframe');
    f.src = 'https://www.youtube-nocookie.com/embed/' + encodeURIComponent(id) +
      '?rel=0&modestbranding=1&playsinline=1&hl=es&cc_lang_pref=es';
    f.title = title;
    f.loading = 'eager';
    f.setAttribute('allowfullscreen', '');
    f.setAttribute('allow', 'autoplay; encrypted-media; picture-in-picture; fullscreen');
    f.setAttribute('referrerpolicy', 'strict-origin-when-cross-origin');
    frame.appendChild(f);
  }

  function unmount() {
    // Sacar el iframe del DOM corta el video y libera el player.
    while (frame.firstChild) frame.removeChild(frame.firstChild);
    lock(false);
    currentID = '';
    if (lastFocus && typeof lastFocus.focus === 'function') lastFocus.focus();
    lastFocus = null;
  }

  function open(id, title) {
    currentID = id;
    titleEl.textContent = title || 'Video';
    openLink.href = 'https://www.youtube.com/watch?v=' + encodeURIComponent(id);
    if (canModal) {
      dlg.showModal();
    } else {
      // Fallback para navegadores sin <dialog> modal: mismo look, ESC manual.
      dlg.setAttribute('open', '');
      dlg.classList.add('is-fallback');
      document.addEventListener('keydown', legacyEscape);
    }
    mount(id, title || 'Video');
    lock(true);
  }

  function close() {
    if (!dlg.open) return;
    if (canModal) {
      dlg.close();
    } else {
      dlg.removeAttribute('open');
      dlg.classList.remove('is-fallback');
      document.removeEventListener('keydown', legacyEscape);
      unmount();
    }
  }

  function legacyEscape(ev) {
    if (ev.key === 'Escape' || ev.key === 'Esc') close();
  }

  // Abrir: delegación, así funciona con los 13 botones sin listeners propios.
  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest && ev.target.closest('[data-video]');
    if (!btn) return;
    ev.preventDefault();
    lastFocus = btn;
    open(btn.getAttribute('data-video'), btn.getAttribute('data-title'));
  });

  dlg.addEventListener('click', function (ev) {
    if (ev.target === dlg) close(); // click en el backdrop
  });

  var closeBtn = dlg.querySelector('[data-video-close]');
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

  dlg.addEventListener('close', unmount);
})();
