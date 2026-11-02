package store

// schema es el DDL completo. Se ejecuta en cada arranque con IF NOT EXISTS,
// así que es idempotente y sirve como migración inicial.
const schema = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS profile (
	id                INTEGER PRIMARY KEY CHECK (id = 1),
	name              TEXT    NOT NULL,
	age               INTEGER NOT NULL,
	height_cm         REAL    NOT NULL,
	weight_kg         REAL    NOT NULL,
	target_weight_min REAL    NOT NULL DEFAULT 0,
	target_weight_max REAL    NOT NULL DEFAULT 0,
	kcal_min          REAL    NOT NULL DEFAULT 0,
	kcal_max          REAL    NOT NULL DEFAULT 0,
	protein_min       REAL    NOT NULL DEFAULT 0,
	protein_max       REAL    NOT NULL DEFAULT 0,
	carb_target       REAL    NOT NULL DEFAULT 0,
	fat_target        REAL    NOT NULL DEFAULT 0,
	notes             TEXT    NOT NULL DEFAULT '',
	updated_at        TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS foods (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	name          TEXT NOT NULL UNIQUE,
	kcal100       REAL NOT NULL DEFAULT 0,
	protein100    REAL NOT NULL DEFAULT 0,
	fat100        REAL NOT NULL DEFAULT 0,
	carbs100      REAL NOT NULL DEFAULT 0,
	fiber100      REAL NOT NULL DEFAULT 0,
	sugar100      REAL NOT NULL DEFAULT 0,
	sodium_mg100  REAL NOT NULL DEFAULT 0,
	source        TEXT NOT NULL DEFAULT '',
	source_detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_foods_name ON foods(name);

CREATE TABLE IF NOT EXISTS diary_entries (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	day        TEXT    NOT NULL,
	meal       TEXT    NOT NULL,
	food_id    INTEGER REFERENCES foods(id) ON DELETE SET NULL,
	food_name  TEXT    NOT NULL,
	grams      REAL    NOT NULL,
	kcal       REAL    NOT NULL DEFAULT 0,
	protein    REAL    NOT NULL DEFAULT 0,
	fat        REAL    NOT NULL DEFAULT 0,
	carbs      REAL    NOT NULL DEFAULT 0,
	fiber      REAL    NOT NULL DEFAULT 0,
	sugar      REAL    NOT NULL DEFAULT 0,
	sodium     REAL    NOT NULL DEFAULT 0,
	note       TEXT    NOT NULL DEFAULT '',
	created_at TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_diary_day  ON diary_entries(day);
CREATE INDEX IF NOT EXISTS idx_diary_meal ON diary_entries(day, meal);

CREATE TABLE IF NOT EXISTS weights (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	day        TEXT NOT NULL UNIQUE,
	kg         REAL NOT NULL,
	note       TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_weights_day ON weights(day);

CREATE TABLE IF NOT EXISTS routine_exercises (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	day           TEXT    NOT NULL,
	ord           INTEGER NOT NULL,
	name          TEXT    NOT NULL,
	sets          INTEGER NOT NULL,
	reps          TEXT    NOT NULL,
	rir           TEXT    NOT NULL DEFAULT '',
	rest_sec      INTEGER NOT NULL DEFAULT 60,
	notes         TEXT    NOT NULL DEFAULT '',
	youtube_query TEXT    NOT NULL DEFAULT '',
	UNIQUE(day, ord)
);

CREATE TABLE IF NOT EXISTS workouts (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	day          TEXT    NOT NULL UNIQUE,
	routine_day  TEXT    NOT NULL,
	notes        TEXT    NOT NULL DEFAULT '',
	duration_min INTEGER NOT NULL DEFAULT 0,
	created_at   TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS workout_sets (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	workout_id  INTEGER NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
	exercise_id INTEGER NOT NULL REFERENCES routine_exercises(id),
	set_number  INTEGER NOT NULL,
	weight_kg   REAL,
	reps        INTEGER,
	rir         INTEGER,
	created_at  TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sets_workout ON workout_sets(workout_id);

CREATE TABLE IF NOT EXISTS meal_plan (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	meal        TEXT    NOT NULL,
	meal_time   TEXT    NOT NULL DEFAULT '',
	option      TEXT    NOT NULL,
	description TEXT    NOT NULL,
	kcal        INTEGER NOT NULL DEFAULT 0,
	protein     INTEGER NOT NULL DEFAULT 0,
	ord         INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS plan_notes (
	id   INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT    NOT NULL,
	text TEXT    NOT NULL,
	ord  INTEGER NOT NULL DEFAULT 0
);
`
