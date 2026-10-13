// Cálculo de objetivos desde el cuerpo. Puro y determinista.
// Mifflin-St Jeor + factor de actividad + meta. Da SUGERENCIAS que el
// usuario confirma; nunca prescribe. Piso: jamás sugiere bajo el TMB.
export type Sexo = "m" | "f";
export type Actividad = "sedentario" | "ligero" | "moderado" | "intenso";
export type Objetivo = "bajar" | "mantener" | "subir";

const FACTOR: Record<Actividad, number> = {
  sedentario: 1.2,
  ligero: 1.375,
  moderado: 1.55,
  intenso: 1.725,
};

export function tmb(sexo: Sexo, kg: number, cm: number, edad: number): number {
  const base = 10 * kg + 6.25 * cm - 5 * edad;
  return Math.round(sexo === "f" ? base - 161 : base + 5);
}

export function imc(kg: number, cm: number): number {
  const m = cm / 100;
  return Math.round((kg / (m * m)) * 10) / 10;
}

export function rangoSaludable(cm: number): { min: number; max: number } {
  const m = cm / 100;
  return { min: Math.round(18.5 * m * m), max: Math.round(24.9 * m * m) };
}

export interface Sugerencia {
  tmb: number;
  tdee: number;
  kcal: number;
  proteina: number;
  netos: number;
  grasa: number;
  imcActual: number;
}

export function sugerir(input: {
  sexo: Sexo;
  kg: number;
  cm: number;
  edad: number;
  actividad: Actividad;
  objetivo: Objetivo;
  keto: boolean;
}): Sugerencia {
  const t = tmb(input.sexo, input.kg, input.cm, input.edad);
  const tdee = Math.round(t * FACTOR[input.actividad]);
  const kcal =
    input.objetivo === "bajar"
      ? Math.max(Math.round((tdee * 0.8) / 50) * 50, t)
      : input.objetivo === "subir"
        ? Math.round((tdee * 1.1) / 50) * 50
        : Math.round(tdee / 50) * 50;
  const proteina =
    Math.round((input.kg * (input.objetivo === "mantener" ? 1.6 : 2.0)) / 5) * 5;
  const netos = input.keto ? 30 : 100;
  const grasa = Math.max(0, Math.round((kcal - proteina * 4 - netos * 4) / 9));
  return { tmb: t, tdee, kcal, proteina, netos, grasa, imcActual: imc(input.kg, input.cm) };
}
