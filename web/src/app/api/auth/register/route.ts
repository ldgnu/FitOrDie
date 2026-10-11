import { NextResponse } from "next/server";
import { hash } from "bcryptjs";
import { eq } from "drizzle-orm";
import { db } from "@/db";
import { auditLog, profiles, users } from "@/db/schema";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

// POST /api/auth/register {email, password, name?} → 201 | 400 | 409
export async function POST(req: Request) {
  let body: { email?: string; password?: string; name?: string };
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "JSON inválido" }, { status: 400 });
  }
  const email = String(body.email ?? "").toLowerCase().trim();
  const password = String(body.password ?? "");
  const name = String(body.name ?? "").trim() || null;

  if (!EMAIL_RE.test(email))
    return NextResponse.json({ error: "Email inválido" }, { status: 400 });
  if (password.length < 10)
    return NextResponse.json(
      { error: "Password mínima de 10 caracteres" },
      { status: 400 },
    );

  const existing = await db
    .select({ id: users.id })
    .from(users)
    .where(eq(users.email, email))
    .limit(1);
  if (existing.length > 0)
    return NextResponse.json(
      { error: "Ese email ya está registrado" },
      { status: 409 },
    );

  const passwordHash = await hash(password, 12);
  const inserted = await db
    .insert(users)
    .values({ email, name, passwordHash })
    .returning({ id: users.id });
  const userId = inserted[0].id;
  await db.insert(profiles).values({ userId, displayName: name });
  await db
    .insert(auditLog)
    .values({ userId, action: "register", meta: { email } });

  return NextResponse.json({ ok: true }, { status: 201 });
}
