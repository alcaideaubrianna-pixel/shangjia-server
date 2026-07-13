CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_tenant" ON "hg_youban_publish_tg_job_log" ("tenant_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_account" ON "hg_youban_publish_tg_job_log" ("tenant_id", "account_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_created" ON "hg_youban_publish_tg_job_log" ("created_at", "id");

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

ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_task_collect_order" ON "hg_youban_publish_task" ("collect_source_id", "collect_source_chat_id", "collect_source_message_id", "id");

ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_collect_order" ON "hg_youban_publish_tg_job" ("channel_id", "target_chat_id", "collect_source_id", "collect_source_chat_id", "collect_source_message_id", "status", "id");

UPDATE "hg_youban_publish_task" t SET
  "collect_event_id"=e."id",
  "collect_source_id"=e."source_id",
  "collect_source_chat_id"=e."source_chat_id",
  "collect_source_message_id"=e."source_message_id"
FROM "hg_youban_publish_collect_event" e
WHERE t."tenant_id"=e."tenant_id"
  AND t."account_id"=e."account_id"
  AND t."collect_source_message_id"=0
  AND t."client_request_id" LIKE ('collect:' || e."source_unique_key" || ':%');

UPDATE "hg_youban_publish_tg_job" j SET
  "collect_event_id"=t."collect_event_id",
  "collect_source_id"=t."collect_source_id",
  "collect_source_chat_id"=t."collect_source_chat_id",
  "collect_source_message_id"=t."collect_source_message_id"
FROM "hg_youban_publish_task" t
WHERE j."task_id"=t."id" AND j."collect_source_message_id"=0 AND t."collect_source_message_id">0;

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectEnabled');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '600', '600', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='realtimePushDelaySec');

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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "source_type" varchar(32) NOT NULL DEFAULT '',
  "event_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_id" bigint NOT NULL DEFAULT 0,
  "source_grouped_id" varchar(128) NOT NULL DEFAULT '',
  "source_media_key" varchar(255) NOT NULL DEFAULT '',
  "media_type" varchar(32) NOT NULL DEFAULT '',
  "source_ref_type" varchar(32) NOT NULL DEFAULT '',
  "source_file_id" varchar(255) NOT NULL DEFAULT '',
  "source_message_ref" varchar(255) NOT NULL DEFAULT '',
  "backup_channel_id" bigint NOT NULL DEFAULT 0,
  "backup_chat_id" varchar(128) NOT NULL DEFAULT '',
  "backup_message_id" bigint NOT NULL DEFAULT 0,
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "meta_json" text,
  "sort_index" integer NOT NULL DEFAULT 0,
  "cache_status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_type" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_grouped_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "meta_json" text;
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_event" ON "hg_youban_publish_collect_event_media" ("event_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_owner" ON "hg_youban_publish_collect_event_media" ("tenant_id", "source_id", "cache_status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_source" ON "hg_youban_publish_collect_event_media" ("source_chat_id", "source_message_id", "source_media_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_file" ON "hg_youban_publish_collect_event_media" ("source_file_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_cache" ON "hg_youban_publish_collect_event_media" ("cache_status", "updated_at", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "stage" varchar(64) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "meta_text" text,
  "created_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "dispatch_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_event" ON "hg_youban_publish_collect_event_log" ("event_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_owner" ON "hg_youban_publish_collect_event_log" ("tenant_id", "account_id", "created_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_stage" ON "hg_youban_publish_collect_event_log" ("event_id", "stage", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_template" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_tpl_owner" ON "hg_youban_publish_message_template" ("tenant_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "template_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT 'image',
  "name" varchar(255) NOT NULL DEFAULT '',
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "poster_storage_path" varchar(1024) NOT NULL DEFAULT '',
  "tg_file_id" varchar(1024) NOT NULL DEFAULT '',
  "tg_thumb_file_id" varchar(1024) NOT NULL DEFAULT '',
  "asset_hash" varchar(1024) NOT NULL DEFAULT '',
  "sort_index" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_media_tpl" ON "hg_youban_publish_message_media" ("template_id", "sort_index", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_push_plan" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "account_id" bigint NOT NULL DEFAULT 0,
  "template_ids" text,
  "target_chat_ids" text,
  "times" text,
  "interval_seconds" integer NOT NULL DEFAULT 60,
  "status" smallint NOT NULL DEFAULT 1,
  "next_run_at" timestamp DEFAULT NULL,
  "last_run_at" timestamp DEFAULT NULL,
  "last_result" text,
  "locked_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_plan_due" ON "hg_youban_publish_message_push_plan" ("status", "next_run_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_plan_owner" ON "hg_youban_publish_message_push_plan" ("tenant_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_plan" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "bind_code" varchar(32) NOT NULL DEFAULT '',
  "notify_chat_id" varchar(128) NOT NULL DEFAULT '',
  "notify_chat_type" varchar(32) NOT NULL DEFAULT '',
  "notify_chat_title" varchar(255) NOT NULL DEFAULT '',
  "notify_bound_at" timestamp DEFAULT NULL,
  "keywords_json" text,
  "status" smallint NOT NULL DEFAULT 1,
  "last_trigger_at" timestamp DEFAULT NULL,
  "last_result" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_plan_owner" ON "hg_youban_publish_message_listen_plan" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_plan_account" ON "hg_youban_publish_message_listen_plan" ("tg_account_id", "status", "id");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_plan_code" ON "hg_youban_publish_message_listen_plan" ("bind_code");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_target" (
  "id" BIGSERIAL PRIMARY KEY,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "target_chat_type" varchar(32) NOT NULL DEFAULT '',
  "target_chat_title" varchar(255) NOT NULL DEFAULT '',
  "target_chat_username" varchar(255) NOT NULL DEFAULT '',
  "last_matched_at" timestamp DEFAULT NULL,
  "last_matched_text" text,
  "last_matched_user_id" varchar(128) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_target_chat" ON "hg_youban_publish_message_listen_target" ("plan_id", "target_chat_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_target_plan" ON "hg_youban_publish_message_listen_target" ("plan_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_notice" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "target_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_id" bigint NOT NULL DEFAULT 0,
  "sender_user_id" varchar(128) NOT NULL DEFAULT '',
  "sender_username" varchar(128) NOT NULL DEFAULT '',
  "normalized_text_hash" varchar(128) NOT NULL DEFAULT '',
  "media_hash" varchar(128) NOT NULL DEFAULT '',
  "dedupe_key" varchar(255) NOT NULL DEFAULT '',
  "match_keywords_json" text,
  "notify_result" text,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_notice_dedupe" ON "hg_youban_publish_message_listen_notice" ("dedupe_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_notice_plan" ON "hg_youban_publish_message_listen_notice" ("plan_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_sender" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "telegram_first_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_last_name" varchar(128) NOT NULL DEFAULT '',
  "display_name" varchar(255) NOT NULL DEFAULT '',
  "last_seen_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_sender_user" ON "hg_youban_publish_message_listen_sender" ("tg_account_id", "telegram_user_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_sender_tenant" ON "hg_youban_publish_message_listen_sender" ("tenant_id", "tg_account_id");
