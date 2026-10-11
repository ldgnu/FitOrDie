# FitOrDie — Brief v2 (rearma)

## Concepto
Plataforma de seguimiento alimentario y cambio físico con una diferencia central:
**la app es tu testigo honesto**. No le podés mentir: si venís bien, te motiva.
Si te pasás o te escondés, te reta. Una sola voz, sin interruptores de personalidad.

## Un solo modo: el Testigo
La app tiene una única personalidad que reacciona a tu conducta con datos,
no a tu cuerpo. No hay que elegir ni configurar modos: el tono lo define
lo que hiciste, no un selector.

- **Venís bien** (registrás, dentro del objetivo): motiva. Corto y real.
  *"Bien. Mañana repetilo."*
- **Te pasás** (sobre el objetivo, patrón de acomodar números): reta, con la
  crudeza que configures. *"Esto no cierra. Anotá la verdad y seguí."*
- **Desaparecés** (días sin registrar): te acusa de esconderte.
  *"Dos días sin aparecer. El dato no muerde, vos sí te estás mintiendo."*
- **Volvés después de fallar**: ni fiesta ni sermón. *"Acá estás. Seguimos."*

Reglas de la voz:
- Habla de la **conducta** (registrar, cumplir, esconderse), nunca del cuerpo.
- Crudeza configurable en el perfil: suave / picante / sin filtro.
  Default: picante. El usuario define el nivel, la app define cuándo reta.
- Nunca prescribe daño (ver piso de seguridad).

## Principio "No le podés mentir" (accountability)
Mecánicas que hacen imposible el autoengaño sin convertir la app en policía:

1. **Racha de honestidad**: cuenta los días con registro completo, te hayas
   pasado o no. Registrar un mal día suma, no resta. Esconderte la resta.
2. **Historial de ediciones**: podés corregir cantidades, pero queda registro
   del cambio. Sin borrado silencioso (soft delete + auditoría).
3. **Resumen semanal verdad**: un número semanal sin maquillaje: días
   registrados, promedio calórico, veces sobre el objetivo. Sin adjetivos.
4. **El dashboard muestra, no juzga**: los números son los números. El tono
   (motivar o retar) sale de los datos, no de un modo elegido.

El derecho a exportar y eliminar la cuenta con sus datos se mantiene intacto
a nivel cuenta (obligación legal, no negociable).

## Disparadores del reto
El reto se dispara por **conducta medible**, nunca por el cuerpo:

- Superar el objetivo calórico diario en más del % configurado.
- Días consecutivos sin registrar (default: 2).
- Editar o borrar registros para "acomodar" números (patrón, no casos sueltos).
- Nunca por: peso, fotos, medidas ni un antojo aislado.

## Piso de seguridad (no se negocia)
- Sin recomendar déficits calóricos extremos.
- Sin prescripción médica ni diagnósticos.
- Sin incentivar vómitos, ayunos extremos ni conductas compensatorias.
- Valores nutricionales: estimaciones identificadas como tales, con fuente.
- Si un patrón sugiere trastorno alimentario, la app deriva a ayuda
  profesional y baja el tono. Esto está por encima de cualquier reto.

## Qué cambia del brief original
- **2. Identidad visual**: se mantiene (cálida, editorial). El tono vive en
  los textos, no en los colores.
- **Dashboard y recordatorios**: copy reactivo a la conducta del usuario.
- **Perfil**: suma selector de crudeza (suave / picante / sin filtro).
- **Modelo de datos**: suma `crudeza_level`, historial de ediciones de
  registros, racha de honestidad.
- **Resto**: sin cambios (arquitectura, seguridad, keto, recetario, Telegram).

## Plataforma (iPhone webapp + web)
- Mobile-first real: la experiencia primaria es el teléfono.
- PWA instalable (iPhone: Add to Home Screen), ícono propio, splash, offline
  mínimo para registrar comidas sin conexión y sincronizar después.
- Touch targets grandes, registro de comida en pocos toques, sin hover-dependencias.
- Desktop web completo, mismo backend y datos. Sin app nativa en el MVP.

## Estado
Brief v2.1 (modo único). Pendiente: fase 1 (inspección), plan de implementación.
