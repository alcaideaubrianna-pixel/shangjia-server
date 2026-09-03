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
