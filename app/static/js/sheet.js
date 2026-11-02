/* Hoja de rutina del día: agregar y vaciar series sin recargar.
 *
 * Toda la hoja es un solo formulario que se manda completo (misma semántica
 * que POST /api/workout): agregar una fila es clonar el <template>, y borrar
 * una serie es vaciar la fila. Sin JS la página sigue funcionando: cada
 * ejercicio trae una fila vacía lista para cargar y el botón de guardar manda
 * todo igual.
 */
(function () {
  'use strict';

  var form = document.querySelector('form.sheet');
  if (!form) return;

  var tpl = document.getElementById('setrow-tpl');
  if (!tpl) return;

  function rowsOf(list) {
    return list.querySelectorAll('.setrow:not(.setrow--head)');
  }

  function nextNumber(list) {
    var n = 0;
    rowsOf(list).forEach(function (row) {
      if (!row.classList.contains('is-empty')) n++;
    });
    return n + 1;
  }

  function isEmpty(row) {
    return ['kg', 'reps', 'rir', 'note'].every(function (name) {
      var input = row.querySelector('input[name="' + name + '"]');
      return !input || input.value.trim() === '';
    });
  }

  function renumber(list) {
    var n = 1;
    rowsOf(list).forEach(function (row) {
      var label = row.querySelector('.setrow__n');
      if (!label) return;
      var empty = isEmpty(row);
      if (!empty) {
        label.textContent = '#' + n;
        n++;
      } else {
        label.textContent = '+';
      }
      row.classList.toggle('is-empty', empty);
    });
  }

  // Agregar serie: clona la fila modelo y le pone el ejercicio.
  form.addEventListener('click', function (ev) {
    var add = ev.target.closest('.js-addset');
    if (add) {
      var list = add.closest('.ex').querySelector('.sets');
      if (!list) return;
      var node = tpl.content.firstElementChild.cloneNode(true);
      var ex = node.querySelector('input[name="ex"]');
      if (ex) ex.value = add.getAttribute('data-ex') || '';
      list.appendChild(node);
      renumber(list);
      var first = node.querySelector('input[name="kg"]');
      if (first) first.focus();
      return;
    }

    // Vaciar (borrar) una serie: la fila queda vacía y no viaja al guardar.
    var clear = ev.target.closest('.js-clear');
    if (clear) {
      var row = clear.closest('.setrow');
      if (!row) return;
      ['kg', 'reps', 'rir', 'note'].forEach(function (name) {
        var input = row.querySelector('input[name="' + name + '"]');
        if (input) input.value = '';
      });
      renumber(row.closest('.sets'));
      var kg = row.querySelector('input[name="kg"]');
      if (kg) kg.focus();
    }
  });

  // Mantener la numeración al tipear (una fila con datos deja de ser "+").
  form.addEventListener('input', function (ev) {
    var row = ev.target.closest('.setrow');
    if (!row) return;
    renumber(row.closest('.sets'));
  });

  // Evitar que Enter en una fila mande el formulario sin querer.
  form.addEventListener('keydown', function (ev) {
    if (ev.key === 'Enter' && ev.target.tagName === 'INPUT' && ev.target.type !== 'submit') {
      ev.preventDefault();
    }
  });

  // Estado inicial: la fila vacía que vino del servidor se marca como tal.
  form.querySelectorAll('.sets').forEach(renumber);
})();
