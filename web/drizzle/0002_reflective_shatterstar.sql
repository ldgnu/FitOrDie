CREATE TABLE "meal_photos" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"user_id" uuid NOT NULL,
	"entry_id" uuid,
	"filename" text NOT NULL,
	"mime" text NOT NULL,
	"size" integer NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "meal_photos" ADD CONSTRAINT "meal_photos_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "meal_photos" ADD CONSTRAINT "meal_photos_entry_id_diary_entries_id_fk" FOREIGN KEY ("entry_id") REFERENCES "public"."diary_entries"("id") ON DELETE set null ON UPDATE no action;