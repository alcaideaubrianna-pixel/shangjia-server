-- Interactive upgrades must remain short. Historical cleanup and backfills belong in upgrade.pgsql.sql.
CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_dedupe_entry" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "layer" varchar(32) NOT NULL DEFAULT '',
  "signature" varchar(64) NOT NULL DEFAULT '',
  "item_total" integer NOT NULL DEFAULT 0,
  "signature_count" integer NOT NULL DEFAULT 0,
  "first_event_id" bigint NOT NULL DEFAULT 0,
  "last_event_id" bigint NOT NULL DEFAULT 0,
  "first_seen_at" timestamp DEFAULT NULL,
  "last_seen_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_dedupe_entry" ON "hg_youban_publish_collect_dedupe_entry" ("tenant_id", "account_id", "channel_id", "layer", "signature", "item_total", "signature_count");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dedupe_lookup" ON "hg_youban_publish_collect_dedupe_entry" ("tenant_id", "account_id", "layer", "signature", "channel_id", "last_seen_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_dedupe_source" (
  "id" BIGSERIAL PRIMARY KEY,
  "entry_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "accepted_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_dedupe_source" ON "hg_youban_publish_collect_dedupe_source" ("entry_id", "dispatch_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dedupe_source_owner" ON "hg_youban_publish_collect_dedupe_source" ("tenant_id", "account_id", "source_id", "entry_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dedupe_source_dispatch" ON "hg_youban_publish_collect_dedupe_source" ("dispatch_id");

-- A bound rule is source-local. Global rules are account-level delete/replace policies only.
UPDATE "hg_youban_publish_collect_rule" r
SET "global_enabled" = 0, "updated_at" = CURRENT_TIMESTAMP
WHERE r."global_enabled" = 1
  AND EXISTS (
    SELECT 1 FROM "hg_youban_publish_collect_source_rule" sr WHERE sr."rule_id" = r."id"
  );

DELETE FROM "hg_youban_publish_collect_source_rule" sr
USING "hg_youban_publish_collect_source_rule" keep
WHERE sr."source_id" = keep."source_id"
  AND (sr."sort", sr."id") > (keep."sort", keep."id");

CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_source_single_rule"
  ON "hg_youban_publish_collect_source_rule" ("source_id");
CREATE TABLE IF NOT EXISTS "hg_youban_publish_profile_fingerprint" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "layer" varchar(32) NOT NULL DEFAULT '',
  "signature" varchar(64) NOT NULL DEFAULT '',
  "item_total" integer NOT NULL DEFAULT 0,
  "signature_count" integer NOT NULL DEFAULT 0,
  "owner_marker" varchar(16) DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_profile_fingerprint_scope" ON "hg_youban_publish_profile_fingerprint" ("tenant_id", "account_id", "channel_id", "layer", "signature", "item_total", "signature_count", "owner_marker");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_profile_fingerprint_profile" ON "hg_youban_publish_profile_fingerprint" ("profile_id", "channel_id", "layer", "signature", "item_total", "signature_count");
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_fingerprint_profile" ON "hg_youban_publish_profile_fingerprint" ("profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_profile_profile" ON "hg_youban_publish_channel_profile" ("profile_id", "channel_id");
