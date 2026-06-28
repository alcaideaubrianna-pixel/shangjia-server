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

CREATE TABLE IF NOT EXISTS "hg_youban_publish_account" (
  "id" BIGSERIAL PRIMARY KEY,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "admin_member_id" bigint NOT NULL DEFAULT 0,
  "parent_id" bigint NOT NULL DEFAULT 0,
  "account_type" varchar(32) NOT NULL DEFAULT 'uploader',
  "nickname" varchar(128) NOT NULL DEFAULT '',
  "username" varchar(128) NOT NULL DEFAULT '',
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
CREATE INDEX IF NOT EXISTS "idx_ybp_account_merchant" ON "hg_youban_publish_account" ("merchant_id", "account_type", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_account_admin_member" ON "hg_youban_publish_account" ("admin_member_id", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "client_request_id" varchar(128) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "province" varchar(64) NOT NULL DEFAULT '',
  "city" varchar(64) NOT NULL DEFAULT '',
  "plain_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
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
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_task_client_request" ON "hg_youban_publish_task" ("merchant_id", "client_request_id") WHERE "client_request_id" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_task_merchant_status" ON "hg_youban_publish_task" ("merchant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_task_account_status" ON "hg_youban_publish_task" ("account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "attachment_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT 'image',
  "name" varchar(255) NOT NULL DEFAULT '',
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "mime_type" varchar(128) NOT NULL DEFAULT '',
  "md5" varchar(64) NOT NULL DEFAULT '',
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
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_attachment" ON "hg_youban_publish_media" ("task_id", "attachment_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_sort" ON "hg_youban_publish_media" ("task_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile" ON "hg_youban_publish_media" ("profile_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_job" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "tg_message_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "retry_count" integer NOT NULL DEFAULT 0,
  "next_retry_at" timestamp DEFAULT NULL,
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_status_retry" ON "hg_youban_publish_tg_job" ("status", "next_retry_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task" ON "hg_youban_publish_tg_job" ("task_id");
