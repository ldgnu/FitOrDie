export { default } from "next-auth/middleware";

// Gate en servidor: sin sesión no hay dashboard ni diario.
// Nada de esconder botones: esto redirige a /login.
export const config = {
  matcher: ["/dashboard/:path*", "/diario/:path*"],
};
