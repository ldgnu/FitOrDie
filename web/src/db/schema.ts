// Schema FitOrDie — Drizzle + Postgres.
// Todo lo personal cuelga de users.id. UTC en DB, zona del usuario en UI.
// Tablas users/accounts/sessions/verification_tokens: forma que exige Auth.js.
import {
  boolean,
  integer,
  jsonb,
  numeric,
  pgTable,
  primaryKey,
  text,
  timestamp,
  uuid,
} from "drizzle-orm/pg-core";

// ---------- Auth ----------
export const users = pgTable("users", {
  id: uuid("id").defaultRandom().primaryKey(),
  name: text("name"),
  email: text("email").unique(),
  emailVerified: timestamp("email_verified", { withTimezone: true }),
  image: text("image"),
  passwordHash: text("password_hash"), // solo credentials; null si OAuth
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

export const accounts = pgTable(
  "accounts",
  {
    userId: uuid("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    type: text("type").notNull(),
    provider: text("provider").notNull(),
    providerAccountId: text("provider_account_id").notNull(),
    refresh_token: text("refresh_token"),
    access_token: text("access_token"),
    expires_at: integer("expires_at"),
    token_type: text("token_type"),
    scope: text("scope"),
    id_token: text("id_token"),
    session_state: text("session_state"),
  },
  (t) => [primaryKey({ columns: [t.provider, t.providerAccountId] })],
);

export const sessions = pgTable("sessions", {
  sessionToken: text("session_token").primaryKey(),
  userId: uuid("user_id")
    .notNull()
    .references(() => users.id, { onDelete: "cascade" }),
  expires: timestamp("expires", { withTimezone: true }).notNull(),
});

export const verificationTokens = pgTable(
  "verification_tokens",
  {
    identifier: text("identifier").notNull(),
    token: text("token").notNull(),
    expires: timestamp("expires", { withTimezone: true }).notNull(),
  },
  (t) => [primaryKey({ columns: [t.identifier, t.token] })],
);

// ---------- Perfil ----------
export const profiles = pgTable("profiles", {
  userId: uuid("user_id")
    .primaryKey()
    .references(() => users.id, { onDelete: "cascade" }),
  displayName: text("display_name"),
  units: text("units").default("metric").notNull(), // metric | imperial
  timezone: text("timezone").default("America/Argentina/Buenos_Aires").notNull(),
  crudeza: text("crudeza").default("picante").notNull(), // suave | picante | sin-filtro
  kcalTarget: integer("kcal_target"),
  proteinTarget: integer("protein_target"), // g/día
  netCarbTarget: integer("net_carb_target").default(30), // g/día
  prefs: text("prefs").array(), // keto, vegetariana, sin-gluten...
  allergies: text("allergies").array(),
  dislikes: text("dislikes").array(),
  notes: text("notes"), // objetivos, horarios, contexto libre
});

// ---------- Alimentos y recetas ----------
export const foods = pgTable("foods", {
  id: uuid("id").defaultRandom().primaryKey(),
  name: text("name").notNull(),
  // verificado | etiqueta | manual | receta ; null en nutrientes = sin datos
  source: text("source").default("manual").notNull(),
  sourceDetail: text("source_detail"),
  kcal100: numeric("kcal_100"),
  protein100: numeric("protein_100"),
  fat100: numeric("fat_100"),
  carbs100: numeric("carbs_100"),
  fiber100: numeric("fiber_100"),
  createdBy: uuid("created_by").references(() => users.id), // null = global
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

export const recipes = pgTable("recipes", {
  id: uuid("id").defaultRandom().primaryKey(),
  title: text("title").notNull(),
  description: text("description"),
  servings: integer("servings").notNull().default(1),
  minutes: integer("minutes"),
  difficulty: text("difficulty"), // facil | media | dificil
  instructions: text("instructions").notNull(),
  photoUrl: text("photo_url"),
  storageTip: text("storage_tip"),
  isPublic: boolean("is_public").default(true).notNull(),
  createdBy: uuid("created_by").references(() => users.id),
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

export const recipeIngredients = pgTable("recipe_ingredients", {
  id: uuid("id").defaultRandom().primaryKey(),
  recipeId: uuid("recipe_id")
    .notNull()
    .references(() => recipes.id, { onDelete: "cascade" }),
  foodId: uuid("food_id").references(() => foods.id),
  freeText: text("free_text"), // si no hay food cargado
  qty: numeric("qty").notNull(),
  unit: text("unit").notNull(), // g | ml | unidad | taza ...
  substitution: text("substitution"),
});

// ---------- Diario ----------
export const diaryEntries = pgTable("diary_entries", {
  id: uuid("id").defaultRandom().primaryKey(),
  userId: uuid("user_id")
    .notNull()
    .references(() => users.id, { onDelete: "cascade" }),
  consumedAt: timestamp("consumed_at", { withTimezone: true }).notNull(),
  meal: text("meal").notNull(), // desayuno | media-manana | almuerzo | merienda | cena | snack
  foodId: uuid("food_id").references(() => foods.id),
  recipeId: uuid("recipe_id").references(() => recipes.id),
  customName: text("custom_name"),
  qty: numeric("qty").notNull(),
  unit: text("unit").notNull(),
  notes: text("notes"),
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
  deletedAt: timestamp("deleted_at", { withTimezone: true }), // soft delete
});

export const entryEdits = pgTable("entry_edits", {
  id: uuid("id").defaultRandom().primaryKey(),
  entryId: uuid("entry_id")
    .notNull()
    .references(() => diaryEntries.id, { onDelete: "cascade" }),
  changedBy: uuid("changed_by")
    .notNull()
    .references(() => users.id),
  diff: jsonb("diff").notNull(), // {campo: [antes, despues]}
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

// ---------- Cuerpo ----------
export const weights = pgTable("weights", {
  id: uuid("id").defaultRandom().primaryKey(),
  userId: uuid("user_id")
    .notNull()
    .references(() => users.id, { onDelete: "cascade" }),
  measuredAt: timestamp("measured_at", { withTimezone: true }).notNull(),
  weightKg: numeric("weight_kg").notNull(),
  note: text("note"),
});

export const measurements = pgTable("measurements", {
  id: uuid("id").defaultRandom().primaryKey(),
  userId: uuid("user_id")
    .notNull()
    .references(() => users.id, { onDelete: "cascade" }),
  measuredAt: timestamp("measured_at", { withTimezone: true }).notNull(),
  waistCm: numeric("waist_cm"),
  hipCm: numeric("hip_cm"),
  chestCm: numeric("chest_cm"),
  armCm: numeric("arm_cm"),
  thighCm: numeric("thigh_cm"),
  note: text("note"),
});

// ---------- Varios ----------
export const favorites = pgTable("favorites", {
  id: uuid("id").defaultRandom().primaryKey(),
  userId: uuid("user_id")
    .notNull()
    .references(() => users.id, { onDelete: "cascade" }),
  kind: text("kind").notNull(), // food | recipe
  itemId: uuid("item_id").notNull(),
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

export const telegramLinks = pgTable("telegram_links", {
  userId: uuid("user_id")
    .primaryKey()
    .references(() => users.id, { onDelete: "cascade" }),
  telegramId: text("telegram_id").unique().notNull(),
  secretHash: text("secret_hash").notNull(),
  verifiedAt: timestamp("verified_at", { withTimezone: true }),
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

export const idempotencyKeys = pgTable("idempotency_keys", {
  key: text("key").primaryKey(),
  userId: uuid("user_id")
    .notNull()
    .references(() => users.id, { onDelete: "cascade" }),
  response: jsonb("response"),
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});

export const auditLog = pgTable("audit_log", {
  id: uuid("id").defaultRandom().primaryKey(),
  userId: uuid("user_id").references(() => users.id, { onDelete: "set null" }),
  action: text("action").notNull(), // login | export | delete-account | ...
  meta: jsonb("meta"),
  createdAt: timestamp("created_at", { withTimezone: true })
    .defaultNow()
    .notNull(),
});
