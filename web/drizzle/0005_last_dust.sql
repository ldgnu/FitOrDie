CREATE TABLE "recipe_votes" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"user_id" uuid NOT NULL,
	"recipe_id" uuid NOT NULL,
	"vote" integer NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "categoria" text;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "categoria_auto" boolean DEFAULT true NOT NULL;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "video_url" text;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "source_url" text;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "source_site" text;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "source_likes" integer DEFAULT 0 NOT NULL;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "declared_kcal" numeric;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "declared_protein" numeric;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "declared_fat" numeric;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "declared_carbs" numeric;--> statement-breakpoint
ALTER TABLE "recipes" ADD COLUMN "declared_fiber" numeric;--> statement-breakpoint
ALTER TABLE "recipe_votes" ADD CONSTRAINT "recipe_votes_user_id_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "recipe_votes" ADD CONSTRAINT "recipe_votes_recipe_id_recipes_id_fk" FOREIGN KEY ("recipe_id") REFERENCES "public"."recipes"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "recipes" ADD CONSTRAINT "recipes_source_url_unique" UNIQUE("source_url");