-- Interactive upgrades must remain short. Historical cleanup and backfills belong in upgrade.pgsql.sql.
ALTER TABLE "hg_youban_publish_account_setting"
  ADD COLUMN IF NOT EXISTS "group_push_enabled" smallint NOT NULL DEFAULT 0;

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

CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_tenant_created" ON "hg_youban_publish_note_index" ("tenant_id", "created_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_account_created" ON "hg_youban_publish_note_index" ("account_id", "created_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_tenant_status_created" ON "hg_youban_publish_note_index" ("tenant_id", "status", "created_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_account_status_created" ON "hg_youban_publish_note_index" ("account_id", "status", "created_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_scope_created" ON "hg_youban_publish_note_index" ("tenant_id", "account_id", "created_at" DESC, "id" DESC, "profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_tenant_published" ON "hg_youban_publish_note_index" ("tenant_id", (COALESCE("published_at", '1970-01-01'::timestamp)) DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_note_index_account_published" ON "hg_youban_publish_note_index" ("account_id", (COALESCE("published_at", '1970-01-01'::timestamp)) DESC, "id" DESC) WHERE "deleted_at" IS NULL;


CREATE TABLE IF NOT EXISTS "hg_youban_publish_image_quota" (
  "id" BIGSERIAL PRIMARY KEY, "tenant_id" bigint NOT NULL DEFAULT 0,
  "monthly_limit" bigint NOT NULL DEFAULT 5000, "monthly_used" bigint NOT NULL DEFAULT 0,
  "purchased_remaining" bigint NOT NULL DEFAULT 0, "period" varchar(7) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL, UNIQUE ("tenant_id")
);
CREATE TABLE IF NOT EXISTS "hg_youban_publish_image_quota_ledger" (
  "id" BIGSERIAL PRIMARY KEY, "tenant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0,
  "event_type" varchar(24) NOT NULL DEFAULT '', "reference_key" varchar(160) NOT NULL DEFAULT '',
  "change_count" bigint NOT NULL DEFAULT 0, "remark" varchar(500) NOT NULL DEFAULT '', "created_at" timestamp DEFAULT NULL,
  UNIQUE ("reference_key")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_image_quota_ledger_tenant" ON "hg_youban_publish_image_quota_ledger" ("tenant_id", "id");
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "anti_scan_mode" varchar(32) NOT NULL DEFAULT 'lightweight';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "anti_scan_background_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "anti_scan_background_name" varchar(255) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_profile_profile" ON "hg_youban_publish_channel_profile" ("profile_id", "channel_id");
ALTER TABLE "hg_youban_publish_anti_scan_cache" ADD COLUMN IF NOT EXISTS "media_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_anti_scan_cache" ADD COLUMN IF NOT EXISTS "image_width" integer NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_anti_scan_cache" ADD COLUMN IF NOT EXISTS "image_height" integer NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_media" ON "hg_youban_publish_anti_scan_cache" ("media_id", "cloud_raw_saved");
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_media_provider" ON "hg_youban_publish_anti_scan_cache" ("media_id", "provider", "cloud_raw_saved");
CREATE INDEX IF NOT EXISTS "idx_ybp_anti_scan_image_provider" ON "hg_youban_publish_anti_scan_cache" ("image_hash", "provider", "cloud_raw_saved");
ALTER TABLE "hg_youban_publish_message_media" ADD COLUMN IF NOT EXISTS "original_file_url" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_message_media" ADD COLUMN IF NOT EXISTS "original_storage_path" varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_message_media" ADD COLUMN IF NOT EXISTS "edit_status" varchar(16) NOT NULL DEFAULT 'raw';

-- Event-driven Telegram delivery receipts. Metadata only; no historical backfill.
ALTER TABLE "hg_youban_publish_bot_message_source" ADD COLUMN IF NOT EXISTS "update_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_bot_message_source" ADD COLUMN IF NOT EXISTS "tg_file_id" varchar(255) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_bot_message_source" ADD COLUMN IF NOT EXISTS "tg_file_unique_id" varchar(255) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_bot_message_source" ADD COLUMN IF NOT EXISTS "matched_attempt_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_bot_message_source" ADD COLUMN IF NOT EXISTS "process_status" varchar(16) NOT NULL DEFAULT 'pending';
CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_attempt" (
  "id" BIGSERIAL PRIMARY KEY, "job_id" bigint NOT NULL DEFAULT 0, "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0, "channel_id" bigint NOT NULL DEFAULT 0, "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "phase" varchar(16) NOT NULL DEFAULT '', "chunk_index" integer NOT NULL DEFAULT 0, "attempt_no" integer NOT NULL DEFAULT 1,
  "attempt_token" varchar(128) NOT NULL DEFAULT '', "expected_count" integer NOT NULL DEFAULT 0, "confirmed_count" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'sending', "response_received" smallint NOT NULL DEFAULT 0,
  "webhook_deadline" timestamp DEFAULT NULL, "error_message" text, "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_attempt_token" ON "hg_youban_publish_tg_attempt" ("attempt_token");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_attempt_scope" ON "hg_youban_publish_tg_attempt" ("job_id", "phase", "chunk_index", "attempt_no");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_attempt_waiting" ON "hg_youban_publish_tg_attempt" ("status", "webhook_deadline", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_attempt_channel" ON "hg_youban_publish_tg_attempt" ("target_chat_id", "status", "created_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_sending_recovery" ON "hg_youban_publish_tg_job" ("updated_at", "id") WHERE "status" = 'sending';
CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_attempt_message" (
  "id" BIGSERIAL PRIMARY KEY, "attempt_id" bigint NOT NULL DEFAULT 0, "job_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '', "message_id" bigint NOT NULL DEFAULT 0,
  "source" varchar(16) NOT NULL DEFAULT '', "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_attempt_message" ON "hg_youban_publish_tg_attempt_message" ("attempt_id", "message_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_attempt_message_job" ON "hg_youban_publish_tg_attempt_message" ("job_id", "attempt_id");
ALTER TABLE "hg_youban_publish_tg_attempt_message" ADD COLUMN IF NOT EXISTS "message_payload" text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_outbox" (
  "id" BIGSERIAL PRIMARY KEY, "job_id" bigint NOT NULL, "event_type" varchar(64) NOT NULL,
  "dedupe_key" varchar(160) NOT NULL, "status" varchar(16) NOT NULL DEFAULT 'pending',
  "available_at" timestamp NOT NULL, "lease_owner" varchar(160) NOT NULL DEFAULT '',
  "lease_expires_at" timestamp DEFAULT NULL, "attempt_count" integer NOT NULL DEFAULT 0,
  "last_error" varchar(255) NOT NULL DEFAULT '', "delivered_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_outbox_dedupe" ON "hg_youban_publish_tg_outbox" ("dedupe_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_outbox_due" ON "hg_youban_publish_tg_outbox" ("available_at", "id") WHERE "status" = 'pending';
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_outbox_lease" ON "hg_youban_publish_tg_outbox" ("lease_expires_at", "id") WHERE "status" = 'processing';
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_outbox_job" ON "hg_youban_publish_tg_outbox" ("job_id", "event_type", "id");
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "last_active_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "channel_suffix" text NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "channel_blacklisted" smallint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_channel_rule" ON "hg_youban_publish_collect_source" ("tenant_id","account_id","source_chat_id","channel_blacklisted");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_intro_fee_channel_rule" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" BIGINT NOT NULL DEFAULT 0,
	"account_id" BIGINT NOT NULL DEFAULT 0,
  "rule_id" BIGINT NOT NULL DEFAULT 0,
  "bot_id" BIGINT NOT NULL DEFAULT 0,
  "channel_id" VARCHAR(128) NOT NULL DEFAULT '',
  "channel_title" VARCHAR(255) NOT NULL DEFAULT '',
  "channel_username" VARCHAR(128) NOT NULL DEFAULT '',
  "suffix" VARCHAR(255) NOT NULL DEFAULT '',
  "status" SMALLINT NOT NULL DEFAULT 1,
  "last_seen_at" TIMESTAMP NULL,
  "created_at" TIMESTAMP NULL,
  "updated_at" TIMESTAMP NULL,
  "deleted_at" TIMESTAMP NULL
);
ALTER TABLE "hg_youban_publish_intro_fee_channel_rule" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0;
DROP INDEX IF EXISTS "uk_ybp_intro_fee_channel_rule";
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_intro_fee_channel_rule" ON "hg_youban_publish_intro_fee_channel_rule" ("tenant_id","account_id","rule_id","bot_id","channel_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_intro_fee_channel_rule_lookup_v2" ON "hg_youban_publish_intro_fee_channel_rule" ("tenant_id","account_id","rule_id","bot_id","channel_id","status");
-- Canonical collection channel metadata and scoped relationships.
CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_channel" (
  "id" BIGSERIAL PRIMARY KEY, "tenant_id" bigint NOT NULL DEFAULT 0,
  "telegram_chat_id" varchar(128) NOT NULL DEFAULT '', "chat_type" varchar(32) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '', "username" varchar(128) NOT NULL DEFAULT '',
  "invite_link" varchar(512) NOT NULL DEFAULT '', "about" text, "photo_file_id" varchar(255) NOT NULL DEFAULT '',
  "is_broadcast" smallint NOT NULL DEFAULT 0, "is_megagroup" smallint NOT NULL DEFAULT 0,
  "last_message_id" bigint NOT NULL DEFAULT 0, "last_message_at" timestamp DEFAULT NULL,
  "last_seen_at" timestamp DEFAULT NULL, "status" varchar(16) NOT NULL DEFAULT 'active',
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL, "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_channel_tenant_chat" ON "hg_youban_publish_collect_channel" ("tenant_id","telegram_chat_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_channel_search" ON "hg_youban_publish_collect_channel" ("tenant_id","status","title","username");
CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_bot_channel" (
  "id" BIGSERIAL PRIMARY KEY, "tenant_id" bigint NOT NULL DEFAULT 0, "bot_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "status" varchar(16) NOT NULL DEFAULT 'discovered',
  "bot_role" varchar(32) NOT NULL DEFAULT 'unknown', "first_seen_at" timestamp DEFAULT NULL,
  "last_seen_at" timestamp DEFAULT NULL, "removed_at" timestamp DEFAULT NULL,
  "removed_reason" varchar(255) NOT NULL DEFAULT '', "last_update_id" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL, "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_bot_channel" ON "hg_youban_publish_collect_bot_channel" ("tenant_id","bot_id","channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_bot_channel_status" ON "hg_youban_publish_collect_bot_channel" ("tenant_id","bot_id","status","last_seen_at");
CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_account_channel" (
  "id" BIGSERIAL PRIMARY KEY, "tenant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "source_id" bigint NOT NULL DEFAULT 0,
  "collect_enabled" smallint NOT NULL DEFAULT 0, "status" varchar(16) NOT NULL DEFAULT 'pending',
  "approval_status" varchar(16) NOT NULL DEFAULT 'pending', "approved_at" timestamp DEFAULT NULL,
  "disabled_at" timestamp DEFAULT NULL, "disabled_reason" varchar(255) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL, "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_account_channel" ON "hg_youban_publish_collect_account_channel" ("tenant_id","account_id","channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_account_channel_status" ON "hg_youban_publish_collect_account_channel" ("tenant_id","account_id","status","id");

ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "channel_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_channel_id" ON "hg_youban_publish_collect_source" ("tenant_id","account_id","channel_id","status");


CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_channel_profile" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "channel_id" varchar(128) NOT NULL DEFAULT '',
  "profile_id" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_channel_profile" ON "hg_youban_publish_collect_channel_profile" ("tenant_id","account_id","rule_id","channel_id","profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_channel_profile_list" ON "hg_youban_publish_collect_channel_profile" ("tenant_id","account_id","rule_id","channel_id","updated_at") WHERE "deleted_at" IS NULL;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_delivery_step" (
  "id" BIGSERIAL PRIMARY KEY, "job_id" bigint NOT NULL DEFAULT 0,
  "step_key" varchar(32) NOT NULL DEFAULT '', "status" varchar(32) NOT NULL DEFAULT 'pending',
  "attempt_id" bigint NOT NULL DEFAULT 0, "retry_count" integer NOT NULL DEFAULT 0,
  "next_retry_at" timestamp DEFAULT NULL, "error_message" text,
  "started_at" timestamp DEFAULT NULL, "confirmed_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_delivery_step_job" ON "hg_youban_publish_tg_delivery_step" ("job_id", "step_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_delivery_step_recovery" ON "hg_youban_publish_tg_delivery_step" ("status", "next_retry_at", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_process_outbox" (
  "id" BIGSERIAL PRIMARY KEY, "event_id" bigint NOT NULL, "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0, "source_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '', "status" varchar(16) NOT NULL DEFAULT 'pending',
  "available_at" timestamp NOT NULL, "lease_owner" varchar(128) NOT NULL DEFAULT '', "lease_expires_at" timestamp NULL,
  "attempt_count" integer NOT NULL DEFAULT 0, "last_error" text, "delivered_at" timestamp NULL,
  "created_at" timestamp NULL, "updated_at" timestamp NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_process_outbox_event" ON "hg_youban_publish_collect_process_outbox" ("event_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_process_outbox_due" ON "hg_youban_publish_collect_process_outbox" ("available_at", "id") WHERE "status" = 'pending';
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_process_outbox_lease" ON "hg_youban_publish_collect_process_outbox" ("lease_expires_at", "id") WHERE "status" = 'processing';
CREATE TABLE IF NOT EXISTS "hg_youban_publish_ai_ops_repair_plan" (
  "id" BIGSERIAL PRIMARY KEY, "plan_id" varchar(64) NOT NULL, "action" varchar(64) NOT NULL,
  "payload_json" text NOT NULL, "status" varchar(16) NOT NULL DEFAULT 'pending', "candidate_count" integer NOT NULL DEFAULT 0,
  "applied_count" integer NOT NULL DEFAULT 0, "failed_ids_json" text, "confirm_token_hash" varchar(128) NOT NULL DEFAULT '',
  "error_message" text, "expires_at" timestamp NOT NULL, "started_at" timestamp NULL, "finished_at" timestamp NULL,
  "created_at" timestamp NULL, "updated_at" timestamp NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_ai_ops_repair_plan" ON "hg_youban_publish_ai_ops_repair_plan" ("plan_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_ai_ops_repair_plan_status" ON "hg_youban_publish_ai_ops_repair_plan" ("status", "expires_at", "id");

ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS account_id bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_ybp_bot_account ON hg_youban_publish_bot(tenant_id,account_id,created_by,status);

ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS bot_type varchar(32) NOT NULL DEFAULT 'normal';
ALTER TABLE hg_youban_publish_tg_job ADD COLUMN IF NOT EXISTS cycle_failure_count integer NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_account_setting" ADD COLUMN IF NOT EXISTS "profile_detail_view_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_material_import_task" ADD COLUMN IF NOT EXISTS "channel_id_json" text;
ALTER TABLE "hg_youban_publish_profile_state" ADD COLUMN IF NOT EXISTS "dedupe_media_replaced_event_id" bigint NOT NULL DEFAULT 0;
