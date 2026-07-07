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
CREATE TABLE IF NOT EXISTS "hg_youban_publish_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "client_request_id" varchar(128) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "province" varchar(64) NOT NULL DEFAULT '',
  "city" varchar(64) NOT NULL DEFAULT '',
  "plain_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "channel_id_json" text,
  "customer_remark" text,
  "anti_scan_enabled" smallint NOT NULL DEFAULT 0,
  "tg_push_enabled" smallint NOT NULL DEFAULT 1,
  "tg_status" varchar(32) NOT NULL DEFAULT 'pending',
  "status" varchar(32) NOT NULL DEFAULT 'draft',
  "error_message" text,
  "submitted_at" timestamp DEFAULT NULL,
  "published_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "channel_id_json" text;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "customer_remark" text;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "anti_scan_enabled" smallint NOT NULL DEFAULT 0;
UPDATE "hg_youban_publish_task" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_task_tenant_client_request" ON "hg_youban_publish_task" ("tenant_id", "client_request_id") WHERE "client_request_id" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_task_tenant_status" ON "hg_youban_publish_task" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_task_account_status" ON "hg_youban_publish_task" ("account_id", "status", "id");

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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "attachment_id" bigint NOT NULL DEFAULT 0,
  "original_attachment_id" bigint NOT NULL DEFAULT 0,
  "edited_attachment_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT 'image',
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
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash" ON "hg_youban_publish_media" ("perceptual_hash");
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
  "id" BIGSERIAL PRIMARY KEY, "task_id" bigint NOT NULL DEFAULT 0, "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0, "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "bot_id" bigint NOT NULL DEFAULT 0, "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "tg_message_id" bigint NOT NULL DEFAULT 0, "asynq_task_id" varchar(128) NOT NULL DEFAULT '', "status" varchar(32) NOT NULL DEFAULT 'pending',
  "retry_count" integer NOT NULL DEFAULT 0, "next_retry_at" timestamp DEFAULT NULL, "sent_at" timestamp DEFAULT NULL,
  "cycle_enabled" smallint NOT NULL DEFAULT 0, "cycle_days" integer NOT NULL DEFAULT 4, "cycle_publish_time" varchar(16) NOT NULL DEFAULT '',
  "next_cycle_at" timestamp DEFAULT NULL, "error_message" text, "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "channel_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "asynq_task_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "sent_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "cycle_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "cycle_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "next_cycle_at" timestamp DEFAULT NULL;
UPDATE "hg_youban_publish_tg_job" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_job_task_channel" ON "hg_youban_publish_tg_job" ("task_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_status_retry" ON "hg_youban_publish_tg_job" ("status", "next_retry_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task" ON "hg_youban_publish_tg_job" ("task_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle" ON "hg_youban_publish_tg_job" ("cycle_enabled", "next_cycle_at", "id");

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
  "message_date" timestamp DEFAULT NULL,
  "media_group_id" varchar(128) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_msg_cache_msg" ON "hg_youban_publish_tg_message_cache" ("tenant_id", "channel_id", "tg_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_msg_cache_channel" ON "hg_youban_publish_tg_message_cache" ("tenant_id", "tg_account_id", "channel_id", "message_date");

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
  "bot_id_json" text,
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
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "is_default_selected" smallint NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" varchar(128) NOT NULL DEFAULT '',
  "access_hash" varchar(128) NOT NULL DEFAULT '',
  "channel_title" varchar(128) NOT NULL DEFAULT '',
  "channel_username" varchar(128) NOT NULL DEFAULT '',
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
    ('youban_publish', 'autoDelete', '频道自动删除开关', 'int', 'autoDeleteEnabled', '0', '0', 200, '是否启用频道自动删除'),
    ('youban_publish', 'autoDelete', '自动删除 Bot ID', '[]int64', 'botIds', '[]', '[]', 210, '执行自动删除的 Bot ID 列表'),
    ('youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '[]', '[]', 220, '命中后自动删除的关键词列表'),
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
  AND "key" = 'enabled';

ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "avatar_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_telegram" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_wechat" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_phone" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "contact_other" text;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "follow_approval_required" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "public_follow_enabled" smallint NOT NULL DEFAULT 1;

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
  "target_channel_id_json" text,
  "bot_id_json" text,
  "backup_channel_id" bigint NOT NULL DEFAULT 0,
  "review_enabled" smallint NOT NULL DEFAULT 0,
  "dedupe_enabled" smallint NOT NULL DEFAULT 1,
  "dedupe_days" integer NOT NULL DEFAULT 7,
  "keyword_json" text,
  "tag_json" text,
  "replace_json" text,
  "block_text_json" text,
  "block_link" smallint NOT NULL DEFAULT 1,
  "block_username" smallint NOT NULL DEFAULT 1,
  "block_plain_text" smallint NOT NULL DEFAULT 1,
  "min_media_count_enabled" smallint NOT NULL DEFAULT 1,
  "min_media_count" integer NOT NULL DEFAULT 2,
  "show_unique_no" smallint NOT NULL DEFAULT 0,
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
  "raw_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "media_json" text,
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
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_dedupe" ON "hg_youban_publish_collect_event" ("tenant_id", "dedupe_key", "created_at");

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
  "media_signature" varchar(128) NOT NULL DEFAULT '',
  "media_json" text,
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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_content_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "content_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(32) NOT NULL DEFAULT '',
  "source_file_id" varchar(255) NOT NULL DEFAULT '',
  "source_unique_key" varchar(255) NOT NULL DEFAULT '',
  "file_md5" varchar(64) NOT NULL DEFAULT '',
  "file_phash" varchar(128) NOT NULL DEFAULT '',
  "sort_index" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'active',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_content_media_file" ON "hg_youban_publish_collect_content_media" ("content_id", "source_file_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_content_media_content" ON "hg_youban_publish_collect_content_media" ("tenant_id", "account_id", "content_id", "sort_index");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_content_media_hash" ON "hg_youban_publish_collect_content_media" ("tenant_id", "file_md5", "file_phash");

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
  "media_json" text,
  "target_channel_id_json" text,
  "bot_id_json" text,
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
  "target_channel_id_json" text,
  "bot_id_json" text,
  "match_json" text,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_event" ON "hg_youban_publish_collect_dispatch" ("event_id", "rule_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_owner" ON "hg_youban_publish_collect_dispatch" ("tenant_id", "account_id", "status", "id");

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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cycle_plan" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "enabled" smallint NOT NULL DEFAULT 0,
  "interval_seconds" integer NOT NULL DEFAULT 345600,
  "publish_time" varchar(16) NOT NULL DEFAULT '',
  "next_run_at" timestamp DEFAULT NULL,
  "last_run_at" timestamp DEFAULT NULL,
  "last_run_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'active',
  "source" varchar(32) NOT NULL DEFAULT '',
  "locked_at" timestamp DEFAULT NULL,
  "last_error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_cycle_plan_profile" ON "hg_youban_publish_cycle_plan" ("tenant_id", "account_id", "profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_plan_due" ON "hg_youban_publish_cycle_plan" ("enabled", "status", "next_run_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_plan_account" ON "hg_youban_publish_cycle_plan" ("tenant_id", "account_id", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cycle_run" (
  "id" BIGSERIAL PRIMARY KEY,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
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
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_run_plan" ON "hg_youban_publish_cycle_run" ("plan_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_run_owner" ON "hg_youban_publish_cycle_run" ("tenant_id", "account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cycle_run_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "run_id" bigint NOT NULL DEFAULT 0,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "level" varchar(16) NOT NULL DEFAULT 'info',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "context_json" jsonb,
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_cycle_run_log_run" ON "hg_youban_publish_cycle_run_log" ("run_id", "id");
