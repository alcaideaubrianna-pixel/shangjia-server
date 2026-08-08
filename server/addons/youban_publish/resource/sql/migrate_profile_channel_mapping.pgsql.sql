CREATE TABLE IF NOT EXISTS "hg_youban_publish_profile_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_profile_channel" UNIQUE ("tenant_id","profile_id","channel_id")
);
ALTER TABLE "hg_youban_publish_profile_channel" ADD COLUMN IF NOT EXISTS "is_manual" smallint NOT NULL DEFAULT 1;
UPDATE "hg_youban_publish_profile_channel" SET "is_manual" = 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_channel_owner" ON "hg_youban_publish_profile_channel" ("tenant_id","account_id","profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_channel_channel" ON "hg_youban_publish_profile_channel" ("tenant_id","channel_id","profile_id");

INSERT INTO "hg_youban_publish_profile_channel" ("tenant_id","account_id","profile_id","channel_id","is_manual","created_at","updated_at")
SELECT ps."tenant_id",ps."account_id",ps."profile_id",jsonb_array_elements_text(CASE WHEN ps."channel_id_json" ~ '^\s*\[.*\]\s*$' THEN ps."channel_id_json"::jsonb ELSE '[]'::jsonb END)::bigint,0,NOW(),NOW()
FROM "hg_youban_publish_profile_state" ps
WHERE ps."channel_id_json" IS NOT NULL AND ps."channel_id_json" <> '' AND ps."deleted_at" IS NULL
ON CONFLICT ("tenant_id","profile_id","channel_id") DO NOTHING;

ALTER TABLE "hg_youban_publish_profile_state" DROP COLUMN IF EXISTS "channel_id_json";
