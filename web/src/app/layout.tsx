import type { Metadata, Viewport } from "next";
import { Fraunces, Inter } from "next/font/google";
import "./globals.css";
import Providers from "./providers";
import AppNav from "./app-nav";

const display = Fraunces({ variable: "--font-display", subsets: ["latin"] });
const body = Inter({ variable: "--font-body", subsets: ["latin"] });

export const metadata: Metadata = {
  title: "FitOrDie — tu testigo honesto",
  description:
    "Registrá qué comés, calculá nutrientes, evaluá keto y enterate la verdad. Diario alimentario y cambio físico, privado por defecto.",
  manifest: "/manifest.webmanifest",
};

export const viewport: Viewport = {
  themeColor: "#171310",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="es" className={`${display.variable} ${body.variable}`}>
      <body className="min-h-screen bg-crema font-body text-tinta antialiased">
        <Providers>
          {children}
          <AppNav />
        </Providers>
      </body>
    </html>
  );
}
