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

ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_tg_channel" ADD COLUMN IF NOT EXISTS "management_role" varchar(16) NOT NULL DEFAULT 'member';
ALTER TABLE "hg_youban_publish_account" ALTER COLUMN "public_follow_enabled" SET DEFAULT 0;

CREATE INDEX IF NOT EXISTS "idx_ybp_task_note_scope" ON "hg_youban_publish_task" ("tenant_id", "account_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_task_profile_tenant" ON "hg_youban_publish_task" ("profile_id", "tenant_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_task_search_scope" ON "hg_youban_publish_task" ("tenant_id", "account_id", "profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_content_profile_note_order" ON "hg_content_profile" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile_cover" ON "hg_youban_publish_media" ("profile_id", "sort_index", "id") WHERE "deleted_at" IS NULL AND ("media_type" IS NULL OR "media_type" = '' OR "media_type" <> 'video');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectEnabled');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '600', '600', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='realtimePushDelaySec');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'autoDelete', '自动删除规则', '[]string', 'rules', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', 230, '仅匹配整条消息的自动删除规则', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='rules');

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
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_notice_cooldown" ON "hg_youban_publish_message_listen_notice" ("plan_id", "sender_user_id", "normalized_text_hash", "created_at");

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

CREATE TABLE IF NOT EXISTS hg_youban_publish_quick_push_plan (
  id bigserial PRIMARY KEY,
  tenant_id bigint NOT NULL DEFAULT 0,
  name varchar(128) NOT NULL DEFAULT '',
  account_id bigint NOT NULL DEFAULT 0,
  target_chat_ids text,
  status smallint NOT NULL DEFAULT 1,
  created_by bigint NOT NULL DEFAULT 0,
  updated_by bigint NOT NULL DEFAULT 0,
  deleted_by bigint NOT NULL DEFAULT 0,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_title" varchar(255) NOT NULL DEFAULT '',
  "source_username" varchar(128) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "pull_offset_id" bigint NOT NULL DEFAULT 0,
  "pull_limit_days" integer NOT NULL DEFAULT 365,
  "message_total" integer NOT NULL DEFAULT 0,
  "message_done" integer NOT NULL DEFAULT 0,
  "group_total" integer NOT NULL DEFAULT 0,
  "group_done" integer NOT NULL DEFAULT 0,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_failed" integer NOT NULL DEFAULT 0,
  "imported" integer NOT NULL DEFAULT 0,
  "duplicate" integer NOT NULL DEFAULT 0,
  "error_message" text,
  "next_run_at" timestamp DEFAULT NULL,
  "result_json" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_group" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_grouped_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_ids" text,
  "source_unique_key" varchar(255) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "nickname" varchar(128) NOT NULL DEFAULT '',
  "profile_no" varchar(64) NOT NULL DEFAULT '',
  "raw_text" text,
  "profile_text" text,
  "verify_text" text,
  "media_json" text,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_failed" integer NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_profile_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "message_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_material_import_group" ON "hg_youban_publish_material_import_group" ("tenant_id", "source_unique_key");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_phash_bucket" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT '',
  "hash_value" varchar(64) NOT NULL DEFAULT '',
  "bucket_pos" smallint NOT NULL DEFAULT 0,
  "bucket_value" varchar(1) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_media_phash_bucket_media_pos" ON "hg_youban_publish_media_phash_bucket" ("media_id", "bucket_pos");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_search" ON "hg_youban_publish_media_phash_bucket" ("tenant_id", "media_type", "bucket_pos", "bucket_value") INCLUDE ("media_id", "profile_id", "account_id", "hash_value");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_phash_lsh" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT '',
  "hash_value" varchar(64) NOT NULL DEFAULT '',
  "bucket_pos" smallint NOT NULL DEFAULT 0,
  "bucket_value" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_media_phash_lsh_media_pos" ON "hg_youban_publish_media_phash_lsh" ("media_id", "bucket_pos");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_lsh_search" ON "hg_youban_publish_media_phash_lsh" ("tenant_id", "media_type", "bucket_pos", "bucket_value") INCLUDE ("media_id", "profile_id", "account_id", "hash_value");
CREATE TABLE IF NOT EXISTS "hg_youban_publish_note_index" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "uuid" varchar(128) NOT NULL DEFAULT '',
  "profile_no" varchar(64) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "summary" text,
  "plain_text" text,
  "tag" text,
  "province" varchar(64) NOT NULL DEFAULT '',
  "city" varchar(64) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "visibility" varchar(32) NOT NULL DEFAULT '',
  "review_status" varchar(32) NOT NULL DEFAULT '',
  "task_status" varchar(32) NOT NULL DEFAULT '',
  "cover_media_id" bigint NOT NULL DEFAULT 0,
  "published_at" timestamp DEFAULT NULL,
  "source_updated_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_note_index_scope_profile" UNIQUE ("tenant_id", "account_id", "profile_id")
);
-- Repair installations created before the projection table migration. The
-- application upsert uses the business unique key, but the generated model
-- and pagination still require a stable primary key.
ALTER TABLE "hg_youban_publish_note_index"
  ADD COLUMN IF NOT EXISTS "id" BIGSERIAL;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = 'hg_youban_publish_note_index'::regclass
      AND contype = 'p'
  ) THEN
    ALTER TABLE "hg_youban_publish_note_index"
      ADD CONSTRAINT "pk_ybp_note_index" PRIMARY KEY ("id");
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_tenant_updated" ON "hg_youban_publish_note_index" ("tenant_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_account_updated" ON "hg_youban_publish_note_index" ("account_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_profile" ON "hg_youban_publish_note_index" ("profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_title_trgm" ON "hg_youban_publish_note_index" USING gin ("title" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_plain_text_trgm" ON "hg_youban_publish_note_index" USING gin ("plain_text" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_content_profile_source_uuid" ON "hg_content_profile" ("source_note_uuid") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_task_profile_status" ON "hg_youban_publish_task" ("profile_id", "status", "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_active" ON "hg_youban_publish_media" ("task_id", "deleted_at", "sort_index", "id");
