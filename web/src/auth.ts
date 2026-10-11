import type { DefaultSession, NextAuthOptions } from "next-auth";
import Credentials from "next-auth/providers/credentials";
import { compare } from "bcryptjs";
import { eq } from "drizzle-orm";
import { db } from "./db";
import { users } from "./db/schema";

declare module "next-auth" {
  interface Session {
    user: { id: string } & DefaultSession["user"];
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    uid?: string;
  }
}

// Auth real v4: credentials (email+password) con sesiones JWT firmadas.
// Passwords solo como hash bcrypt. OAuth queda para después.
export const authOptions: NextAuthOptions = {
  session: { strategy: "jwt", maxAge: 30 * 24 * 60 * 60 },
  secret: process.env.NEXTAUTH_SECRET,
  pages: { signIn: "/login" },
  providers: [
    Credentials({
      credentials: {
        email: { label: "Email", type: "email" },
        password: { label: "Password", type: "password" },
      },
      async authorize(creds) {
        const email = String(creds?.email ?? "").toLowerCase().trim();
        const password = String(creds?.password ?? "");
        if (!email || !password) return null;
        const rows = await db
          .select()
          .from(users)
          .where(eq(users.email, email))
          .limit(1);
        const u = rows[0];
        if (!u?.passwordHash) return null;
        const ok = await compare(password, u.passwordHash);
        if (!ok) return null;
        return { id: u.id, email: u.email, name: u.name };
      },
    }),
  ],
  callbacks: {
    async jwt({ token, user }) {
      if (user?.id) token.uid = user.id;
      return token;
    },
    async session({ session, token }) {
      if (session.user) session.user.id = token.uid ?? token.sub ?? "";
      return session;
    },
  },
};
