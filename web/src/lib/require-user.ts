import { getServerSession } from "next-auth";
import { authOptions } from "@/auth";

// userId de la sesión o null. Toda ruta privada lo exige: sin esto, 401.
export async function requireUser(): Promise<string | null> {
  const session = await getServerSession(authOptions);
  return session?.user?.id ?? null;
}
