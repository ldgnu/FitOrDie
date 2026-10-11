import { NextResponse } from "next/server";
import { and, ilike, isNull, or, eq } from "drizzle-orm";
import { db } from "@/db";
import { foods } from "@/db/schema";
import { requireUser } from "@/lib/require-user";

// GET /api/foods?q=pollo → alimentos globales + propios del usuario. Max 30.
export async function GET(req: Request) {
  const userId = await requireUser();
  if (!userId) return NextResponse.json({ error: "No autenticado" }, { status: 401 });

  const q = new URL(req.url).searchParams.get("q")?.trim() ?? "";
  const rows = await db
    .select()
    .from(foods)
    .where(
      and(
        or(isNull(foods.createdBy), eq(foods.createdBy, userId)),
        q ? ilike(foods.name, `%${q}%`) : undefined,
      ),
    )
    .limit(30);

  return NextResponse.json({ foods: rows });
}
