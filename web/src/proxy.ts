import { getToken } from "next-auth/jwt";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

// Gate en servidor (Next 16: proxy): sin sesión no hay dashboard ni diario.
// La autorización real sigue en cada API (requireUser); esto solo redirige.
export async function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;
  if (
    pathname.startsWith("/dashboard") ||
    pathname.startsWith("/diario") ||
    pathname.startsWith("/perfil") ||
    pathname.startsWith("/progreso")
  ) {
    let token = null;
    try {
      token = await getToken({
        req,
        secret: process.env.NEXTAUTH_SECRET,
      });
    } catch {
      token = null; // header malformado = no autenticado, nunca 500
    }
    if (!token) {
      const url = new URL("/login", req.url);
      url.searchParams.set("callbackUrl", pathname);
      return NextResponse.redirect(url);
    }
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*", "/diario/:path*", "/perfil/:path*", "/progreso/:path*"],
};
