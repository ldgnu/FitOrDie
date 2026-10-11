import Link from "next/link";
import { getServerSession } from "next-auth";
import { authOptions } from "@/auth";

export default async function Home() {
  const session = await getServerSession(authOptions);

  return (
    <main className="mx-auto w-full max-w-2xl px-5 pb-16 pt-10">
      <p className="text-sm tracking-wide text-oliva">FITORDIE</p>
      <h1 className="mt-2 font-display text-4xl leading-tight">
        Tu alimentación, sin mentiras.
      </h1>
      <p className="mt-4 text-lg leading-relaxed">
        Registrá qué comés, calculá nutrientes y enterate la verdad: si venís
        bien, te motiva. Si te escondés, te reta. Sin humo en ninguna
        dirección.
      </p>

      <div className="mt-6 flex flex-col gap-3 sm:flex-row">
        {session?.user ? (
          <Link
            href="/dashboard"
            className="rounded bg-tinta px-5 py-3 text-center text-crema"
          >
            Entrar a mi día
          </Link>
        ) : (
          <>
            <Link
              href="/register"
              className="rounded bg-tinta px-5 py-3 text-center text-crema"
            >
              Crear cuenta
            </Link>
            <Link
              href="/login"
              className="rounded border border-tinta/30 px-5 py-3 text-center"
            >
              Entrar
            </Link>
          </>
        )}
      </div>

      <section className="mt-12">
        <h2 className="font-display text-2xl">Cómo funciona</h2>
        <ol className="mt-4 space-y-4">
          <li>
            <strong>1. Anotás.</strong> Qué comiste, cuánto y a qué hora. En
            segundos, desde el celu.
          </li>
          <li>
            <strong>2. Calcula.</strong> Calorías, proteínas, grasas, carbos y
            carbos netos. Con fuentes, sin inventar.
          </li>
          <li>
            <strong>3. Te dice la verdad.</strong> Compatible con tu objetivo
            o no. Y si no registrás, también te lo dice.
          </li>
        </ol>
      </section>

      <section className="mt-12">
        <h2 className="font-display text-2xl">Preguntas de verdad</h2>
        <div className="mt-4 space-y-4">
          <div>
            <strong>¿Es solo para keto?</strong>
            <p>
              No. Keto es una evaluación orientativa más. La app sirve para
              cualquier objetivo.
            </p>
          </div>
          <div>
            <strong>¿Mis datos son privados?</strong>
            <p>
              Sí. Tu diario, peso y fotos son privados por defecto. Nada se
              indexa ni se comparte.
            </p>
          </div>
          <div>
            <strong>¿Cuánto sale?</strong>
            <p>Nada, por ahora. Sin pagos ni publicidad en esta versión.</p>
          </div>
          <div>
            <strong>¿Esto es consejo médico?</strong>
            <p>
              No. Son estimaciones ajustables, no indicaciones médicas. Ante
              dudas, profesional de la salud.
            </p>
          </div>
        </div>
      </section>

      <footer className="mt-12 border-t border-tinta/15 pt-4 text-sm text-tinta/70">
        FitOrDie — comer bien puede ser práctico, rico y sostenible.
      </footer>
    </main>
  );
}
