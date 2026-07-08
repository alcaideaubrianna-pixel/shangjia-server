ALTER TABLE "hg_youban_publish_collect_rule"
  ADD COLUMN IF NOT EXISTS "full_match_enabled" smallint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_collect_rule"
  ADD COLUMN IF NOT EXISTS "delete_text_json" text;

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_enabled" smallint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_mode" varchar(32) NOT NULL DEFAULT 'recent_days';

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_days" integer NOT NULL DEFAULT 30;

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectEnabled');

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_history_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "mode" varchar(32) NOT NULL DEFAULT 'recent_days',
  "days" integer NOT NULL DEFAULT 30,
  "offset_id" integer NOT NULL DEFAULT 0,
  "scanned_count" integer NOT NULL DEFAULT 0,
  "event_count" integer NOT NULL DEFAULT 0,
  "duplicate_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "next_run_at" timestamp DEFAULT NULL,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_history_owner" ON "hg_youban_publish_collect_history_task" ("tenant_id", "account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_history_source" ON "hg_youban_publish_collect_history_task" ("source_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_history_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "level" varchar(16) NOT NULL DEFAULT 'info',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "meta_json" text,
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_history_log_task" ON "hg_youban_publish_collect_history_log" ("task_id", "id");
