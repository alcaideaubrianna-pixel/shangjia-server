ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "next_retry_at" timestamp DEFAULT NULL;

CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle_due"
  ON "hg_youban_publish_tg_job" ("next_cycle_at", "id")
  INCLUDE ("tenant_id", "account_id", "profile_id", "channel_id", "cycle_days")
  WHERE "cycle_enabled" = 1 AND "status" IN ('sent','superseded') AND "next_cycle_at" IS NOT NULL;

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
ALTER TABLE "hg_youban_publish_tenant_vip_event" ADD COLUMN IF NOT EXISTS "activity_code" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tenant_vip_event" ADD COLUMN IF NOT EXISTS "activity_generation" integer NOT NULL DEFAULT 1;
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

CREATE INDEX IF NOT EXISTS "idx_ybp_daily_stat_date" ON "hg_youban_publish_daily_stat" ("stat_date", "account_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_monitor" ON "hg_youban_publish_success_record" ("created_at", "status", "profile_id");
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "must_send" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "processing_status" varchar(16) NOT NULL DEFAULT 'ready';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "processing_error" text;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "processing_started_at" timestamp DEFAULT NULL;
UPDATE "hg_youban_publish_media" SET "must_send" = 0 WHERE EXISTS (SELECT 1 FROM "information_schema"."columns" WHERE "table_schema" = current_schema() AND "table_name" = 'hg_youban_publish_media' AND "column_name" = 'must_send' AND "column_default" LIKE '1%');
ALTER TABLE "hg_youban_publish_media" ALTER COLUMN "must_send" SET DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "send_phase" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "reconcile_count" integer NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_tenant_channel_status" ON "hg_youban_publish_tg_job" ("tenant_id", "channel_id", "status", "id");
DELETE FROM "hg_youban_publish_tg_message" older USING "hg_youban_publish_tg_message" newer WHERE older."job_id" = newer."job_id" AND older."tg_message_id" = newer."tg_message_id" AND older."id" < newer."id";
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_message_job_message" ON "hg_youban_publish_tg_message" ("job_id", "tg_message_id");
ALTER TABLE "hg_youban_publish_tg_channel_stat" ALTER COLUMN "last_error_message" TYPE text;
ALTER TABLE "hg_youban_publish_tg_bot_stat" ALTER COLUMN "last_error_message" TYPE text;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_profile_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "is_manual" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_profile_channel" UNIQUE ("tenant_id","profile_id","channel_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_channel_owner" ON "hg_youban_publish_profile_channel" ("tenant_id","account_id","profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_channel_channel" ON "hg_youban_publish_profile_channel" ("tenant_id","channel_id","profile_id");

ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "bot_collect_scope" varchar(16) NOT NULL DEFAULT 'chat';
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_bot_scope" ON "hg_youban_publish_collect_source" ("bot_id", "bot_collect_scope", "source_chat_id", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_bot_message_source" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "received_bot_id" bigint NOT NULL DEFAULT 0,
  "chat_id" varchar(64) NOT NULL DEFAULT '',
  "message_id" bigint NOT NULL DEFAULT 0,
  "media_group_id" varchar(128) NOT NULL DEFAULT '',
  "sender_user_id" bigint NOT NULL DEFAULT 0,
  "sender_username" varchar(255) NOT NULL DEFAULT '',
  "sender_chat_id" bigint NOT NULL DEFAULT 0,
  "sender_chat_title" varchar(255) NOT NULL DEFAULT '',
  "reply_to_message_id" bigint NOT NULL DEFAULT 0,
  "reply_job_id" bigint NOT NULL DEFAULT 0,
  "reply_profile_id" bigint NOT NULL DEFAULT 0,
  "reply_purpose" varchar(16) NOT NULL DEFAULT '',
  "update_type" varchar(64) NOT NULL DEFAULT '',
  "message_text" text,
  "received_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "deleted_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_bot_message_source" UNIQUE ("chat_id", "message_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_message_source_chat_time" ON "hg_youban_publish_bot_message_source" ("chat_id", "received_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_message_source_reply" ON "hg_youban_publish_bot_message_source" ("reply_job_id", "reply_profile_id", "reply_to_message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_message_source_sender" ON "hg_youban_publish_bot_message_source" ("sender_user_id", "sender_chat_id");
DELETE FROM "hg_youban_publish_bot_message_source" older USING "hg_youban_publish_bot_message_source" newer
WHERE older."chat_id" = newer."chat_id" AND older."message_id" = newer."message_id" AND older."id" > newer."id";
ALTER TABLE "hg_youban_publish_bot_message_source" DROP CONSTRAINT IF EXISTS "uk_ybp_bot_message_source";
ALTER TABLE "hg_youban_publish_bot_message_source" ADD CONSTRAINT "uk_ybp_bot_message_source" UNIQUE ("chat_id", "message_id");
