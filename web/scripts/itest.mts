// Test de integración E2E contra app corriendo (default http://localhost:3000).
// Cubre: público, auth, diario CRUD, aislamiento entre usuarios, peso,
// validación, sesiones, auditoría. Corre con: npx tsx scripts/itest.mts
import postgres from "postgres";

const BASE = process.env.BASE_URL ?? "http://localhost:3000";
const stamp = Date.now();
const A = { email: `a${stamp}@t.local`, password: "password-larga-1", name: "A" };
const B = { email: `b${stamp}@t.local`, password: "password-larga-2", name: "B" };

let pass = 0;
let fail = 0;
function check(name: string, cond: boolean, extra?: unknown) {
  if (cond) {
    pass++;
    console.log(`ok ${name}`);
  } else {
    fail++;
    console.log(`FAIL ${name}${extra !== undefined ? ` :: ${JSON.stringify(extra)}` : ""}`);
  }
}

function jar(setCookies: string[]): string {
  return setCookies.map((c) => c.split(";")[0]).join("; ");
}

async function login(email: string, password: string): Promise<string> {
  const csrfRes = await fetch(`${BASE}/api/auth/csrf`);
  const csrf = await csrfRes.json();
  const csrfCookies = jar(csrfRes.headers.getSetCookie?.() ?? []);
  const r = await fetch(`${BASE}/api/auth/callback/credentials`, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Cookie: csrfCookies,
    },
    body: new URLSearchParams({
      csrfToken: csrf.csrfToken,
      email,
      password,
      callbackUrl: `${BASE}/dashboard`,
      json: "true",
    }),
    redirect: "manual",
  });
  const cookies = r.headers.getSetCookie?.() ?? [];
  return jar(cookies);
}

const J = { "Content-Type": "application/json" };
const day = new Date().toISOString().slice(0, 10);

// 1. Público
check("landing 200", (await fetch(`${BASE}/`)).status === 200);
const dashNoAuth = await fetch(`${BASE}/dashboard`, { redirect: "manual" });
check("dashboard sin auth redirige a login", [307, 308].includes(dashNoAuth.status));
const progNoAuth = await fetch(`${BASE}/progreso`, { redirect: "manual" });
check("progreso sin auth redirige a login", [307, 308].includes(progNoAuth.status));
check("diary sin auth 401", (await fetch(`${BASE}/api/diary`)).status === 401);
check(
  "diary POST sin auth 401",
  (await fetch(`${BASE}/api/diary`, { method: "POST", headers: J, body: "{}" })).status === 401,
);

// 2. Registro
let r = await fetch(`${BASE}/api/auth/register`, {
  method: "POST", headers: J, body: JSON.stringify(A),
});
check("register A 201", r.status === 201, r.status);
r = await fetch(`${BASE}/api/auth/register`, {
  method: "POST", headers: J, body: JSON.stringify(A),
});
check("register duplicado 409", r.status === 409, r.status);
r = await fetch(`${BASE}/api/auth/register`, {
  method: "POST", headers: J, body: JSON.stringify({ email: "x@t.local", password: "corta" }),
});
check("register password corta 400", r.status === 400, r.status);
r = await fetch(`${BASE}/api/auth/register`, {
  method: "POST", headers: J, body: JSON.stringify(B),
});
check("register B 201", r.status === 201, r.status);

// 3. Login + sesión
const cA = await login(A.email, A.password);
check("login A da cookie", cA.includes("next-auth.session-token"));
const cB = await login(B.email, B.password);
check("login B da cookie", cB.includes("next-auth.session-token"));
const bad = await login(A.email, "password-incorrecta");
check("login malo no da sesión", !bad.includes("next-auth.session-token"));

// 4. Diario CRUD (A)
const H = (c: string) => ({ ...J, Cookie: c });
r = await fetch(`${BASE}/api/diary?day=${day}`, { headers: { Cookie: cA } });
let d = await r.json();
check("diary vacío", r.status === 200 && d.entries.length === 0);

r = await fetch(`${BASE}/api/diary`, {
  method: "POST", headers: H(cA),
  body: JSON.stringify({ consumedAt: new Date().toISOString(), meal: "almuerzo", customName: "Yogur", qty: 200, unit: "g" }),
});
const created = await r.json();
check("diary create 201", r.status === 201 && !!created.id, created);
const entryId = created.id as string;

r = await fetch(`${BASE}/api/diary`, {
  method: "POST", headers: H(cA),
  body: JSON.stringify({ consumedAt: new Date().toISOString(), meal: "no-existe", qty: 1, unit: "g", customName: "X" }),
});
check("diary meal inválida 400", r.status === 400, r.status);
r = await fetch(`${BASE}/api/diary`, {
  method: "POST", headers: H(cA),
  body: JSON.stringify({ consumedAt: new Date().toISOString(), meal: "cena", qty: -5, unit: "g", customName: "X" }),
});
check("diary qty negativa 400", r.status === 400, r.status);

r = await fetch(`${BASE}/api/diary?day=${day}`, { headers: { Cookie: cA } });
d = await r.json();
check("diary lista 1 + parcial (custom sin datos)", d.entries.length === 1 && d.partial === true, d);

// con alimento real del seed: pollo 200 g → 330 kcal
const fr = await (await fetch(`${BASE}/api/foods?q=pollo`, { headers: { Cookie: cA } })).json();
const pollo = fr.foods?.find((f: { name: string }) => f.name === "Pollo pechuga");
check("seed trae Pollo pechuga", !!pollo, fr.foods?.map((f: { name: string }) => f.name));
if (pollo) {
  r = await fetch(`${BASE}/api/diary`, {
    method: "POST", headers: H(cA),
    body: JSON.stringify({ consumedAt: new Date().toISOString(), meal: "cena", foodId: pollo.id, qty: 200, unit: "g" }),
  });
  check("diary create con food 201", r.status === 201, r.status);
  r = await fetch(`${BASE}/api/diary?day=${day}`, { headers: { Cookie: cA } });
  d = await r.json();
  check("totales incluyen pollo 330 kcal", d.totals.kcal === 330 && d.totals.protein === 62, d.totals);
  check("keto verdict calculado", typeof d.keto?.verdict === "string", d.keto);
}

// 5. PATCH + historial + aislamiento
r = await fetch(`${BASE}/api/diary/${entryId}`, {
  method: "PATCH", headers: H(cA), body: JSON.stringify({ qty: 250 }),
});
check("PATCH propio ok", r.status === 200, r.status);
r = await fetch(`${BASE}/api/diary/${entryId}`, {
  method: "PATCH", headers: H(cB), body: JSON.stringify({ qty: 1 }),
});
check("PATCH ajeno 404", r.status === 404, r.status);
r = await fetch(`${BASE}/api/diary/${entryId}`, { method: "DELETE", headers: { Cookie: cB } });
check("DELETE ajeno 404", r.status === 404, r.status);
r = await fetch(`${BASE}/api/diary?day=${day}`, { headers: { Cookie: cB } });
d = await r.json();
check("B no ve entradas de A", d.entries.length === 0, d.entries.length);
r = await fetch(`${BASE}/api/diary/${entryId}`, { method: "DELETE", headers: { Cookie: cA } });
check("DELETE propio ok", r.status === 200, r.status);

// 6. Peso
r = await fetch(`${BASE}/api/weight`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ weightKg: 80.5 }),
});
check("weight create 201", r.status === 201, r.status);
r = await fetch(`${BASE}/api/weight`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ weightKg: 5 }),
});
check("weight absurdo 400", r.status === 400, r.status);
r = await fetch(`${BASE}/api/weight`, { headers: { Cookie: cA } });
d = await r.json();
check("weight lista 1", d.weights.length === 1, d.weights.length);

// 7. Recetario + perfil + semana (A)
r = await fetch(`${BASE}/api/recipes`);
let rec = await r.json();
check("recipes público", r.status === 200 && rec.recipes.length >= 6, rec.recipes?.length);
r = await fetch(`${BASE}/api/recipes?keto=1`);
rec = await r.json();
check("recipes keto=1 todos compatibles", rec.recipes.length > 0 && rec.recipes.every((x: { verdict: string }) => x.verdict === "compatible"));
const tortilla = (await (await fetch(`${BASE}/api/recipes?q=tortilla`)).json()).recipes[0];
check("recipes search tortilla", !!tortilla, tortilla?.title);
r = await fetch(`${BASE}/api/recipes/${tortilla.id}`);
const det = await r.json();
check("recipe detail con ingredientes y porción", r.status === 200 && det.ingredients.length >= 2 && det.perPortion.kcal > 0, det);
r = await fetch(`${BASE}/api/diary`, {
  method: "POST", headers: H(cA),
  body: JSON.stringify({ consumedAt: new Date().toISOString(), meal: "cena", recipeId: tortilla.id, qty: 1, unit: "porción" }),
});
check("diary create desde receta 201", r.status === 201, r.status);
r = await fetch(`${BASE}/api/profile`, { headers: { Cookie: cA } });
check("profile propio 200", r.status === 200, r.status);
r = await fetch(`${BASE}/api/profile`, {
  method: "PATCH", headers: H(cA),
  body: JSON.stringify({ kcalTarget: 2300, crudeza: "sin-filtro", dislikes: ["apio"] }),
});
let prof = await r.json();
check("profile PATCH ok", r.status === 200 && prof.profile.kcalTarget === 2300 && prof.profile.crudeza === "sin-filtro", prof.profile);
r = await fetch(`${BASE}/api/profile`, {
  method: "PATCH", headers: H(cA), body: JSON.stringify({ kcalTarget: 50 }),
});
check("profile objetivo absurdo 400", r.status === 400, r.status);
r = await fetch(`${BASE}/api/profile`, {
  method: "PATCH", headers: H(cA),
  body: JSON.stringify({ weightKg: 89.65, heightCm: 185, age: 38, sex: "m", actividad: "moderado", objetivo: "bajar" }),
});
prof = await r.json();
check("profile cuerpo ok", r.status === 200 && prof.profile.heightCm === 185, prof.profile);
r = await fetch(`${BASE}/api/profile`, {
  method: "PATCH", headers: H(cA), body: JSON.stringify({ weightKg: 5 }),
});
check("profile peso absurdo 400", r.status === 400, r.status);
r = await fetch(`${BASE}/api/profile`);
check("profile sin auth 401", r.status === 401, r.status);
r = await fetch(`${BASE}/api/diary/week`, { headers: { Cookie: cA } });
const week = await r.json();
check("week 7 días con hoy cargado", r.status === 200 && week.days.length === 7 && week.days[6].entries >= 1, week.days?.map((x: { entries: number }) => x.entries));
r = await fetch(`${BASE}/api/logros`, { headers: { Cookie: cA } });
const lg = await r.json();
check(
  "logros 200 + xp + primer-registro",
  r.status === 200 && lg.xp > 0 && lg.logros.find((x: { key: string; desbloqueado: boolean }) => x.key === "primer-registro")?.desbloqueado,
  { xp: lg.xp, racha: lg.racha },
);
r = await fetch(`${BASE}/api/logros`);
check("logros sin auth 401", r.status === 401, r.status);

// 9. Votos + favoritos + categoría (A)
const tortillaId = (await (await fetch(`${BASE}/api/recipes?q=tortilla`)).json()).recipes[0]?.id;
r = await fetch(`${BASE}/api/recipes?categoria=desayuno`);
const des = await r.json();
check("filtro categoria desayuno", r.status === 200 && des.recipes.length > 0 && des.recipes.every((x: { categoria: string }) => x.categoria === "desayuno"), des.recipes?.length);
r = await fetch(`${BASE}/api/recipes/${tortillaId}/voto`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ vote: 1 }),
});
let vt = await r.json();
check("voto me gusta", r.status === 200 && vt.up === 1 && vt.mine === 1, vt);
r = await fetch(`${BASE}/api/recipes/${tortillaId}/voto`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ vote: -1 }),
});
vt = await r.json();
check("voto cambia a no me gusta", vt.up === 0 && vt.down === 1, vt);
r = await fetch(`${BASE}/api/recipes/${tortillaId}/voto`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ vote: 0 }),
});
vt = await r.json();
check("voto se quita", vt.up === 0 && vt.down === 0 && vt.mine === 0, vt);
r = await fetch(`${BASE}/api/recipes/${tortillaId}/voto`, {
  method: "POST", headers: J, body: JSON.stringify({ vote: 1 }),
});
check("voto sin auth 401", r.status === 401, r.status);
r = await fetch(`${BASE}/api/favoritos`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ kind: "recipe", itemId: tortillaId }),
});
check("fav add 201", r.status === 201, r.status);
r = await fetch(`${BASE}/api/favoritos?kind=recipe`, { headers: { Cookie: cA } });
const favs = await r.json();
check("fav lista 1", favs.favoritos.length === 1, favs.favoritos?.length);
r = await fetch(`${BASE}/api/favoritos?kind=recipe&itemId=${tortillaId}`, { method: "DELETE", headers: { Cookie: cA } });
check("fav delete ok", r.status === 200, r.status);

// 7b. Parse de texto libre (A)
r = await fetch(`${BASE}/api/diary/parse`, {
  method: "POST", headers: H(cA),
  body: JSON.stringify({ text: "hoy desayune 4 huevos con una palta chica echa aderezo con aceite" }),
});
const pr = await r.json();
check("parse 3 items + desayuno", r.status === 200 && pr.meal === "desayuno" && pr.items.length === 3, pr);
check("parse huevo 200g", pr.items[0]?.qty === 200 && pr.items[0]?.unit === "g", pr.items[0]);
check("parse palta 105g", pr.items[1]?.qty === 105, pr.items[1]);
check("parse aceite asumido", (pr.items[2]?.assumed?.length ?? 0) > 0, pr.items[2]);
r = await fetch(`${BASE}/api/diary/parse`, {
  method: "POST", headers: H(cA), body: JSON.stringify({ text: "   " }),
});
check("parse vacío 400", r.status === 400, r.status);
r = await fetch(`${BASE}/api/diary/parse`, {
  method: "POST", headers: H(cA),
  body: JSON.stringify({ text: "4 huevos revueltos\n60 gr de queso cremoso\nmate" }),
});
const pm = await r.json();
check("parse multilinea 3 items", r.status === 200 && pm.items.length === 3, pm.items?.length);
check(
  "parse queso 60g + mate 500ml",
  pm.items[1]?.qty === 60 && pm.items[2]?.qty === 500 && pm.items[2]?.unit === "ml",
  pm.items,
);

// 7c. Foto: PNG 1x1 con auth → 201 + photoId; sin auth → 401
const png = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  "base64",
);
let fd = new FormData();
fd.append("photo", new Blob([png], { type: "image/png" }), "test.png");
r = await fetch(`${BASE}/api/diary/foto`, { method: "POST", headers: { Cookie: cA }, body: fd });
const frFoto = await r.json().catch(() => ({}));
check("foto upload 201 + photoId", r.status === 201 && !!frFoto.photoId, { status: r.status });
fd = new FormData();
fd.append("photo", new Blob([png], { type: "image/png" }), "t.png");
r = await fetch(`${BASE}/api/diary/foto`, { method: "POST", body: fd });
check("foto sin auth 401", r.status === 401, r.status);
if (frFoto.photoId) {
  r = await fetch(`${BASE}/api/fotos/${frFoto.photoId}`, { headers: { Cookie: cB } });
  check("foto ajena 404", r.status === 404, r.status);
}

// 8. Auditoría en DB (dev): ediciones + deletes + registros quedaron trazados
const sql = postgres(process.env.DATABASE_URL!);
const edits = await sql`select count(*)::int as n from entry_edits where entry_id = ${entryId}`;
const audits = await sql`select action, count(*)::int as n from audit_log group by action`;
await sql.end();
check("entry_edits tiene el PATCH", edits[0].n >= 1, edits);
const kinds: Record<string, number> = {};
for (const a of audits) kinds[String(a.action)] = Number(a.n);
check("audit: register x2 + diary-delete x1", kinds.register >= 2 && kinds["diary-delete"] >= 1, kinds);

console.log(`\n${pass} ok, ${fail} fallos`);
process.exit(fail > 0 ? 1 : 0);
