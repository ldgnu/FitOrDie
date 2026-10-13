import { NextResponse } from "next/server";
import { mkdir, writeFile } from "fs/promises";
import { join } from "path";
import { randomUUID } from "crypto";
import { eq, isNull, or } from "drizzle-orm";
import { db } from "@/db";
import { foods, mealPhotos, recipes } from "@/db/schema";
import { requireUser } from "@/lib/require-user";
import { identificarFoto, visionDisponible } from "@/lib/vision";

const MAX_MB = 8;
const EXT: Record<string, string> = {
  "image/jpeg": "jpg",
  "image/png": "png",
  "image/webp": "webp",
  "image/heic": "heic",
  "image/heif": "heif",
};

function uploadsDir(): string {
  return process.env.UPLOADS_DIR ?? "./uploads";
}

// POST /api/diary/foto (multipart: photo) → guarda + identifica (si hay visión).
// Nunca auto-guarda en el diario: devuelve items para confirmar.
export async function POST(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let form: FormData;
  try {
    form = await req.formData();
  } catch {
    return NextResponse.json({ error: "Multipart inválido" }, { status: 400 });
  }
  const file = form.get("photo");
  if (!file || typeof (file as File).arrayBuffer !== "function")
    return NextResponse.json({ error: "Falta la foto" }, { status: 400 });
  const f = file as File;
  if (!f.type.startsWith("image/"))
    return NextResponse.json({ error: "No es imagen" }, { status: 400 });
  if (f.size <= 0 || f.size > MAX_MB * 1024 * 1024)
    return NextResponse.json({ error: `Máximo ${MAX_MB} MB` }, { status: 413 });

  const ext = EXT[f.type] ?? "bin";
  const filename = `${randomUUID()}.${ext}`;
  const dir = uploadsDir();
  await mkdir(dir, { recursive: true });
  const buf = Buffer.from(await f.arrayBuffer());
  await writeFile(join(dir, filename), buf);

  const ins = await db
    .insert(mealPhotos)
    .values({ userId, filename, mime: f.type, size: f.size })
    .returning({ id: mealPhotos.id });
  const photoId = ins[0].id;

  if (!visionDisponible()) {
    return NextResponse.json(
      { photoId, vision: false, items: [], questions: ["Visión no configurada: describí la foto o cargala manual."] },
      { status: 201 },
    );
  }

  try {
    const foodRows = await db
      .select({ id: foods.id, name: foods.name })
      .from(foods)
      .where(or(isNull(foods.createdBy), eq(foods.createdBy, userId)));
    const recRows = await db
      .select({ id: recipes.id, title: recipes.title })
      .from(recipes)
      .where(eq(recipes.isPublic, true));
    const refs = [
      ...foodRows.map((x) => ({ id: x.id, name: x.name, kind: "food" as const })),
      ...recRows.map((x) => ({ id: x.id, name: x.title, kind: "recipe" as const })),
    ];
    const { items, questions } = await identificarFoto(buf.toString("base64"), f.type, refs);
    return NextResponse.json({ photoId, vision: true, items, questions }, { status: 201 });
  } catch (e) {
    console.error("vision fallo:", e instanceof Error ? e.message : e);
    return NextResponse.json(
      {
        photoId, vision: true, items: [],
        questions: ["No pude leer la foto. Describila con texto o cargala manual."],
      },
      { status: 201 },
    );
  }
}

// PATCH /api/diary/foto {photoId, entryId} — vincula la foto a un registro.
export async function PATCH(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  let body: { photoId?: string; entryId?: string };
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  if (!body.photoId || !body.entryId)
    return NextResponse.json({ error: "Faltan ids" }, { status: 400 });

  const [p] = await db.select().from(mealPhotos).where(eq(mealPhotos.id, body.photoId)).limit(1);
  if (!p || p.userId !== userId)
    return NextResponse.json({ error: "No encontrada" }, { status: 404 });
  await db.update(mealPhotos).set({ entryId: body.entryId }).where(eq(mealPhotos.id, body.photoId));
  return NextResponse.json({ ok: true });
}
