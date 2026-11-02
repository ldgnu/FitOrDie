/* ==========================================================================
   FitOrDie — demo.js
   Interactividad del PROTOTIPO estático. No es el JS de producción:
   en Go, la verdad la tiene el servidor (SQLite) y esto se reemplaza por
   HTMX/vanilla contra los endpoints documentados en README.md.

   Lo que sí es real acá:
     - el cálculo de macros por gramos (kcal/100 g × g/100), igual que el motor Go
     - el parser local determinista de texto libre (sin IA, sin inventar macros)
     - los targets, las medidas caseras y los estados de UI
   ========================================================================== */
(function () {
  "use strict";

  var STORE_KEY = "fitordie:demo:v1";
  var SLOTS = [
    { id: "desayuno", name: "Desayuno", time: "7:30" },
    { id: "media", name: "Media mañana", time: "11:00" },
    { id: "almuerzo", name: "Almuerzo", time: "13:30" },
    { id: "merienda", name: "Merienda", time: "16:30" },
    { id: "cena", name: "Cena", time: "21:00" },
    { id: "picoteo", name: "Picoteo", time: "—" }
  ];

  /* Medidas caseras: gramos por unidad. Sin dato → se usa 100 g y se avisa. */
  var MEASURES = {
    "Huevo": [["unidad", 50]],
    "Banana": [["unidad", 120]],
    "Manzana": [["unidad", 180]],
    "Palta": [["unidad", 150]],
    "Tomate": [["unidad", 120]],
    "Pan blanco": [["rodaja", 30]],
    "Queso cremoso": [["cucharada", 15], ["feta", 30]],
    "Queso crema": [["cucharada", 15]],
    "Queso duro": [["feta", 30]],
    "Aceite de oliva": [["cucharada", 10]],
    "Avena": [["cucharada", 8], ["taza", 80]],
    "Arroz blanco cocido": [["taza", 158]],
    "Fideos secos": [["taza", 75]],
    "Leche entera": [["vaso", 200]],
    "Yogur griego sin azúcar": [["pote", 170]],
    "Yogur natural": [["pote", 170]],
    "Nueces": [["puñado", 20]],
    "Pollo pechuga": [["filete", 150]],
    "Carne vacuna magra": [["bife", 150]],
    "Atún al natural escurrido": [["lata", 120]],
    "Merluza": [["filete", 150]],
    "Papa": [["unidad", 200]],
    "Café negro": [["taza", 200]],
    "Mate": [["taza", 200]]
  };

  var TARGET = { kcal: 2150, kcalMax: 2400, p: 155, pMax: 170 };

  /* ------------------------------------------------------------------ utils */
  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  }
  function fold(s) {
    return String(s || "")
      .toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
      .trim();
  }
  function nf(n, d) {
    d = d === undefined ? 0 : d;
    if (n === null || n === undefined || isNaN(n)) return "—";
    return Number(n).toLocaleString("es-AR", {
      minimumFractionDigits: d,
      maximumFractionDigits: d
    });
  }
  function r0(n) { return Math.round(Number(n) || 0); }
  function r1(n) { return Math.round((Number(n) || 0) * 10) / 10; }
  function clamp(n, a, b) { return Math.min(b, Math.max(a, n)); }
  function pct(v, t) { return t > 0 ? clamp((v / t) * 100, 0, 100) : 0; }
  function uid() { return "e" + Date.now().toString(36) + Math.random().toString(36).slice(2, 6); }
  function slotById(id) {
    for (var i = 0; i < SLOTS.length; i++) if (SLOTS[i].id === id) return SLOTS[i];
    return SLOTS[0];
  }

  /* ------------------------------------------------------------------ store */
  var state = {
    theme: null,
    entries: [],
    checks: {},
    weights: [],
    activeSlot: "almuerzo",
    day: "A"
  };
  var seed = { entries: [], weights: [] };

  function readJSON(id) {
    var node = document.getElementById(id);
    if (!node) return null;
    try {
      return JSON.parse(node.textContent);
    } catch (e) {
      return null;
    }
  }

  function loadStore() {
    var stored = null;
    try {
      stored = JSON.parse(localStorage.getItem(STORE_KEY) || "null");
    } catch (e) {
      stored = null;
    }
    if (stored && typeof stored === "object") {
      if (Array.isArray(stored.entries)) state.entries = stored.entries;
      if (stored.checks && typeof stored.checks === "object") state.checks = stored.checks;
      if (Array.isArray(stored.weights)) state.weights = stored.weights;
      if (stored.theme) state.theme = stored.theme;
      if (stored.activeSlot) state.activeSlot = stored.activeSlot;
    }
    if (!state.entries.length) state.entries = seed.entries.slice();
    if (!state.weights.length) state.weights = (seed.weights || []).slice();
  }

  function saveStore() {
    try {
      localStorage.setItem(
        STORE_KEY,
        JSON.stringify({
          entries: state.entries,
          checks: state.checks,
          weights: state.weights,
          theme: state.theme,
          activeSlot: state.activeSlot
        })
      );
    } catch (e) {
      /* modo privado o cuota llena: el demo sigue andando en memoria */
    }
  }

  /* ----------------------------------------------------------------- macros */
  function entryMacros(entry) {
    var f = entry._food || null;
    if (entry.kcal !== undefined && entry.kcal !== null) {
      return {
        kcal: Number(entry.kcal),
        p: Number(entry.p) || 0,
        c: Number(entry.c) || 0,
        f: Number(entry.f) || 0,
        fiber: Number(entry.fiber) || 0
      };
    }
    if (!f) return { kcal: 0, p: 0, c: 0, f: 0, fiber: 0 };
    var k = (Number(entry.grams) || 0) / 100;
    return {
      kcal: (f.kcal || 0) * k,
      p: (f.p || 0) * k,
      c: (f.c || 0) * k,
      f: (f.fat || 0) * k,
      fiber: (f.fiber || 0) * k
    };
  }

  function totals(entries) {
    var t = { kcal: 0, p: 0, c: 0, f: 0, fiber: 0 };
    entries.forEach(function (e) {
      var m = entryMacros(e);
      t.kcal += m.kcal;
      t.p += m.p;
      t.c += m.c;
      t.f += m.f;
      t.fiber += m.fiber;
    });
    return t;
  }

  /* ------------------------------------------------------------------ toast */
  var toastTimer = null;
  function toast(msg) {
    var node = $("#toast");
    if (!node) return;
    node.textContent = msg;
    node.classList.add("is-on");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      node.classList.remove("is-on");
    }, 2600);
  }

  /* ------------------------------------------------------------------ sheet */
  var lastFocus = null;
  function openSheet(id) {
    var sheet = document.getElementById(id);
    if (!sheet) return;
    lastFocus = document.activeElement;
    sheet.classList.add("is-open");
    sheet.removeAttribute("aria-hidden");
    document.body.style.overflow = "hidden";
    var first = sheet.querySelector("input, textarea, select, button");
    if (first) setTimeout(function () { first.focus(); }, 60);
  }
  function closeSheet(sheet) {
    if (!sheet) return;
    sheet.classList.remove("is-open");
    sheet.setAttribute("aria-hidden", "true");
    document.body.style.overflow = "";
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  }
  function initSheets() {
    $$(".sheet").forEach(function (sheet) {
      sheet.setAttribute("aria-hidden", "true");
      sheet.addEventListener("click", function (ev) {
        if (ev.target === sheet) closeSheet(sheet);
      });
      $$("[data-close]", sheet).forEach(function (btn) {
        btn.addEventListener("click", function () { closeSheet(sheet); });
      });
    });
    document.addEventListener("keydown", function (ev) {
      if (ev.key === "Escape") {
        var open = $(".sheet.is-open");
        if (open) closeSheet(open);
      }
    });
  }

  /* ------------------------------------------------------------------ tema */
  function applyTheme(theme, persist) {
    var root = document.documentElement;
    if (theme === "light" || theme === "dark") root.setAttribute("data-theme", theme);
    else root.removeAttribute("data-theme");
    state.theme = theme === "light" || theme === "dark" ? theme : null;
    var meta = document.querySelector('meta[name="theme-color"]');
    var dark = getComputedStyle(document.body).backgroundColor;
    if (meta && dark) meta.setAttribute("content", dark);
    $$("[data-theme-choice]").forEach(function (btn) {
      btn.setAttribute("aria-pressed", String((btn.getAttribute("data-theme-choice") || "system") === (theme || "system")));
    });
    if (persist) saveStore();
  }
  function initTheme() {
    applyTheme(state.theme || null, false);
    $$("[data-theme-toggle]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var isLight = document.documentElement.getAttribute("data-theme") === "light";
        var next = isLight ? "dark" : "light";
        applyTheme(next, true);
        toast(next === "light" ? "Tema claro" : "Tema oscuro");
      });
    });
    $$("[data-theme-choice]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        applyTheme(btn.getAttribute("data-theme-choice"), true);
      });
    });
  }

  /* ======================================================================
     PANTALLA: HOY
     ====================================================================== */
  function renderHoy() {
    var t = totals(state.entries);
    var kcalLeft = TARGET.kcal - t.kcal;
    var targetP = TARGET.pMax;

    var ring = $("#ring");
    if (ring) {
      ring.style.setProperty("--pct", String(Math.min(100, pct(t.kcal, TARGET.kcal)).toFixed(1)));
      ring.classList.toggle("is-over", t.kcal > TARGET.kcalMax);
      var rn = ring.querySelector(".ring__num");
      var rl = ring.querySelector(".ring__label");
      if (rn) rn.textContent = nf(r0(t.kcal));
      if (rl) rl.textContent = "de " + nf(TARGET.kcal) + " kcal";
    }

    var kcalEl = $("#hero-kcal");
    if (kcalEl) {
      kcalEl.innerHTML =
        (kcalLeft >= 0 ? nf(r0(kcalLeft)) : "−" + nf(r0(-kcalLeft))) +
        " <small>kcal " + (kcalLeft >= 0 ? "disponibles" : "de más") + "</small>";
    }
    var stateEl = $("#hero-state");
    if (stateEl) {
      stateEl.textContent =
        "Llevás " + nf(r0(t.kcal)) + " kcal · proteína " + nf(r0(t.p)) + " de " + TARGET.pMax + " g";
    }

    ["p", "c", "f"].forEach(function (k) {
      var bar = $("#bar-" + k);
      var val = $("#val-" + k);
      var tot = k === "p" ? t.p : k === "c" ? t.c : t.f;
      var tgt = k === "p" ? targetP : k === "c" ? 190 : 155;
      if (bar) {
        bar.style.setProperty("--v", pct(tot, tgt).toFixed(1) + "%");
        if (tot > tgt * 1.05) bar.classList.add("is-over");
        else bar.classList.remove("is-over");
      }
      if (val)
        val.innerHTML =
          nf(r0(tot)) + ' / <span class="macro__target">' + nf(tgt) + " g</span>";
    });

    var list = $("#meal-list");
    if (list) {
      var html = "";
      SLOTS.forEach(function (slot) {
        var items = state.entries.filter(function (e) { return e.slot === slot.id; });
        var sub = items.map(function (e) { return e.name; }).join(" · ");
        var st = totals(items);
        html +=
          '<li class="meal' + (items.length ? "" : " meal__empty") + '">' +
          '<div class="meal__time">' + slot.time + "</div>" +
          '<div style="min-width:0">' +
          '<div class="meal__name">' + (items.length ? slot.name : slot.name) +
          (items.length ? "" : ' <span class="badge">sin cargar</span>') +
          "</div>" +
          '<div class="meal__items">' + (items.length ? sub : "Tocá + para anotar") + "</div>" +
          "</div>" +
          '<div style="display:flex;align-items:center;gap:8px">' +
          '<div class="meal__cal">' + (items.length ? nf(r0(st.kcal)) : "—") +
          "<small>" + (items.length ? "P " + nf(r0(st.p)) + " g" : "kcal") + "</small></div>" +
          '<a class="iconbtn iconbtn--plain" href="comidas.html?slot=' + slot.id + '" aria-label="Cargar en ' + slot.name + '">' +
          '<svg class="i"><use href="#i-plus"/></svg></a>' +
          "</div></li>";
      });
      list.innerHTML = html;
    }
  }

  /* ======================================================================
     PANTALLA: COMIDAS
     ====================================================================== */
  var foods = [];

  function loadFoods() {
    var data = readJSON("foods-data");
    foods = (data && data.foods) || [];
    foods.forEach(function (f) {
      f._key = fold(f.name);
    });
  }

  function measuresFor(food) {
    return MEASURES[food.name] || [];
  }

  function entryFromFood(food, qty, unit) {
    var m = measuresFor(food).filter(function (x) { return x[0] === unit; })[0];
    var grams = unit === "g" ? qty : (m ? m[1] * qty : qty);
    var k = grams / 100;
    return {
      id: uid(),
      slot: state.activeSlot,
      name: food.name,
      foodId: food.id,
      qty: qty,
      unit: unit,
      grams: r1(grams),
      kcal: r1((food.kcal || 0) * k),
      p: r1((food.p || 0) * k),
      c: r1((food.c || 0) * k),
      f: r1((food.fat || 0) * k),
      fiber: r1((food.fiber || 0) * k)
    };
  }

  function renderDay() {
    var wrap = $("#day-list");
    var t = totals(state.entries);
    if (wrap) {
      var html = "";
      SLOTS.forEach(function (slot) {
        var items = state.entries.filter(function (e) { return e.slot === slot.id; });
        if (!items.length) return;
        var st = totals(items);
        html +=
          '<div class="hist__week" style="padding-top:16px">' + slot.name +
          '<span class="badge">' + nf(r0(st.kcal)) + " kcal</span></div>";
        items.forEach(function (e) {
          html +=
            '<div class="entry" data-entry="' + e.id + '">' +
            '<div style="min-width:0">' +
            '<div class="entry__name truncate">' + e.name + "</div>" +
            '<div class="entry__qty">' + nf(e.qty) + " " + e.unit + " · " + nf(e.grams) + " g</div>" +
            "</div>" +
            '<div class="entry__macros">' + nf(r0(e.kcal)) + " kcal<br>P " + nf(r0(e.p)) + " · C " + nf(r0(e.c)) + " · G " + nf(r0(e.f)) + "</div>" +
            '<button class="iconbtn iconbtn--plain" data-del="' + e.id + '" aria-label="Borrar ' + e.name + '">' +
            '<svg class="i i--sm"><use href="#i-trash"/></svg></button>' +
            "</div>";
        });
      });
      if (!html) {
        html =
          '<div class="empty"><svg class="i i--lg"><use href="#i-bowl"/></svg>' +
          "Todavía no cargaste nada hoy. Buscá un alimento arriba o estimá por texto.</div>";
      }
      wrap.innerHTML = html;
      $$("[data-del]", wrap).forEach(function (btn) {
        btn.addEventListener("click", function () {
          var id = btn.getAttribute("data-del");
          state.entries = state.entries.filter(function (e) { return e.id !== id; });
          saveStore();
          renderDay();
          toast("Entrada borrada");
        });
      });
    }

    var barKcal = $("#daybar-kcal");
    if (barKcal) barKcal.innerHTML = nf(r0(t.kcal)) + ' <small>/ ' + nf(TARGET.kcal) + " kcal</small>";
    var barP = $("#daybar-p");
    if (barP) barP.textContent = "P " + nf(r0(t.p)) + " g";
    var barC = $("#daybar-c");
    if (barC) barC.textContent = "C " + nf(r0(t.c)) + " g";
    var barF = $("#daybar-f");
    if (barF) barF.textContent = "G " + nf(r0(t.f)) + " g";

    var pbar = $("#daybar-bar");
    if (pbar) pbar.style.setProperty("--v", pct(t.kcal, TARGET.kcal).toFixed(1) + "%");
  }

  function renderResults(query) {
    var box = $("#results");
    if (!box) return;
    var q = fold(query);
    var out = q
      ? foods.filter(function (f) { return f._key.indexOf(q) !== -1; })
      : foods.slice(0, 8);
    if (!out.length) {
      box.innerHTML =
        '<div class="empty">Sin datos para «' + query + '». No inventamos valores: ' +
        "probá con otro nombre o cargá el alimento a mano.</div>";
      return;
    }
    var html = "";
    out.slice(0, 40).forEach(function (f) {
      html +=
        '<button class="food" data-food="' + f.name.replace(/"/g, "&quot;") + '">' +
        '<span style="min-width:0;flex:1">' +
        '<span class="food__name">' + f.name + "</span>" +
        '<span class="food__meta">' + nf(f.kcal) + " kcal · P " + nf(f.p, 1) + " · C " + nf(f.c, 1) + " · G " + nf(f.fat, 1) + " /100 g</span>" +
        "</span>" +
        '<span class="food__cal">' + nf(f.kcal) + '<br><span class="text-xs mute">kcal</span></span>' +
        '<span class="iconbtn iconbtn--plain" aria-hidden="true"><svg class="i"><use href="#i-plus"/></svg></span>' +
        "</button>";
    });
    box.innerHTML = html;
    $$("[data-food]", box).forEach(function (btn) {
      btn.addEventListener("click", function () {
        var name = btn.getAttribute("data-food");
        var food = foods.filter(function (f) { return f.name === name; })[0];
        if (food) openFoodEditor(food);
      });
    });
  }

  var editing = null;

  function openFoodEditor(food) {
    editing = { food: food, qty: 100, unit: "g" };
    var sheet = $("#sheet-food");
    if (!sheet) return;
    $("#ef-name").textContent = food.name;
    $("#ef-meta").textContent =
      nf(food.kcal) + " kcal · P " + nf(food.p, 1) + " · C " + nf(food.c, 1) + " · G " + nf(food.fat, 1) + " por 100 g";

    var measures = measuresFor(food);
    var sel = $("#ef-unit");
    var opts = '<option value="g">gramos (g)</option>';
    measures.forEach(function (m) {
      opts += '<option value="' + m[0] + '">' + m[0] + " (" + m[1] + " g)</option>";
    });
    sel.innerHTML = opts;
    sel.value = "g";

    $("#ef-qty").value = "100";
    $("#ef-hint").textContent = "Pesá crudo, como indica el plan.";
    updateEditor();
    $("#ef-slot").textContent = slotById(state.activeSlot).name;
    openSheet("sheet-food");
  }

  function updateEditor() {
    if (!editing) return;
    var food = editing.food;
    var qty = parseFloat($("#ef-qty").value.replace(",", ".")) || 0;
    var unit = $("#ef-unit").value;
    var m = measuresFor(food).filter(function (x) { return x[0] === unit; })[0];
    var grams = unit === "g" ? qty : m ? m[1] * qty : qty;
    editing.qty = qty;
    editing.unit = unit;
    editing.grams = grams;

    var k = grams / 100;
    $("#ef-kcal").textContent = nf(r0((food.kcal || 0) * k));
    $("#ef-p").textContent = nf(r1((food.p || 0) * k));
    $("#ef-c").textContent = nf(r1((food.c || 0) * k));
    $("#ef-g").textContent = nf(r1((food.fat || 0) * k));
    $("#ef-grams").textContent = unit === "g" ? "" : "= " + nf(grams) + " g";
  }

  function initComidas() {
    loadFoods();

    var params = new URLSearchParams(location.search);
    var slotParam = params.get("slot");
    if (slotParam && SLOTS.some(function (s) { return s.id === slotParam; })) {
      state.activeSlot = slotParam;
    }

    var search = $("#search");
    if (search) {
      search.addEventListener("input", function () { renderResults(search.value); });
      search.addEventListener("keydown", function (ev) {
        if (ev.key === "Enter") {
          ev.preventDefault();
          var first = $("#results [data-food]");
          if (first) first.click();
        }
      });
    }
    renderResults("");

    $$("[data-slot]").forEach(function (btn) {
      btn.setAttribute("aria-pressed", String(btn.getAttribute("data-slot") === state.activeSlot));
      btn.addEventListener("click", function () {
        state.activeSlot = btn.getAttribute("data-slot");
        saveStore();
        $$("[data-slot]").forEach(function (b) {
          b.setAttribute("aria-pressed", String(b.getAttribute("data-slot") === state.activeSlot));
        });
        var lbl = $("#target-slot");
        if (lbl) lbl.textContent = slotById(state.activeSlot).name;
        toast("Cargando en " + slotById(state.activeSlot).name);
      });
    });
    var lbl0 = $("#target-slot");
    if (lbl0) lbl0.textContent = slotById(state.activeSlot).name;

    var qty = $("#ef-qty");
    if (qty) {
      qty.addEventListener("input", updateEditor);
      var unit = $("#ef-unit");
      if (unit) {
        unit.addEventListener("change", function () {
          /* Al cambiar de medida se conservan los gramos: 100 g -> 2 unidades de 50 g.
             Si venías de gramos, la cantidad se recalcula; nunca queda "250 filetes". */
          var prev = editing && editing.grams ? editing.grams : null;
          var f = 1;
          if (unit.value !== "g") {
            var m = measuresFor(editing.food).filter(function (x) { return x[0] === unit.value; })[0];
            f = m ? m[1] : 1;
          }
          if (prev) qty.value = String(unit.value === "g" ? r1(prev) : Math.max(1, Math.round(prev / f)));
          updateEditor();
        });
      }
      $$("[data-step]", $("#sheet-food")).forEach(function (btn) {
        btn.addEventListener("click", function () {
          var step = parseFloat(btn.getAttribute("data-step")) || 0;
          /* en gramos el paso es de 10; en medidas caseras, de a una unidad */
          if (unit && unit.value !== "g") step = step > 0 ? 1 : -1;
          var v = (parseFloat(qty.value.replace(",", ".")) || 0) + step;
          qty.value = String(Math.max(0, Math.round(v * 10) / 10));
          updateEditor();
        });
      });
      var add = $("#ef-add");
      if (add) {
        add.addEventListener("click", function () {
          if (!editing || !(editing.grams > 0)) {
            toast("Poné una cantidad mayor a 0");
            return;
          }
          var entry = entryFromFood(editing.food, editing.qty, editing.unit);
          state.entries.push(entry);
          saveStore();
          closeSheet($("#sheet-food"));
          renderDay();
          toast("Sumado a " + slotById(entry.slot).name + ": " + entry.name);
        });
      }
    }

    $("#open-ai") && $("#open-ai").addEventListener("click", function () { openSheet("sheet-ai"); });
    var est = $("#ai-estimate");
    if (est) est.addEventListener("click", runAIEstimate);
    var addAll = $("#ai-add-all");
    if (addAll) addAll.addEventListener("click", addAIResults);

    /* Atajo desde Hoy: comidas.html#ia abre directo el estimador */
    if (location.hash === "#ia" && $("#sheet-ai")) {
      setTimeout(function () { openSheet("sheet-ai"); }, 120);
    }

    renderDay();
  }

  /* ---------------------------------------------- estimador de texto libre */
  /* Sinónimos de uso real: lo que Javi escribe vs. el nombre en la tabla.
     Si el alimento no está en la tabla, no se inventa: se avisa. */
  var SYNONYMS = {
    ensalada: "Lechuga",
    verduras: "Zapallitos",
    verdura: "Zapallitos",
    fideos: "Fideos secos",
    pasta: "Fideos secos",
    pollo: "Pollo pechuga",
    carne: "Carne vacuna magra",
    bife: "Bife de chorizo",
    arroz: "Arroz blanco cocido",
    huevo: "Huevo",
    huevos: "Huevo",
    yogur: "Yogur griego sin azúcar",
    pan: "Pan blanco",
    queso: "Queso cremoso",
    "frutos secos": "Nueces",
    aceite: "Aceite de oliva",
    atun: "Atún al natural escurrido",
    pescado: "Merluza",
    palta: "Palta",
    papa: "Papa",
    tomate: "Tomate",
    banana: "Banana",
    manzana: "Manzana",
    avena: "Avena",
    almendras: "Harina de almendra"
  };

  function foodByName(name) {
    for (var i = 0; i < foods.length; i++) if (foods[i].name === name) return foods[i];
    return null;
  }

  function matchFood(chunk) {
    var c = " " + fold(chunk) + " ";
    var best = null;
    var bestScore = 0;

    /* 1) sinónimo exacto como palabra completa */
    Object.keys(SYNONYMS).forEach(function (key) {
      if (!new RegExp("\\b" + key + "\\b").test(c)) return;
      var f = foodByName(SYNONYMS[key]);
      if (f) {
        var s = key.length + 10;
        if (s > bestScore) { bestScore = s; best = f; }
      }
    });

    /* 2) palabras significativas del nombre en la tabla ("pechuga", "griego"…) */
    foods.forEach(function (f) {
      var words = f._key.split(/\s+/).filter(function (w) { return w.length >= 4; });
      var score = 0;
      words.forEach(function (w) {
        if (new RegExp("\\b" + w + "(s|es)?\\b").test(c)) score += w.length;
      });
      if (score > bestScore) { bestScore = score; best = f; }
    });

    return best;
  }

  var aiParsed = [];
  var aiUnmatched = [];

  function parseText(text) {
    var clean = String(text || "")
      .toLowerCase()
      .replace(/\s+/g, " ")
      .trim();
    var out = { items: [], unmatched: [] };
    if (!clean) return out;

    var chunks = clean.split(/\s*(?:,|;|\+| con | y | más | mas )\s*/).filter(Boolean);
    var seen = {};

    chunks.forEach(function (chunk) {
      var numMatch = chunk.match(/(\d+(?:[.,]\d+)?)\s*(kg|kilos?|grs?|gr|g|gramos|ml|unidades?|unidad|tazas?|cucharadas?|cdas?|potes?|vasos?|rodajas?|filetes?|fetas?|puñados?|latas?|platos?)?/);
      var qty = numMatch && numMatch[1] ? parseFloat(numMatch[1].replace(",", ".")) : null;
      var unitRaw = numMatch && numMatch[2] ? numMatch[2] : null;

      var food = matchFood(chunk);
      if (!food) {
        if (chunk.replace(/[^a-záéíóúñ ]/g, "").trim().length > 2) out.unmatched.push(chunk.trim());
        return;
      }
      if (seen[food.name]) return;
      seen[food.name] = true;

      var unit = "g";
      var grams = 100;
      if (unitRaw) {
        if (unitRaw.charAt(0) === "k") {
          unit = "kg";
          grams = (qty || 0) * 1000;
        } else if (unitRaw.indexOf("ml") === 0) {
          unit = "ml";
          grams = qty || 0;
        } else if (unitRaw.charAt(0) === "g") {
          grams = qty || 100;
        } else {
          var wantUnit = unitRaw
            .replace(/es$/, "")
            .replace(/s$/, "")
            .replace(/^unidad$/, "unidad");
          var ms = measuresFor(food);
          var hit = ms.filter(function (m) {
            return m[0].indexOf(wantUnit) === 0 || wantUnit.indexOf(m[0]) === 0;
          })[0];
          unit = hit ? hit[0] : "unidad";
          grams = hit ? hit[1] * (qty || 1) : 100 * (qty || 1);
        }
      } else if (qty !== null) {
        /* número sin unidad: si es chico (≤20) se lee como cantidad de unidades
           ("3 huevos"), si es grande, como gramos ("200 arroz" → 200 g). */
        var m0 = measuresFor(food)[0];
        if (m0 && qty <= 20) {
          unit = m0[0];
          grams = m0[1] * qty;
        } else {
          grams = qty;
        }
      } else {
        var meas = measuresFor(food)[0];
        if (meas) {
          unit = meas[0];
          grams = meas[1];
          qty = 1;
        } else {
          qty = 100;
        }
      }

      var k = grams / 100;
      out.items.push({
        name: food.name,
        grams: r1(grams),
        qty: qty === null ? r1(grams) : qty,
        unit: unit,
        kcal: r1((food.kcal || 0) * k),
        p: r1((food.p || 0) * k),
        c: r1((food.c || 0) * k),
        f: r1((food.fat || 0) * k),
        fiber: r1((food.fiber || 0) * k)
      });
    });

    return out;
  }

  function renderAIResults(items, note, unmatched) {
    var box = $("#ai-results");
    if (!box) return;
    var html = "";

    if (!items.length) {
      html =
        '<div class="callout callout--warn"><svg class="i i--sm"><use href="#i-alert"/></svg>' +
        "<div><strong>Sin datos.</strong> No reconocí alimentos de la tabla local. " +
        "No inventamos macros: elegí los alimentos a mano en el buscador.</div></div>";
      box.innerHTML = html;
      return;
    }

    items.forEach(function (it, i) {
      html +=
        '<label class="setting" style="align-items:flex-start">' +
        '<span style="min-width:0">' +
        '<span class="setting__label truncate" style="display:block">' + it.name + "</span>" +
        '<span class="setting__sub">' + nf(it.qty) + " " + it.unit + " · " +
        nf(it.kcal) + " kcal · P " + nf(r0(it.p)) + " · C " + nf(r0(it.c)) + " · G " + nf(r0(it.f)) + "</span>" +
        "</span>" +
        '<span style="display:flex;align-items:center;gap:8px">' +
        '<input class="input" style="width:5rem;text-align:center" type="number" min="0" step="10" value="' +
        Math.round(it.grams) + '" data-ai-grams="' + i + '" aria-label="Gramos de ' + it.name + '">' +
        "</span></label>";
    });

    if (unmatched && unmatched.length) {
      html +=
        '<div class="callout callout--warn mt-3"><svg class="i i--sm"><use href="#i-alert"/></svg>' +
        "<div><strong>No están en la tabla:</strong> " + unmatched.join(", ") +
        ". Cargalos a mano o sumalos al CSV — no se estiman a ojo.</div></div>";
    }

    box.innerHTML = html;
    if (note) {
      var noteNode = $("#ai-note");
      if (noteNode) noteNode.textContent = note;
    }
    $$("[data-ai-grams]", box).forEach(function (inp) {
      inp.addEventListener("change", function () {
        var i = parseInt(inp.getAttribute("data-ai-grams"), 10);
        var g = parseFloat(inp.value) || 0;
        var it = aiParsed[i];
        if (!it) return;
        var factor = g / (it.grams || 1);
        it.grams = r1(g);
        it.kcal = r1(it.kcal * factor);
        it.p = r1(it.p * factor);
        it.c = r1(it.c * factor);
        it.f = r1(it.f * factor);
        it.qty = r1(g);
        it.unit = "g";
        renderAIResults(aiParsed, note, unmatched);
      });
    });
  }

  function runAIEstimate() {
    var text = $("#ai-text") ? $("#ai-text").value : "";
    var parsed = parseText(text);
    aiParsed = parsed.items;
    aiUnmatched = parsed.unmatched;
    var note =
      "Interpretación local (mock). Revisá gramos antes de sumar: los macros salen de la tabla, " +
      "la IA solo interpreta el texto.";
    renderAIResults(aiParsed, note, aiUnmatched);
  }

  function addAIResults() {
    if (!aiParsed.length) {
      toast("No hay ítems para sumar");
      return;
    }
    aiParsed.forEach(function (it) {
      state.entries.push({
        id: uid(),
        slot: state.activeSlot,
        name: it.name,
        qty: it.qty,
        unit: it.unit,
        grams: it.grams,
        kcal: it.kcal,
        p: it.p,
        c: it.c,
        f: it.f,
        fiber: it.fiber,
        estimated: true
      });
    });
    saveStore();
    renderDay();
    closeSheet($("#sheet-ai"));
    toast("Sumaste " + aiParsed.length + " ítem(s) a " + slotById(state.activeSlot).name);
    aiParsed = [];
  }

  /* ======================================================================
     PANTALLA: RUTINA
     ====================================================================== */
  function initRutina() {
    var tabs = $$("[data-daytab]");
    function showDay(day) {
      state.day = day;
      tabs.forEach(function (t) {
        t.setAttribute("aria-pressed", String(t.getAttribute("data-daytab") === day));
      });
      $$("[data-day]").forEach(function (sec) {
        sec.hidden = sec.getAttribute("data-day") !== day;
      });
    }
    tabs.forEach(function (t) {
      t.addEventListener("click", function () { showDay(t.getAttribute("data-daytab")); });
    });
    showDay(state.day || "A");

    var checks = $$("[data-check]");
    function paintCheck(btn) {
      var id = btn.getAttribute("data-check");
      var done = !!state.checks[id];
      btn.setAttribute("aria-pressed", String(done));
      var card = btn.closest(".ex");
      if (card) card.classList.toggle("is-done", done);
    }
    checks.forEach(function (btn) {
      paintCheck(btn);
      btn.addEventListener("click", function () {
        var id = btn.getAttribute("data-check");
        state.checks[id] = !state.checks[id];
        saveStore();
        paintCheck(btn);
        paintProgress();
        toast(state.checks[id] ? "Ejercicio completado" : "Ejercicio desmarcado");
      });
    });

    function paintProgress() {
      /* progreso por día: cada sección lleva su propia barra */
      $$("[data-day]").forEach(function (sec) {
        var boxes = $$("[data-check]", sec);
        var done = boxes.filter(function (b) {
          return !!state.checks[b.getAttribute("data-check")];
        }).length;
        var bar = $("[data-progress]", sec);
        var lbl = $("[data-progress-label]", sec);
        if (bar) {
          bar.style.setProperty("--v", boxes.length ? ((done / boxes.length) * 100).toFixed(1) + "%" : "0%");
        }
        if (lbl) lbl.textContent = done + " de " + boxes.length + " hechos";
      });
    }
    paintProgress();
  }

  /* ======================================================================
     PANTALLA: PROGRESO
     ====================================================================== */
  function initProgreso() {
    var form = $("#weight-form");
    if (!form) return;
    form.addEventListener("submit", function (ev) {
      ev.preventDefault();
      var inp = $("#weight-input");
      var kg = parseFloat((inp.value || "").replace(",", "."));
      if (!kg || kg < 30 || kg > 250) {
        toast("Peso inválido: entre 30 y 250 kg");
        inp.focus();
        return;
      }
      var today = new Date().toISOString().slice(0, 10);
      state.weights = state.weights.filter(function (w) { return w.date !== today; });
      state.weights.push({ date: today, kg: kg });
      saveStore();

      var big = $("#weight-now");
      if (big) big.innerHTML = nf(kg, 2).replace(",00", "") + " <small>kg</small>";
      var list = $("#hist-list");
      if (list) {
        /* la fila "Hoy" pre-renderizada se reemplaza por la nueva */
        $$(".hist__row", list).forEach(function (r) {
          var d = r.querySelector(".hist__date");
          if (d && d.textContent.indexOf("Hoy") === 0) r.remove();
        });
        var row = document.createElement("div");
        row.className = "hist__row";
        row.innerHTML =
          '<div class="hist__date">Hoy · ' + today.slice(8, 10) + "/" + today.slice(5, 7) + "</div>" +
          '<div class="badge badge--accent">nuevo</div>' +
          '<div class="hist__kg">' + nf(kg, 2) + " kg</div>";
        list.insertBefore(row, list.firstChild);
      }
      inp.value = "";
      toast("Peso guardado: " + nf(kg, 2) + " kg");
    });
  }

  /* ======================================================================
     PANTALLA: PERFIL
     ====================================================================== */
  function initPerfil() {
    $$("[data-mock]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        toast(btn.getAttribute("data-mock"));
      });
    });
    var pin = $("#pin-switch");
    if (pin) {
      pin.addEventListener("change", function () {
        toast(pin.checked ? "PIN: se activa desde el servidor (ver SECURITY.md)" : "PIN desactivado en el demo");
      });
    }
  }

  /* ------------------------------------------------------------------- init */
  function init() {
    seed.entries = (readJSON("seed-data") || {}).entries || [];
    seed.weights = (readJSON("seed-data") || {}).weights || [];
    loadStore();
    initTheme();
    initSheets();

    var page = document.body.getAttribute("data-page");
    if (page === "hoy") renderHoy();
    if (page === "comidas") initComidas();
    if (page === "rutina") initRutina();
    if (page === "progreso") initProgreso();
    if (page === "perfil") initPerfil();

    if ("serviceWorker" in navigator && location.protocol !== "file:") {
      window.addEventListener("load", function () {
        navigator.serviceWorker.register("sw.js").catch(function () {
          /* sin SW: la app igual funciona, solo pierde el modo offline */
        });
      });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
