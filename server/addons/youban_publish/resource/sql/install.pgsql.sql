CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant" (
  "id" BIGSERIAL PRIMARY KEY,
  "name" varchar(128) NOT NULL DEFAULT '',
  "contact_name" varchar(128) NOT NULL DEFAULT '',
  "contact_phone" varchar(64) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_status" ON "hg_youban_publish_tenant" ("status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_remark_trgm" ON "hg_youban_publish_tenant" USING gin ("remark" gin_trgm_ops);
CREATE TABLE IF NOT EXISTS "hg_youban_publish_merchant" (
  "id" BIGSERIAL PRIMARY KEY,
  "name" varchar(128) NOT NULL DEFAULT '',
  "contact_name" varchar(128) NOT NULL DEFAULT '',
  "contact_phone" varchar(64) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_merchant_status" ON "hg_youban_publish_merchant" ("status", "id");
INSERT INTO "hg_youban_publish_tenant" ("id", "name", "contact_name", "contact_phone", "remark", "status", "created_by", "updated_by", "deleted_by", "created_at", "updated_at", "deleted_at")
SELECT m."id", m."name", m."contact_name", m."contact_phone", m."remark", m."status", m."created_by", m."updated_by", m."deleted_by", m."created_at", m."updated_at", m."deleted_at"
FROM "hg_youban_publish_merchant" m
WHERE NOT EXISTS (SELECT 1 FROM "hg_youban_publish_tenant" t WHERE t."id" = m."id");
SELECT setval(pg_get_serial_sequence('"hg_youban_publish_tenant"', 'id'), COALESCE((SELECT MAX("id") FROM "hg_youban_publish_tenant"), 1), true);
CREATE TABLE IF NOT EXISTS "hg_youban_publish_account" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "parent_id" bigint NOT NULL DEFAULT 0,
  "account_type" varchar(32) NOT NULL DEFAULT 'uploader',
  "nickname" varchar(128) NOT NULL DEFAULT '',
  "username" varchar(128) NOT NULL DEFAULT '',
  "password_hash" varchar(128) NOT NULL DEFAULT '',
  "salt" varchar(16) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "daily_publish_limit" integer NOT NULL DEFAULT 0,
  "can_direct_publish" smallint NOT NULL DEFAULT 0,
  "allowed_channel_json" text,
  "allowed_region_json" text,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "password_hash" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "salt" varchar(16) NOT NULL DEFAULT '';
UPDATE "hg_youban_publish_account" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_account_tenant" ON "hg_youban_publish_account" ("tenant_id", "account_type", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_account_username_trgm" ON "hg_youban_publish_account" USING gin ("username" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_content_profile_note_order" ON "hg_content_profile" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_profile_state" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id_json" text,
  "customer_remark" text,
  "anti_scan_enabled" smallint NOT NULL DEFAULT 0,
  "publish_at" timestamp DEFAULT NULL,
  "publish_operation_no" varchar(128) NOT NULL DEFAULT '',
  "publish_task_status" varchar(32) NOT NULL DEFAULT '',
  "publish_task_updated_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_profile_state_profile" ON "hg_youban_publish_profile_state" ("profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_state_owner" ON "hg_youban_publish_profile_state" ("tenant_id", "account_id", "profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_state_publish_active" ON "hg_youban_publish_profile_state" ("publish_task_status", "publish_task_updated_at", "profile_id") WHERE "deleted_at" IS NULL AND "publish_task_status" <> '';

CREATE TABLE IF NOT EXISTS "hg_youban_publish_note_index" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint DEFAULT NULL,
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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_notice" (
  "id" BIGSERIAL PRIMARY KEY,
  "type" smallint NOT NULL DEFAULT 1,
  "title" varchar(255) NOT NULL DEFAULT '',
  "content" text,
  "tag" bigint NOT NULL DEFAULT 0,
  "receiver" text,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "sort" bigint NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "publish_at" timestamp DEFAULT NULL,
  "expire_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_notice_list" ON "hg_youban_publish_notice" ("status","type","sort","id");
CREATE INDEX IF NOT EXISTS "idx_ybp_notice_time" ON "hg_youban_publish_notice" ("status","publish_at","expire_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_notice_read" (
  "id" BIGSERIAL PRIMARY KEY,
  "notice_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "clicks" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_notice_read_account" UNIQUE ("notice_id","account_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_notice_read_account" ON "hg_youban_publish_notice_read" ("account_id","notice_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_tenant_updated" ON "hg_youban_publish_note_index" ("tenant_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_account_updated" ON "hg_youban_publish_note_index" ("account_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_updated_cursor" ON "hg_youban_publish_note_index" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_profile" ON "hg_youban_publish_note_index" ("profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_title_trgm" ON "hg_youban_publish_note_index" USING gin ("title" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_plain_text_trgm" ON "hg_youban_publish_note_index" USING gin ("plain_text" gin_trgm_ops) WHERE "deleted_at" IS NULL;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_name" varchar(64) NOT NULL DEFAULT 'lyy_cms',
  "base_url" varchar(255) NOT NULL DEFAULT '',
  "server_ip" varchar(64) NOT NULL DEFAULT '',
  "username" varchar(128) NOT NULL DEFAULT '',
  "password_cipher" varchar(512) NOT NULL DEFAULT '',
  "cookie_cipher" text,
  "limit_count" integer NOT NULL DEFAULT 0,
  "per_page" integer NOT NULL DEFAULT 12,
  "proxy_enabled" smallint NOT NULL DEFAULT 0,
  "proxy_pool" text,
  "media_concurrency" integer NOT NULL DEFAULT 4,
  "channel_id_json" text,
  "tg_start_at" timestamp DEFAULT NULL,
  "tg_end_at" timestamp DEFAULT NULL,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "progress_total" integer NOT NULL DEFAULT 0,
  "progress_done" integer NOT NULL DEFAULT 0,
  "page_total" integer NOT NULL DEFAULT 0,
  "page_done" integer NOT NULL DEFAULT 0,
  "item_total" integer NOT NULL DEFAULT 0,
  "item_done" integer NOT NULL DEFAULT 0,
  "imported" integer NOT NULL DEFAULT 0,
  "duplicate" integer NOT NULL DEFAULT 0,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_imported" integer NOT NULL DEFAULT 0,
  "tg_total" integer NOT NULL DEFAULT 0,
  "tg_done" integer NOT NULL DEFAULT 0,
  "tg_matched" integer NOT NULL DEFAULT 0,
  "last_source_note_id" bigint NOT NULL DEFAULT 0,
  "error_message" text,
  "result_json" text,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_import_task_scope" ON "hg_youban_publish_import_task" ("tenant_id", "account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_import_task_status" ON "hg_youban_publish_import_task" ("status", "updated_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_import_task_source" ON "hg_youban_publish_import_task" ("source_name", "id");

ALTER TABLE "hg_youban_publish_import_task" ADD COLUMN IF NOT EXISTS "server_ip" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_import_task" ADD COLUMN IF NOT EXISTS "cookie_cipher" text;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_run" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_name" varchar(64) NOT NULL DEFAULT 'lyy_cms',
  "base_url" varchar(255) NOT NULL DEFAULT '',
  "username" varchar(128) NOT NULL DEFAULT '',
  "run_type" varchar(32) NOT NULL DEFAULT 'import',
  "import_mode" varchar(32) NOT NULL DEFAULT 'incremental',
  "scan_mode" varchar(32) NOT NULL DEFAULT 'recent',
  "recent_count" integer NOT NULL DEFAULT 100,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "progress_total" integer NOT NULL DEFAULT 0,
  "progress_done" integer NOT NULL DEFAULT 0,
  "page_total" integer NOT NULL DEFAULT 0,
  "page_done" integer NOT NULL DEFAULT 0,
  "item_total" integer NOT NULL DEFAULT 0,
  "item_done" integer NOT NULL DEFAULT 0,
  "imported" integer NOT NULL DEFAULT 0,
  "duplicate" integer NOT NULL DEFAULT 0,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_imported" integer NOT NULL DEFAULT 0,
  "media_missing_storage" integer NOT NULL DEFAULT 0,
  "tg_total" integer NOT NULL DEFAULT 0,
  "tg_done" integer NOT NULL DEFAULT 0,
  "tg_matched" integer NOT NULL DEFAULT 0,
  "error_message" text,
  "params_json" text,
  "result_json" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_import_run_scope" ON "hg_youban_publish_import_run" ("tenant_id", "account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_import_run_task" ON "hg_youban_publish_import_run" ("task_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_run_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "run_id" bigint NOT NULL DEFAULT 0,
  "level" varchar(16) NOT NULL DEFAULT 'info',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "context" text,
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_import_run_log_run" ON "hg_youban_publish_import_run_log" ("run_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_title" varchar(255) NOT NULL DEFAULT '',
  "source_username" varchar(128) NOT NULL DEFAULT '',
  "channel_id_json" text,
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
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_owner" ON "hg_youban_publish_material_import_task" ("tenant_id", "account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_tg" ON "hg_youban_publish_material_import_task" ("tenant_id", "tg_account_id", "source_chat_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_status" ON "hg_youban_publish_material_import_task" ("status", "next_run_at", "id");

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
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_group_task" ON "hg_youban_publish_material_import_group" ("task_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_group_profile" ON "hg_youban_publish_material_import_group" ("profile_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_group_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "group_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "purpose" varchar(16) NOT NULL DEFAULT 'display',
  "media_type" varchar(32) NOT NULL DEFAULT '',
  "sort_index" integer NOT NULL DEFAULT 0,
  "source_file_id" varchar(255) NOT NULL DEFAULT '',
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "source_kind" varchar(32) NOT NULL DEFAULT '',
  "source_media_id" bigint NOT NULL DEFAULT 0,
  "source_access_hash" bigint NOT NULL DEFAULT 0,
  "source_file_reference" bytea,
  "source_thumb_size" varchar(32) NOT NULL DEFAULT '',
  "source_mime_type" varchar(128) NOT NULL DEFAULT '',
  "source_dc_id" integer NOT NULL DEFAULT 0,
  "source_size" bigint NOT NULL DEFAULT 0,
  "file_md5" varchar(64) NOT NULL DEFAULT '',
  "file_phash" varchar(128) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_group_media_group" ON "hg_youban_publish_material_import_group_media" ("group_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_group_media_task" ON "hg_youban_publish_material_import_group_media" ("task_id", "group_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_match_run" (
  "id" BIGSERIAL PRIMARY KEY,
  "import_run_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "channel_id_json" text,
  "scan_days" integer NOT NULL DEFAULT 180,
  "threshold" integer NOT NULL DEFAULT 80,
  "profile_total" integer NOT NULL DEFAULT 0,
  "profile_done" integer NOT NULL DEFAULT 0,
  "candidate_total" integer NOT NULL DEFAULT 0,
  "auto_matched" integer NOT NULL DEFAULT 0,
  "manual_pending" integer NOT NULL DEFAULT 0,
  "confirmed" integer NOT NULL DEFAULT 0,
  "skipped" integer NOT NULL DEFAULT 0,
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_import_match_run_import" ON "hg_youban_publish_import_match_run" ("import_run_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_import_match_run_scope" ON "hg_youban_publish_import_match_run" ("tenant_id", "account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_match_item" (
  "id" BIGSERIAL PRIMARY KEY,
  "match_run_id" bigint NOT NULL DEFAULT 0,
  "import_run_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "display_group_key" varchar(128) NOT NULL DEFAULT '',
  "verify_group_key" varchar(128) NOT NULL DEFAULT '',
  "display_score" integer NOT NULL DEFAULT 0,
  "verify_score" integer NOT NULL DEFAULT 0,
  "total_score" integer NOT NULL DEFAULT 0,
  "match_status" varchar(32) NOT NULL DEFAULT 'manual_pending',
  "match_mode" varchar(32) NOT NULL DEFAULT '',
  "reason_json" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_import_match_item_scope" ON "hg_youban_publish_import_match_item" ("match_run_id", "profile_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_import_match_item_profile" ON "hg_youban_publish_import_match_item" ("profile_id", "task_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_match_candidate" (
  "id" BIGSERIAL PRIMARY KEY,
  "match_run_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "group_key" varchar(128) NOT NULL DEFAULT '',
  "media_group_id" varchar(128) NOT NULL DEFAULT '',
  "first_message_id" bigint NOT NULL DEFAULT 0,
  "last_message_id" bigint NOT NULL DEFAULT 0,
  "message_date" timestamp DEFAULT NULL,
  "caption_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "media_types" varchar(128) NOT NULL DEFAULT '',
  "preview_json" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_import_match_candidate_group" ON "hg_youban_publish_import_match_candidate" ("match_run_id", "channel_id", "group_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_import_match_candidate_channel" ON "hg_youban_publish_import_match_candidate" ("match_run_id", "channel_id", "message_date");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_account_setting" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "enable_suffix" smallint NOT NULL DEFAULT 0,
  "suffix_content" text,
  "enable_title_mark" smallint NOT NULL DEFAULT 0,
  "mark_mode" varchar(16) NOT NULL DEFAULT 'nickname',
  "number_source" varchar(16) NOT NULL DEFAULT 'sequence',
  "custom_mark_text" varchar(128) NOT NULL DEFAULT '',
  "mark_position" varchar(16) NOT NULL DEFAULT 'bottom',
  "default_recycle_days" integer NOT NULL DEFAULT 0,
  "cycle_publish_enabled" smallint NOT NULL DEFAULT 0,
  "cycle_publish_days" integer NOT NULL DEFAULT 4,
  "cycle_publish_time" varchar(16) NOT NULL DEFAULT '',
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_account_setting_account" ON "hg_youban_publish_account_setting" ("tenant_id", "account_id") WHERE "deleted_at" IS NULL;
ALTER TABLE "hg_youban_publish_account_setting" ADD COLUMN IF NOT EXISTS "cycle_publish_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account_setting" ADD COLUMN IF NOT EXISTS "cycle_publish_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_account_setting" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_auto_delete_config" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "enabled" smallint NOT NULL DEFAULT 0,
  "bot_ids_json" text,
  "custom_keywords_json" text,
  "custom_rules_json" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tenant_auto_delete_config_tenant" ON "hg_youban_publish_tenant_auto_delete_config" ("tenant_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint DEFAULT NULL,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "attachment_id" bigint NOT NULL DEFAULT 0,
  "original_attachment_id" bigint NOT NULL DEFAULT 0,
  "edited_attachment_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT 'image',
  "must_send" smallint NOT NULL DEFAULT 0,
  "purpose" varchar(16) NOT NULL DEFAULT 'display',
  "name" varchar(255) NOT NULL DEFAULT '',
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "original_file_url" varchar(1024) NOT NULL DEFAULT '',
  "edited_file_url" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "poster_storage_path" varchar(1024) NOT NULL DEFAULT '',
  "tg_file_id" varchar(255) NOT NULL DEFAULT '',
  "tg_thumb_file_id" varchar(255) NOT NULL DEFAULT '',
  "tg_cache_asset_hash" varchar(1024) NOT NULL DEFAULT '',
  "tg_cache_status" varchar(16) NOT NULL DEFAULT 'invalid',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "original_storage_path" varchar(1024) NOT NULL DEFAULT '',
  "edited_storage_path" varchar(1024) NOT NULL DEFAULT '',
  "mime_type" varchar(128) NOT NULL DEFAULT '',
  "md5" varchar(64) NOT NULL DEFAULT '',
  "perceptual_hash" varchar(64) NOT NULL DEFAULT '',
  "edit_config_json" text,
  "edit_status" varchar(16) NOT NULL DEFAULT 'raw',
  "size" bigint NOT NULL DEFAULT 0,
  "sort_index" integer NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "perceptual_hash" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "purpose" varchar(16) NOT NULL DEFAULT 'display';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "poster_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "poster_storage_path" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "tg_file_id" varchar(255) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "tg_thumb_file_id" varchar(255) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "tg_cache_asset_hash" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "tg_cache_status" varchar(16) NOT NULL DEFAULT 'invalid';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "original_attachment_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "original_file_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "original_storage_path" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "edited_attachment_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "edited_file_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "edited_storage_path" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "edit_config_json" text;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "edit_status" varchar(16) NOT NULL DEFAULT 'raw';
UPDATE "hg_youban_publish_media" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
UPDATE "hg_youban_publish_media" SET "original_attachment_id" = "attachment_id", "original_file_url" = "file_url", "original_storage_path" = "storage_path" WHERE "original_attachment_id" = 0 AND "attachment_id" > 0;
UPDATE "hg_youban_publish_media" SET "edit_status" = 'edited' WHERE ("edit_status" = '' OR "edit_status" = 'raw' OR "edit_status" IS NULL) AND ("edited_attachment_id" > 0 OR "edited_storage_path" <> '' OR "edited_file_url" <> '' OR lower("name") LIKE '%-edited.%' OR lower("name") LIKE '%_edited.%');
UPDATE "hg_youban_publish_media" SET "tg_cache_status" = 'valid', "tg_cache_asset_hash" = COALESCE(NULLIF("md5", ''), NULLIF("storage_path", ''), NULLIF("file_url", '')) WHERE "tg_file_id" <> '' AND ("tg_cache_status" = '' OR "tg_cache_status" = 'invalid' OR "tg_cache_status" IS NULL) AND "edit_status" = 'raw';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_attachment" ON "hg_youban_publish_media" ("task_id", "attachment_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_sort" ON "hg_youban_publish_media" ("task_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile" ON "hg_youban_publish_media" ("profile_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile_current" ON "hg_youban_publish_media" ("profile_id", "purpose", "sort_index", "id") WHERE "task_id" IS NULL AND "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile_cover" ON "hg_youban_publish_media" ("profile_id", "sort_index", "id") WHERE "deleted_at" IS NULL AND ("media_type" IS NULL OR "media_type" = '' OR "media_type" <> 'video');
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash" ON "hg_youban_publish_media" ("perceptual_hash");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_md5_scope" ON "hg_youban_publish_media" ("account_id", "md5", "tenant_id", "profile_id", "id") WHERE "deleted_at" IS NULL AND "md5" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_similar_tenant" ON "hg_youban_publish_media" ("tenant_id", "media_type", "account_id", "profile_id", "id") WHERE "deleted_at" IS NULL AND "perceptual_hash" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_similar_account" ON "hg_youban_publish_media" ("account_id", "media_type", "profile_id", "id") WHERE "deleted_at" IS NULL AND "perceptual_hash" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_purpose" ON "hg_youban_publish_media" ("task_id", "purpose", "sort_index", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_face" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "face_index" integer NOT NULL DEFAULT 0,
  "bbox_json" text NOT NULL DEFAULT '',
  "embedding_model" varchar(64) NOT NULL DEFAULT '',
  "embedding_vector" text NOT NULL DEFAULT '',
  "feature_hash" varchar(128) NOT NULL DEFAULT '',
  "quality_score" numeric(10,4) NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_media_face_media" ON "hg_youban_publish_media_face" ("media_id", "face_index");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_face_profile" ON "hg_youban_publish_media_face" ("tenant_id", "account_id", "profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_face_feature" ON "hg_youban_publish_media_face" ("feature_hash");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_anti_scan_cache" (
  "id" BIGSERIAL PRIMARY KEY,
  "image_hash" varchar(64) NOT NULL DEFAULT '',
  "config_hash" varchar(64) NOT NULL DEFAULT '',
  "provider" varchar(32) NOT NULL DEFAULT '',
  "face_count" integer NOT NULL DEFAULT 0,
  "face_json" text NOT NULL DEFAULT '',
  "segment_json" text NOT NULL DEFAULT '',
  "original_url" varchar(1024) NOT NULL DEFAULT '',
  "preview_url" varchar(1024) NOT NULL DEFAULT '',
  "warnings_json" text NOT NULL DEFAULT '',
  "cloud_raw_saved" smallint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_image" ON "hg_youban_publish_anti_scan_cache" ("image_hash");
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_config" ON "hg_youban_publish_anti_scan_cache" ("image_hash", "config_hash");
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_provider" ON "hg_youban_publish_anti_scan_cache" ("provider", "cloud_raw_saved");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cloud_resource_usage" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "resource_type" varchar(32) NOT NULL DEFAULT '',
  "scene" varchar(32) NOT NULL DEFAULT '',
  "usage_date" date NOT NULL,
  "request_count" bigint NOT NULL DEFAULT 0,
  "success_count" bigint NOT NULL DEFAULT 0,
  "failure_count" bigint NOT NULL DEFAULT 0,
  "total_duration_ms" bigint NOT NULL DEFAULT 0,
  "last_called_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_cloud_usage_daily" ON "hg_youban_publish_cloud_resource_usage" ("tenant_id", "account_id", "resource_type", "scene", "usage_date");
CREATE INDEX IF NOT EXISTS "idx_ybp_cloud_usage_date" ON "hg_youban_publish_cloud_resource_usage" ("usage_date", "resource_type", "account_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_cloud_usage_account" ON "hg_youban_publish_cloud_resource_usage" ("account_id", "usage_date");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_anti_scan_material" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "type" varchar(32) NOT NULL DEFAULT 'sticker',
  "name" varchar(120) NOT NULL DEFAULT '',
  "url" varchar(1024) NOT NULL DEFAULT '',
  "sort" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_material_owner" ON "hg_youban_publish_anti_scan_material" ("tenant_id", "account_id", "type", "deleted_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tag" (
  "id" BIGSERIAL PRIMARY KEY,
  "name" varchar(64) NOT NULL DEFAULT '',
  "review_status" varchar(32) NOT NULL DEFAULT 'pending',
  "status" smallint NOT NULL DEFAULT 1,
  "use_count" integer NOT NULL DEFAULT 0,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tag_name_deleted" ON "hg_youban_publish_tag" ("name", "deleted_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_tag_review_status" ON "hg_youban_publish_tag" ("review_status", "status", "id");

ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "name" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "review_status" varchar(32) NOT NULL DEFAULT 'pending';
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "status" smallint NOT NULL DEFAULT 1;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "use_count" integer NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "created_by" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "updated_by" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "deleted_by" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "created_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "updated_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "deleted_at" timestamp DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tag_name_deleted" ON "hg_youban_publish_tag" ("name", "deleted_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_tag_review_status" ON "hg_youban_publish_tag" ("review_status", "status", "id");

INSERT INTO "hg_youban_publish_tag" ("name", "review_status", "status", "use_count", "created_by", "updated_by", "deleted_by", "created_at", "updated_at", "deleted_at")
SELECT seed."name", 'approved', 1, 0, 0, 0, 0, NOW(), NOW(), NULL
FROM (
  VALUES ('颜值'), ('穿搭'), ('美食'), ('探店'), ('旅行'), ('运动'), ('健身'), ('摄影'), ('音乐'), ('舞蹈'),
    ('日常'), ('生活'), ('情感'), ('职场'), ('学习'), ('数码'), ('游戏'), ('电影'), ('宠物'), ('家居')
) AS seed("name")
WHERE NOT EXISTS (SELECT 1 FROM "hg_youban_publish_tag");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_job" (
  "id" BIGSERIAL PRIMARY KEY, "task_id" bigint DEFAULT NULL, "operation_no" varchar(128) NOT NULL DEFAULT '', "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0, "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "bot_id" bigint NOT NULL DEFAULT 0, "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "collect_event_id" bigint NOT NULL DEFAULT 0, "collect_source_id" bigint NOT NULL DEFAULT 0,
  "collect_source_chat_id" varchar(128) NOT NULL DEFAULT '', "collect_source_message_id" bigint NOT NULL DEFAULT 0,
  "tg_message_id" bigint NOT NULL DEFAULT 0, "asynq_task_id" varchar(128) NOT NULL DEFAULT '', "status" varchar(32) NOT NULL DEFAULT 'pending',
  "retry_count" integer NOT NULL DEFAULT 0, "next_retry_at" timestamp DEFAULT NULL, "sent_at" timestamp DEFAULT NULL,
  "cycle_enabled" smallint NOT NULL DEFAULT 0, "cycle_days" integer NOT NULL DEFAULT 4, "cycle_publish_time" varchar(16) NOT NULL DEFAULT '',
  "next_cycle_at" timestamp DEFAULT NULL, "priority" integer NOT NULL DEFAULT 100, "queue_name" varchar(64) NOT NULL DEFAULT '',
  "dispatch_status" varchar(32) NOT NULL DEFAULT 'idle', "dispatched_at" timestamp DEFAULT NULL, "dispatch_count" integer NOT NULL DEFAULT 0,
  "send_phase" varchar(32) NOT NULL DEFAULT '', "reconcile_count" integer NOT NULL DEFAULT 0,
  "last_dispatch_error" varchar(512) NOT NULL DEFAULT '', "error_message" text, "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "operation_no" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "channel_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "asynq_task_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "sent_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "cycle_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "cycle_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "next_cycle_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "priority" integer NOT NULL DEFAULT 100;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "queue_name" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "dispatch_status" varchar(32) NOT NULL DEFAULT 'idle';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "dispatched_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "dispatch_count" integer NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "last_dispatch_error" varchar(512) NOT NULL DEFAULT '';
UPDATE "hg_youban_publish_tg_job" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
DROP INDEX IF EXISTS "uk_ybp_tg_job_task_channel";
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task_channel" ON "hg_youban_publish_tg_job" ("task_id", "channel_id", "id");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_job_operation_channel" ON "hg_youban_publish_tg_job" ("task_id", "operation_no", "channel_id") WHERE "operation_no" <> '';
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_job_profile_operation_channel" ON "hg_youban_publish_tg_job" ("profile_id", "operation_no", "channel_id") WHERE "task_id" IS NULL AND "profile_id" > 0 AND "operation_no" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_status_retry" ON "hg_youban_publish_tg_job" ("status", "next_retry_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task" ON "hg_youban_publish_tg_job" ("task_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_profile_operation" ON "hg_youban_publish_tg_job" ("profile_id", "operation_no", "status", "id") WHERE "task_id" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_profile_cleanup" ON "hg_youban_publish_tg_job" ("profile_id", "tenant_id", "created_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle" ON "hg_youban_publish_tg_job" ("cycle_enabled", "next_cycle_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle_due" ON "hg_youban_publish_tg_job" ("next_cycle_at", "id") INCLUDE ("tenant_id", "account_id", "profile_id", "channel_id", "cycle_days") WHERE "cycle_enabled" = 1 AND "status" IN ('sent','superseded') AND "next_cycle_at" IS NOT NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_operation" ON "hg_youban_publish_tg_job" ("operation_no", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_scheduler" ON "hg_youban_publish_tg_job" ("dispatch_status", "status", "priority", "next_retry_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_channel_dispatch" ON "hg_youban_publish_tg_job" ("target_chat_id", "dispatch_status", "status", "updated_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_collect_order" ON "hg_youban_publish_tg_job" ("channel_id", "target_chat_id", "collect_source_id", "collect_source_chat_id", "collect_source_message_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle_channel_status_op" ON "hg_youban_publish_tg_job" ("channel_id", "status", "operation_no", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_profile_channel_chat" ON "hg_youban_publish_tg_job" ("tenant_id", "profile_id", "target_chat_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_queue_stat" (
  "id" BIGSERIAL PRIMARY KEY,
  "stat_time" timestamp DEFAULT NULL,
  "queue_name" varchar(64) NOT NULL DEFAULT '',
  "priority_level" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT '',
  "job_count" integer NOT NULL DEFAULT 0,
  "oldest_job_at" timestamp DEFAULT NULL,
  "latest_job_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_queue_stat" ON "hg_youban_publish_tg_queue_stat" ("queue_name", "priority_level", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_queue_stat_count" ON "hg_youban_publish_tg_queue_stat" ("job_count", "updated_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_channel_stat" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "channel_title" varchar(255) NOT NULL DEFAULT '',
  "pending_count" integer NOT NULL DEFAULT 0,
  "queued_count" integer NOT NULL DEFAULT 0,
  "sending_count" integer NOT NULL DEFAULT 0,
  "sent_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "retry_count" integer NOT NULL DEFAULT 0,
  "rate_limit_count" integer NOT NULL DEFAULT 0,
  "last_sent_at" timestamp DEFAULT NULL,
  "last_error_at" timestamp DEFAULT NULL,
  "last_error_message" varchar(512) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_channel_stat" ON "hg_youban_publish_tg_channel_stat" ("channel_id", "target_chat_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_channel_stat_tenant" ON "hg_youban_publish_tg_channel_stat" ("tenant_id", "account_id", "updated_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_bot_stat" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "bot_name" varchar(128) NOT NULL DEFAULT '',
  "bot_username" varchar(128) NOT NULL DEFAULT '',
  "pending_count" integer NOT NULL DEFAULT 0,
  "queued_count" integer NOT NULL DEFAULT 0,
  "sending_count" integer NOT NULL DEFAULT 0,
  "sent_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "retry_count" integer NOT NULL DEFAULT 0,
  "rate_limit_count" integer NOT NULL DEFAULT 0,
  "last_sent_at" timestamp DEFAULT NULL,
  "last_error_at" timestamp DEFAULT NULL,
  "last_error_message" varchar(512) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_bot_stat" ON "hg_youban_publish_tg_bot_stat" ("tenant_id", "bot_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_bot_stat_updated" ON "hg_youban_publish_tg_bot_stat" ("updated_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_message" (
  "id" BIGSERIAL PRIMARY KEY,
  "job_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "tg_message_id" bigint NOT NULL DEFAULT 0,
  "media_group_id" varchar(128) NOT NULL DEFAULT '',
  "media_id" bigint NOT NULL DEFAULT 0,
  "purpose" varchar(16) NOT NULL DEFAULT '',
  "tg_file_id" varchar(255) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'sent',
  "sent_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_message_job_message" ON "hg_youban_publish_tg_message" ("job_id", "tg_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_message_job" ON "hg_youban_publish_tg_message" ("job_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_message_task" ON "hg_youban_publish_tg_message" ("task_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_message_profile" ON "hg_youban_publish_tg_message" ("tenant_id", "account_id", "profile_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_message_repair_run" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "progress" integer NOT NULL DEFAULT 0,
  "channel_count" integer NOT NULL DEFAULT 0,
  "scanned_count" integer NOT NULL DEFAULT 0,
  "matched_count" integer NOT NULL DEFAULT 0,
  "error_message" varchar(1000) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_msg_repair_scope" ON "hg_youban_publish_tg_message_repair_run" ("tenant_id", "account_id", "profile_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_msg_repair_status" ON "hg_youban_publish_tg_message_repair_run" ("status", "updated_at", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_message_cache" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "tg_message_id" bigint NOT NULL DEFAULT 0,
  "message_text" text,
  "media_type" varchar(32) NOT NULL DEFAULT '',
  "message_date" timestamp DEFAULT NULL,
  "media_group_id" varchar(128) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_msg_cache_msg" ON "hg_youban_publish_tg_message_cache" ("tenant_id", "channel_id", "tg_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_msg_cache_channel" ON "hg_youban_publish_tg_message_cache" ("tenant_id", "tg_account_id", "channel_id", "message_date");
ALTER TABLE "hg_youban_publish_tg_message_cache" ADD COLUMN IF NOT EXISTS "media_type" varchar(32) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_job_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "job_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "action" varchar(32) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_tenant" ON "hg_youban_publish_tg_job_log" ("tenant_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_account" ON "hg_youban_publish_tg_job_log" ("tenant_id", "account_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_created" ON "hg_youban_publish_tg_job_log" ("created_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_job" ON "hg_youban_publish_tg_job_log" ("job_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_task" ON "hg_youban_publish_tg_job_log" ("task_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_daily_stat" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "stat_date" date NOT NULL,
  "new_profile_count" integer NOT NULL DEFAULT 0,
  "published_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "down_count" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_daily_stat_account_date" ON "hg_youban_publish_daily_stat" ("tenant_id", "account_id", "stat_date");
CREATE INDEX IF NOT EXISTS "idx_ybp_daily_stat_date" ON "hg_youban_publish_daily_stat" ("stat_date", "account_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_bot" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_name" varchar(128) NOT NULL DEFAULT '',
  "bot_username" varchar(128) NOT NULL DEFAULT '',
  "bot_token" varchar(255) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_bot" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_tenant" ON "hg_youban_publish_bot" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_status" ON "hg_youban_publish_bot" ("status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_login" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "login_token" varchar(128) NOT NULL DEFAULT '',
  "qr_url" varchar(1024) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "session_key" varchar(255) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "expires_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_login" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_login" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
UPDATE "hg_youban_publish_tg_login" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_login_token" ON "hg_youban_publish_tg_login" ("login_token");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_login_account" ON "hg_youban_publish_tg_login" ("account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_account" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "display_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "telegram_first_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_last_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_phone" varchar(64) NOT NULL DEFAULT '',
  "telegram_is_bot" smallint NOT NULL DEFAULT 0,
  "session_key" varchar(255) NOT NULL DEFAULT '',
  "login_token" varchar(128) NOT NULL DEFAULT '',
  "qr_url" varchar(1024) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "last_login_at" timestamp DEFAULT NULL,
  "expires_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_first_name" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_last_name" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_phone" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_is_bot" smallint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_account_tenant" ON "hg_youban_publish_tg_account" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_account_login" ON "hg_youban_publish_tg_account" ("login_token");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_session" (
  "id" BIGSERIAL PRIMARY KEY,
  "session_key" varchar(255) NOT NULL DEFAULT '',
  "session_data" bytea NOT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "idx_ybp_tg_session_key" ON "hg_youban_publish_tg_session" ("session_key");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_title" varchar(128) NOT NULL DEFAULT '',
  "channel_username" varchar(128) NOT NULL DEFAULT '',
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "publish_direction" varchar(16) NOT NULL DEFAULT 'up',
  "cycle_publish_enabled" smallint NOT NULL DEFAULT 0,
  "cycle_publish_days" integer NOT NULL DEFAULT 4,
  "cycle_publish_time" varchar(16) NOT NULL DEFAULT '',
  "is_default_selected" smallint NOT NULL DEFAULT 1,
  "publish_visible" smallint NOT NULL DEFAULT 1,
  "anti_scan_enabled" smallint NOT NULL DEFAULT 0,
  "text_obfuscation_enabled" smallint NOT NULL DEFAULT 0,
  "bot_id_json" text,
  "bot_permission_status_json" text NOT NULL DEFAULT '[]',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "last_refresh_status" varchar(32) NOT NULL DEFAULT '',
  "last_refresh_message" varchar(500) NOT NULL DEFAULT '',
  "last_refresh_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_tenant_account" ON "hg_youban_publish_channel" ("tenant_id", "tg_account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_tenant_direction" ON "hg_youban_publish_channel" ("tenant_id", "publish_direction", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_chat" ON "hg_youban_publish_channel" ("tenant_id", "target_chat_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_feature_permission" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "feature_code" varchar(64) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 2,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  UNIQUE("tenant_id", "feature_code")
);

ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_next_run_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_run_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_active_run_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_error_message" text;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "is_default_selected" smallint NOT NULL DEFAULT 1;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "publish_visible" smallint NOT NULL DEFAULT 1;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "anti_scan_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "text_obfuscation_enabled" smallint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" varchar(128) NOT NULL DEFAULT '',
  "access_hash" varchar(128) NOT NULL DEFAULT '',
  "channel_title" varchar(128) NOT NULL DEFAULT '',
  "channel_username" varchar(128) NOT NULL DEFAULT '',
  "management_role" varchar(16) NOT NULL DEFAULT 'member',
  "is_broadcast" smallint NOT NULL DEFAULT 0,
  "is_megagroup" smallint NOT NULL DEFAULT 0,
  "can_post_messages" smallint NOT NULL DEFAULT 0,
  "can_invite_users" smallint NOT NULL DEFAULT 0,
  "can_add_admins" smallint NOT NULL DEFAULT 0,
  "last_sync_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_channel_account_channel" ON "hg_youban_publish_tg_channel" ("tenant_id", "tg_account_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_channel_search" ON "hg_youban_publish_tg_channel" ("tenant_id", "tg_account_id", "channel_title", "channel_username");

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Telegram App ID', 'int', 'appId', '0', '0', 10, '扫码登录使用的 Telegram API ID，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='appId');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Telegram App Hash', 'string', 'appHash', '', '', 20, '扫码登录使用的 Telegram App Hash，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='appHash');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', '代理地址', 'string', 'proxyUrl', '', '', 30, '本地开发可配置 socks5://127.0.0.1:7890，也支持 http/https 代理', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='proxyUrl');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Bot运行模式', 'string', 'botRuntimeMode', 'auto', 'auto', 40, 'auto/develop 使用 pull，production 使用 webhook；也可显式配置 pull/webhook', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='botRuntimeMode');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Webhook Base URL', 'string', 'webhookBaseUrl', '', '', 50, '线上 webhook 的公网域名，例如 https://api.example.com', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='webhookBaseUrl');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Webhook Secret', 'string', 'webhookSecret', '', '', 60, 'Telegram webhook secret token，可选', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='webhookSecret');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', '默认推送 Chat ID', 'string', 'defaultTargetChat', '', '', 70, '资料发布后默认推送的 Telegram chat_id，可由后续频道配置覆盖', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='defaultTargetChat');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectEnabled');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集推送总开关', 'int', 'collectPushEnabled', '1', '1', 15, '是否允许采集资料推送到 Telegram 频道', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectPushEnabled');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '600', '600', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='realtimePushDelaySec');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT seed."addon_name", seed."group", seed."name", seed."type", seed."key", seed."value", seed."default_value", seed."sort", seed."tip", 0, 1, NOW(), NOW()
FROM (
  VALUES
    ('youban_publish', 'cloudResource', '腾讯云人脸检测开关', 'int', 'tencentVisionEnabled', '0', '0', 80, '是否启用腾讯云人脸检测，用于二维码和贴图避开人脸'),
    ('youban_publish', 'cloudResource', '腾讯云站点', 'string', 'tencentCloudSite', 'mainland', 'mainland', 85, 'mainland 国内云；intl 国际云'),
    ('youban_publish', 'cloudResource', '腾讯云 SecretId', 'string', 'tencentSecretId', '', '', 90, 'CAM 子用户 SecretId，仅授予 IAI 必要权限'),
    ('youban_publish', 'cloudResource', '腾讯云 SecretKey', 'string', 'tencentSecretKey', '', '', 100, 'CAM 子用户 SecretKey，页面回显会脱敏'),
    ('youban_publish', 'cloudResource', '腾讯云 Region', 'string', 'tencentRegion', 'ap-guangzhou', 'ap-guangzhou', 110, '国内云默认 ap-guangzhou，国际云默认 ap-singapore'),
    ('youban_publish', 'cloudResource', '腾讯云 BDA Endpoint', 'string', 'tencentBdaEndpoint', 'bda.tencentcloudapi.com', 'bda.tencentcloudapi.com', 120, '旧国内版人体分析接口域名，当前默认不强制使用'),
    ('youban_publish', 'cloudResource', '腾讯云 IAI Endpoint', 'string', 'tencentIaiEndpoint', 'iai.tencentcloudapi.com', 'iai.tencentcloudapi.com', 130, '国内云 iai.tencentcloudapi.com；国际云 iai.intl.tencentcloudapi.com'),
    ('youban_publish', 'cloudResource', 'FAPIHub抠图开关', 'int', 'fapiHubEnabled', '0', '0', 140, '是否启用 FAPIHub 抠图，用于背景替换和人像背景贴图'),
    ('youban_publish', 'cloudResource', 'FAPIHub API Key', 'string', 'fapiHubApiKey', '', '', 150, 'FAPIHub API Key，页面回显会脱敏'),
    ('youban_publish', 'cloudResource', 'FAPIHub Endpoint', 'string', 'fapiHubEndpoint', 'https://fapihub.com/v2/rembg/', 'https://fapihub.com/v2/rembg/', 160, 'FAPIHub 抠图接口地址'),
    ('youban_publish', 'cloudResource', 'FAPIHub Model', 'string', 'fapiHubModel', 'falcon', 'falcon', 170, 'FAPIHub 抠图模型，默认 falcon')
) AS seed("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip")
WHERE NOT EXISTS (
  SELECT 1 FROM "hg_sys_addons_config" c
  WHERE c."addon_name" = seed."addon_name"
    AND c."group" = seed."group"
    AND c."key" = seed."key"
);

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT seed."addon_name", seed."group", seed."name", seed."type", seed."key", seed."value", seed."default_value", seed."sort", seed."tip", 0, 1, NOW(), NOW()
FROM (
  VALUES
    ('youban_publish', 'publish', '下架不推送到下架频道', 'int', 'skipDownChannelEnabled', '1', '1', 130, '资料下架时是否跳过下架频道推送'),
    ('youban_publish', 'publish', '发送间隔秒数', 'int', 'sendIntervalSeconds', '3', '3', 140, 'Telegram 消息发送间隔'),
    ('youban_publish', 'publish', '发送时间窗口开关', 'int', 'sendWindowEnabled', '0', '0', 150, '是否限制自动发送执行时间段'),
    ('youban_publish', 'publish', '发送开始时间', 'string', 'sendWindowStart', '', '', 160, '自动发送允许开始时间'),
    ('youban_publish', 'publish', '发送结束时间', 'string', 'sendWindowEnd', '', '', 170, '自动发送允许结束时间'),
    ('youban_publish', 'publish', '失败处理策略', 'string', 'failureStrategy', 'continue', 'continue', 180, 'continue 继续后续任务，stop 停止后续任务'),
    ('youban_publish', 'publish', '失败重试开关', 'int', 'retryEnabled', '1', '1', 190, '发送失败后是否重试'),
    ('youban_publish', 'publish', '最大重试次数', 'int', 'maxRetryCount', '3', '3', 200, '发送失败最大重试次数'),
    ('youban_publish', 'publish', '重试间隔分钟', 'int', 'retryIntervalMinutes', '5', '5', 210, '发送失败重试间隔'),
    ('youban_publish', 'publish', '默认防扫图开关', 'int', 'defaultAntiScanEnabled', '0', '0', 220, '新发布内容默认是否启用防扫图'),
    ('youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', 220, '所有租户继承的默认自动删除关键词'),
    ('youban_publish', 'autoDelete', '自动删除规则', '[]string', 'rules', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', 230, '仅匹配整条消息的自动删除规则'),
    ('youban_publish', 'antiScan', '防扫图总开关', 'int', 'antiScanEnabled', '0', '0', 300, '是否启用防扫图能力'),
    ('youban_publish', 'antiScan', '新笔记默认防扫图', 'int', 'defaultNewNoteEnabled', '0', '0', 310, '新笔记默认是否开启防扫图'),
    ('youban_publish', 'antiScan', '已有资料批量处理', 'int', 'existingBatchEnabled', '0', '0', 320, '是否对已有资料触发批量处理意图'),
    ('youban_publish', 'antiScan', '发送前强制处理', 'int', 'forceBeforeSendEnabled', '0', '0', 330, '发送前是否强制生成防扫图副本'),
    ('youban_publish', 'antiScan', '单条资料允许覆盖', 'int', 'allowSingleOverrideEnabled', '0', '0', 340, '单条资料是否允许覆盖全局开关'),
    ('youban_publish', 'antiScan', '移除图片元信息', 'int', 'metadataStripEnabled', '0', '0', 350, '是否移除 EXIF 等图片元信息'),
    ('youban_publish', 'antiScan', '尺寸微调', 'int', 'resizeEnabled', '0', '0', 360, '是否轻微调整图片尺寸'),
    ('youban_publish', 'antiScan', '尺寸缩放比例', 'int', 'resizeScale', '96', '96', 370, '尺寸缩放比例，80-100'),
    ('youban_publish', 'antiScan', '轻微裁剪', 'int', 'cropEnabled', '0', '0', 380, '是否轻微裁剪图片边缘'),
    ('youban_publish', 'antiScan', '裁剪比例', 'int', 'cropPercent', '2', '2', 390, '边缘裁剪比例，1-8'),
    ('youban_publish', 'antiScan', '人像背景贴图', 'int', 'portraitBackgroundEnabled', '0', '0', 410, '是否启用人像背景贴图'),
    ('youban_publish', 'antiScan', '人像背景替换', 'int', 'backgroundReplaceEnabled', '0', '0', 420, '是否启用替换背景处理'),
    ('youban_publish', 'antiScan', '背景模糊', 'int', 'backgroundBlurEnabled', '0', '0', 430, '是否模糊背景'),
    ('youban_publish', 'antiScan', '背景纹理叠加', 'int', 'backgroundTextureEnabled', '0', '0', 440, '是否叠加背景纹理'),
    ('youban_publish', 'antiScan', '背景纹理预设', 'string', 'backgroundTexturePreset', 'rabbit', 'rabbit', 445, '背景纹理预设 rabbit/heart/dot/grid'),
    ('youban_publish', 'antiScan', '素材库背景贴图', 'string', 'backgroundTextureImage', '', '', 446, '素材库背景贴图地址，留空使用预设'),
    ('youban_publish', 'antiScan', '内容遮挡', 'int', 'maskEnabled', '0', '0', 450, '是否启用二维码或贴图遮挡'),
    ('youban_publish', 'antiScan', '打码方式', 'string', 'maskMode', 'qr', 'qr', 460, '打码方式：qr二维码模式，sticker贴图模式'),
    ('youban_publish', 'antiScan', '打码数量', 'int', 'maskCount', '1', '1', 470, '同一张图打码数量，最多3个'),
    ('youban_publish', 'antiScan', '二维码文案', 'string', 'qrText', '', '', 480, '二维码模式的展示文案'),
    ('youban_publish', 'antiScan', '贴图图片', 'string', 'stickerImage', '', '', 490, '贴图模式使用的正方形贴图'),
    ('youban_publish', 'antiScan', '手动遮挡素材', 'string', 'maskItemsJson', '[]', '[]', 495, '手动摆放二维码或贴图 JSON'),
    ('youban_publish', 'antiScan', '贴图透明度', 'int', 'stickerOpacity', '18', '18', 500, '防扫图贴图透明度，1-100'),
    ('youban_publish', 'antiScan', '水印开关', 'int', 'watermarkEnabled', '0', '0', 510, '是否启用背景水印'),
    ('youban_publish', 'antiScan', '资料编号水印', 'int', 'profileNoWatermarkEnabled', '0', '0', 520, '是否叠加资料编号水印'),
    ('youban_publish', 'antiScan', '水印字体大小', 'int', 'watermarkFontSize', '22', '22', 530, '水印字体大小，12-56'),
    ('youban_publish', 'antiScan', '水印透明度', 'int', 'watermarkOpacity', '28', '28', 540, '水印透明度，5-80'),
    ('youban_publish', 'antiScan', '水印文案', 'string', 'watermarkText', '', '', 550, '防扫图水印文案'),
    ('youban_publish', 'antiScan', '贴纸文案', 'string', 'stickerText', '', '', 560, '防扫图贴纸文案'),
    ('youban_publish', 'antiScan', '噪点扰动', 'int', 'noiseEnabled', '0', '0', 570, '是否启用轻微噪点扰动'),
    ('youban_publish', 'antiScan', '噪点强度', 'int', 'noiseStrength', '18', '18', 580, '噪点扰动强度'),
    ('youban_publish', 'antiScan', '压缩重采样', 'int', 'compressionEnabled', '0', '0', 590, '是否启用压缩重采样'),
    ('youban_publish', 'antiScan', '输出质量', 'int', 'compressionQuality', '82', '82', 600, '压缩重采样输出质量'),
    ('youban_publish', 'antiScan', 'JPEG质量控制', 'int', 'jpegQualityControlEnabled', '0', '0', 610, '是否启用 JPEG 质量控制'),
    ('youban_publish', 'antiScan', '色彩轻扰动', 'int', 'colorJitterEnabled', '0', '0', 620, '是否启用色彩轻扰动'),
    ('youban_publish', 'antiScan', '色彩扰动强度', 'int', 'colorJitterStrength', '12', '12', 630, '色彩扰动强度'),
    ('youban_publish', 'antiScan', '锐化模糊微扰', 'int', 'sharpenBlurEnabled', '0', '0', 640, '是否启用锐化或模糊微扰'),
    ('youban_publish', 'antiScan', '微扰方式', 'string', 'sharpenBlurMode', 'blur', 'blur', 650, 'blur 或 sharpen'),
    ('youban_publish', 'antiScan', '微扰强度', 'int', 'sharpenBlurStrength', '8', '8', 660, '锐化模糊微扰强度')
) AS seed("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip")
WHERE NOT EXISTS (
  SELECT 1 FROM "hg_sys_addons_config" c
  WHERE c."addon_name" = seed."addon_name"
    AND c."group" = seed."group"
    AND c."key" = seed."key"
);
DELETE FROM "hg_sys_addons_config"
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'publish'
  AND "key" IN ('cyclePublishEnabled', 'cyclePublishDays', 'cyclePublishTime');

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'account'
  AND "key" IN ('defaultRoleId', 'defaultDeptId');

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'antiScan'
  AND "key" IN ('enabled', 'mode', 'method', 'stickerStyle', 'stickerDensity', 'qrMaskEnabled', 'qrCustomStickerEnabled', 'qrCount', 'customStickerEnabled');

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'autoDelete'
  AND "key" IN ('enabled', 'autoDeleteEnabled', 'botIds');

ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "avatar_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_telegram" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_wechat" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_phone" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_other" text;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "follow_approval_required" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "public_follow_enabled" smallint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_source" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_type" varchar(32) NOT NULL DEFAULT 'bot',
  "title" varchar(255) NOT NULL DEFAULT '',
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_username" varchar(128) NOT NULL DEFAULT '',
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "follow_account_id" bigint NOT NULL DEFAULT 0,
  "collect_enabled" smallint NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "event_total" bigint NOT NULL DEFAULT 0,
  "success_total" bigint NOT NULL DEFAULT 0,
  "failed_total" bigint NOT NULL DEFAULT 0,
  "last_event_at" timestamp DEFAULT NULL,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_owner" ON "hg_youban_publish_collect_source" ("tenant_id", "account_id", "source_type", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_bot_chat" ON "hg_youban_publish_collect_source" ("bot_id", "source_chat_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "global_enabled" smallint NOT NULL DEFAULT 0,
  "review_enabled" smallint NOT NULL DEFAULT 0,
  "dedupe_enabled" smallint NOT NULL DEFAULT 1,
  "dedupe_days" integer NOT NULL DEFAULT 7,
  "full_match_enabled" smallint NOT NULL DEFAULT 0,
  "block_link" smallint NOT NULL DEFAULT 1,
  "block_username" smallint NOT NULL DEFAULT 1,
  "block_plain_text" smallint NOT NULL DEFAULT 1,
  "header_enabled" smallint NOT NULL DEFAULT 0,
  "header_markdown" text,
  "footer_enabled" smallint NOT NULL DEFAULT 0,
  "footer_markdown" text,
  "sort" integer NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_owner" ON "hg_youban_publish_collect_rule" ("tenant_id", "account_id", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_global" ON "hg_youban_publish_collect_rule" ("tenant_id", "global_enabled", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_rule_channel" ON "hg_youban_publish_collect_rule_channel" ("rule_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_channel_owner" ON "hg_youban_publish_collect_rule_channel" ("tenant_id", "account_id", "rule_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule_item" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "item_type" varchar(32) NOT NULL DEFAULT '',
  "value" text,
  "replacement" text,
  "sort" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_item_rule" ON "hg_youban_publish_collect_rule_item" ("rule_id", "item_type", "sort", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_item_owner" ON "hg_youban_publish_collect_rule_item" ("tenant_id", "account_id", "rule_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_source_rule" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "sort" integer NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_source_rule" ON "hg_youban_publish_collect_source_rule" ("source_id", "rule_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_rule_source" ON "hg_youban_publish_collect_source_rule" ("tenant_id", "source_id", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "source_type" varchar(32) NOT NULL DEFAULT '',
  "bot_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_id" bigint NOT NULL DEFAULT 0,
  "source_grouped_id" varchar(128) NOT NULL DEFAULT '',
  "source_unique_key" varchar(255) NOT NULL DEFAULT '',
  "material_role" varchar(16) NOT NULL DEFAULT 'pending',
  "material_parent_event_id" bigint NOT NULL DEFAULT 0,
  "material_group_status" varchar(32) NOT NULL DEFAULT 'pending',
  "raw_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "text_hash" varchar(64) NOT NULL DEFAULT '',
  "dedupe_key" varchar(128) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "received_at" timestamp DEFAULT NULL,
  "processed_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_event_unique" ON "hg_youban_publish_collect_event" ("source_unique_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_source" ON "hg_youban_publish_collect_event" ("tenant_id", "source_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_chat" ON "hg_youban_publish_collect_event" ("source_chat_id", "source_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_order" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "source_id", "source_chat_id", "source_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_material" ON "hg_youban_publish_collect_event" ("source_id", "source_chat_id", "material_role", "material_parent_event_id", "source_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_dedupe" ON "hg_youban_publish_collect_event" ("tenant_id", "dedupe_key", "created_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_text_hash" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "text_hash", "received_at", "id");

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
  "source_kind" varchar(32) NOT NULL DEFAULT '',
  "source_media_id" bigint NOT NULL DEFAULT 0,
  "source_access_hash" bigint NOT NULL DEFAULT 0,
  "source_file_reference" bytea,
  "source_thumb_size" varchar(32) NOT NULL DEFAULT '',
  "source_mime_type" varchar(128) NOT NULL DEFAULT '',
  "source_dc_id" integer NOT NULL DEFAULT 0,
  "source_size" bigint NOT NULL DEFAULT 0,
  "file_md5" varchar(64) NOT NULL DEFAULT '',
  "file_phash" varchar(128) NOT NULL DEFAULT '',
  "meta_json" text,
  "sort_index" integer NOT NULL DEFAULT 0,
  "cache_status" varchar(32) NOT NULL DEFAULT 'pending',
  "download_duration_ms" bigint NOT NULL DEFAULT 0,
  "download_bytes" bigint NOT NULL DEFAULT 0,
  "download_attempts" integer NOT NULL DEFAULT 0,
  "cache_hit" smallint NOT NULL DEFAULT 0,
  "download_error_type" varchar(64) NOT NULL DEFAULT '',
  "next_retry_at" timestamp DEFAULT NULL,
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_event" ON "hg_youban_publish_collect_event_media" ("event_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_owner" ON "hg_youban_publish_collect_event_media" ("tenant_id", "source_id", "cache_status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_source" ON "hg_youban_publish_collect_event_media" ("source_chat_id", "source_message_id", "source_media_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_file" ON "hg_youban_publish_collect_event_media" ("source_file_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_cache" ON "hg_youban_publish_collect_event_media" ("cache_status", "updated_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_event_retry" ON "hg_youban_publish_collect_event_media" ("event_id", "cache_status", "next_retry_at", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_media_stat" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT '',
  "media_total" integer NOT NULL DEFAULT 0,
  "success_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "pending_count" integer NOT NULL DEFAULT 0,
  "cache_hit_count" integer NOT NULL DEFAULT 0,
  "retry_count" integer NOT NULL DEFAULT 0,
  "bytes" bigint NOT NULL DEFAULT 0,
  "duration_ms" bigint NOT NULL DEFAULT 0,
  "p50_ms" bigint NOT NULL DEFAULT 0,
  "p95_ms" bigint NOT NULL DEFAULT 0,
  "throughput_mbps" numeric(18,4) NOT NULL DEFAULT 0,
  "success_rate" numeric(8,5) NOT NULL DEFAULT 0,
  "failure_rate" numeric(8,5) NOT NULL DEFAULT 0,
  "error_summary_json" text,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_collect_media_stat_event" UNIQUE ("event_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_media_stat_owner" ON "hg_youban_publish_collect_media_stat" ("tenant_id", "account_id", "tg_account_id", "created_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_media_stat_source" ON "hg_youban_publish_collect_media_stat" ("source_id", "created_at");

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
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_event" ON "hg_youban_publish_collect_event_log" ("event_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_owner" ON "hg_youban_publish_collect_event_log" ("tenant_id", "account_id", "created_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_stage" ON "hg_youban_publish_collect_event_log" ("event_id", "stage", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_content" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "first_event_id" bigint NOT NULL DEFAULT 0,
  "last_event_id" bigint NOT NULL DEFAULT 0,
  "source_type" varchar(32) NOT NULL DEFAULT '',
  "raw_text" text,
  "normalized_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "text_hash" varchar(64) NOT NULL DEFAULT '',
  "dedupe_key" varchar(128) NOT NULL DEFAULT '',
  "duplicate_total" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'active',
  "first_seen_at" timestamp DEFAULT NULL,
  "previous_seen_at" timestamp DEFAULT NULL,
  "last_seen_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_content_dedupe" ON "hg_youban_publish_collect_content" ("tenant_id", "account_id", "dedupe_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_content_text" ON "hg_youban_publish_collect_content" ("tenant_id", "account_id", "text_hash", "first_seen_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_content_seen" ON "hg_youban_publish_collect_content" ("tenant_id", "account_id", "last_seen_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_content_previous" ON "hg_youban_publish_collect_content" ("tenant_id", "account_id", "previous_seen_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_review" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "raw_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "review_reason" varchar(500) NOT NULL DEFAULT '',
  "reviewed_by" bigint NOT NULL DEFAULT 0,
  "reviewed_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_review_owner" ON "hg_youban_publish_collect_review" ("tenant_id", "account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_review_event" ON "hg_youban_publish_collect_review" ("event_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_dispatch" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "review_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "match_json" text,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_event" ON "hg_youban_publish_collect_dispatch" ("event_id", "rule_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_owner" ON "hg_youban_publish_collect_dispatch" ("tenant_id", "account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_dispatch_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_dispatch_channel" ON "hg_youban_publish_collect_dispatch_channel" ("dispatch_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_channel_owner" ON "hg_youban_publish_collect_dispatch_channel" ("tenant_id", "account_id", "dispatch_id");

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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_account_follow" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "follower_account_id" bigint NOT NULL DEFAULT 0,
  "following_account_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "approval_required_snapshot" smallint NOT NULL DEFAULT 0,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "blocked_by" bigint NOT NULL DEFAULT 0,
  "approved_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_account_follow_pair" ON "hg_youban_publish_account_follow" ("follower_account_id", "following_account_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_account_follow_follower" ON "hg_youban_publish_account_follow" ("tenant_id", "follower_account_id", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_account_follow_following" ON "hg_youban_publish_account_follow" ("tenant_id", "following_account_id", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cycle_run" (
  "id" BIGSERIAL PRIMARY KEY,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "scheduled_at" timestamp DEFAULT NULL,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "error_message" text,
  "retry_count" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "channel_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "cursor_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "total_count" integer NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "queued_count" integer NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_channel_profile" (
  "id" bigserial PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "last_job_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(16) NOT NULL DEFAULT 'active',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_channel_profile" ON "hg_youban_publish_channel_profile" ("channel_id", "profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_profile_scan" ON "hg_youban_publish_channel_profile" ("channel_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_run_plan" ON "hg_youban_publish_cycle_run" ("plan_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_run_owner" ON "hg_youban_publish_cycle_run" ("tenant_id", "account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cycle_run_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "run_id" bigint NOT NULL DEFAULT 0,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "level" varchar(16) NOT NULL DEFAULT 'info',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "context_json" jsonb,
  "created_at" timestamp DEFAULT NULL
);
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run_log" ADD COLUMN IF NOT EXISTS "channel_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_run_log_run" ON "hg_youban_publish_cycle_run_log" ("run_id", "id");
DELETE FROM "hg_youban_publish_cycle_run_log";
DELETE FROM "hg_youban_publish_cycle_run";
DROP TABLE IF EXISTS "hg_youban_publish_cycle_plan";

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_template" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "serial_no" varchar(32) NOT NULL DEFAULT '',
  "push_mode" varchar(16) NOT NULL DEFAULT 'bot',
  "source_message_record_id" bigint NOT NULL DEFAULT 0,
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
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_tpl_serial" ON "hg_youban_publish_message_template" ("serial_no");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "template_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "source_message_record_id" bigint NOT NULL DEFAULT 0,
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
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_sender_tenant" ON "hg_youban_publish_message_listen_sender" ("tenant_id", "tg_account_id");

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
CREATE INDEX IF NOT EXISTS idx_ybp_quick_plan_owner ON hg_youban_publish_quick_push_plan (tenant_id,status,id);

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
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_lookup" ON "hg_youban_publish_media_phash_bucket" ("tenant_id", "media_type", "bucket_pos", "bucket_value", "account_id", "profile_id", "task_id", "media_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_search" ON "hg_youban_publish_media_phash_bucket" ("tenant_id", "media_type", "bucket_pos", "bucket_value") INCLUDE ("media_id", "profile_id", "account_id", "hash_value");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_profile_id" ON "hg_youban_publish_media_phash_bucket" ("profile_id");

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
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_lsh_lookup" ON "hg_youban_publish_media_phash_lsh" ("tenant_id", "media_type", "bucket_pos", "bucket_value", "account_id", "profile_id", "media_id") INCLUDE ("hash_value");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_lsh_profile_id" ON "hg_youban_publish_media_phash_lsh" ("profile_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_success_record" (
  "id" BIGSERIAL PRIMARY KEY, "job_id" bigint NOT NULL DEFAULT 0, "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0, "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "bot_id" bigint NOT NULL DEFAULT 0, "operation_no" varchar(128) NOT NULL DEFAULT '',
  "target_chat_id" varchar(128) NOT NULL DEFAULT '', "action" varchar(32) NOT NULL DEFAULT 'profile_publish',
  "status" varchar(16) NOT NULL DEFAULT 'success', "message" varchar(255) NOT NULL DEFAULT '', "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_success_record_job" ON "hg_youban_publish_success_record" ("job_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_owner" ON "hg_youban_publish_success_record" ("tenant_id", "account_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_profile" ON "hg_youban_publish_success_record" ("profile_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_monitor" ON "hg_youban_publish_success_record" ("created_at", "status", "profile_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "level" integer NOT NULL DEFAULT 0,
  "status" integer NOT NULL DEFAULT 2,
  "opened_at" timestamp DEFAULT NULL,
  "expired_at" timestamp DEFAULT NULL,
  "remark" text NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tenant_vip_tenant" ON "hg_youban_publish_tenant_vip" ("tenant_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_vip_expired" ON "hg_youban_publish_tenant_vip" ("status", "expired_at", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "operator_id" bigint NOT NULL DEFAULT 0,
  "source" varchar(32) NOT NULL DEFAULT '',
  "action" varchar(32) NOT NULL DEFAULT '',
  "before_status" integer NOT NULL DEFAULT 0,
  "before_level" integer NOT NULL DEFAULT 0,
  "before_expired_at" timestamp DEFAULT NULL,
  "after_status" integer NOT NULL DEFAULT 0,
  "after_level" integer NOT NULL DEFAULT 0,
  "after_expired_at" timestamp DEFAULT NULL,
  "remark" text NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_vip_log_tenant" ON "hg_youban_publish_tenant_vip_log" ("tenant_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip_coupon" (
  "id" BIGSERIAL PRIMARY KEY,
  "code" varchar(64) NOT NULL DEFAULT '',
  "use_type" varchar(16) NOT NULL DEFAULT 'single',
  "amount" numeric(10,2) NOT NULL DEFAULT 0,
  "total_count" integer NOT NULL DEFAULT 1,
  "used_count" integer NOT NULL DEFAULT 0,
  "status" integer NOT NULL DEFAULT 1,
  "remark" text NOT NULL DEFAULT '',
  "expired_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tenant_vip_coupon_code" ON "hg_youban_publish_tenant_vip_coupon" ("code") WHERE "deleted_at" IS NULL;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_vip_event" (
  "id" BIGSERIAL PRIMARY KEY,
  "event_key" varchar(160) NOT NULL DEFAULT '',
  "event_type" varchar(48) NOT NULL DEFAULT '',
  "activity_code" varchar(64) NOT NULL DEFAULT '',
  "activity_generation" integer NOT NULL DEFAULT 1,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "trigger_tenant_id" bigint NOT NULL DEFAULT 0,
  "trigger_account_id" bigint NOT NULL DEFAULT 0,
  "reference_type" varchar(32) NOT NULL DEFAULT '',
  "reference_id" varchar(64) NOT NULL DEFAULT '',
  "change_days" integer NOT NULL DEFAULT 0,
  "before_expired_at" timestamp DEFAULT NULL,
  "after_expired_at" timestamp DEFAULT NULL,
  "notify_status" varchar(16) NOT NULL DEFAULT 'pending',
  "notify_retry_count" integer NOT NULL DEFAULT 0,
  "notify_next_retry_at" timestamp DEFAULT NULL,
  "notified_at" timestamp DEFAULT NULL,
  "error_message" text NOT NULL DEFAULT '',
  "remark" text NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_vip_event_key" ON "hg_youban_publish_tenant_vip_event" ("event_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_vip_event_activity" ON "hg_youban_publish_tenant_vip_event" ("activity_code", "activity_generation", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_vip_event_tenant" ON "hg_youban_publish_tenant_vip_event" ("tenant_id", "event_type", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_vip_event_notify" ON "hg_youban_publish_tenant_vip_event" ("notify_status", "notify_next_retry_at", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_activity_generation" (
  "id" BIGSERIAL PRIMARY KEY,
  "activity_code" varchar(64) NOT NULL DEFAULT '',
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "generation" integer NOT NULL DEFAULT 1,
  "reset_reason" text NOT NULL DEFAULT '',
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_activity_generation" ON "hg_youban_publish_activity_generation" ("activity_code", "tenant_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_full_push_batch" (
  "id" BIGSERIAL PRIMARY KEY, "batch_no" varchar(128) NOT NULL, "tenant_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "requested_by" bigint NOT NULL DEFAULT 0,
  "snapshot_max_profile_id" bigint NOT NULL DEFAULT 0, "cursor_profile_id" bigint NOT NULL DEFAULT 0,
  "total_count" integer NOT NULL DEFAULT 0, "queued_count" integer NOT NULL DEFAULT 0, "retry_count" integer NOT NULL DEFAULT 0,
  "status" varchar(16) NOT NULL DEFAULT 'pending', "active_key" varchar(64) DEFAULT NULL, "error_message" text,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL, "finished_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_full_push_batch_no" ON "hg_youban_publish_full_push_batch" ("batch_no");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_full_push_active" ON "hg_youban_publish_full_push_batch" ("active_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_full_push_schedule" ON "hg_youban_publish_full_push_batch" ("status", "id");
