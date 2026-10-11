CREATE TABLE IF NOT EXISTS "hg_youban_publish_relay_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" varchar(128) NOT NULL DEFAULT '',
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "status" varchar(16) NOT NULL DEFAULT 'disabled',
  "health_status" varchar(16) NOT NULL DEFAULT 'unknown',
  "bot_permission_status" varchar(16) NOT NULL DEFAULT 'unknown',
  "cooldown_until" timestamp DEFAULT NULL,
  "last_error" text,
  "in_flight_count" integer NOT NULL DEFAULT 0,
  "last_used_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_relay_channel_tenant_chat" ON "hg_youban_publish_relay_channel" ("tenant_id", "target_chat_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_relay_channel_pick" ON "hg_youban_publish_relay_channel" ("tenant_id", "status", "health_status", "cooldown_until", "last_used_at") WHERE "deleted_at" IS NULL;
CREATE TABLE IF NOT EXISTS "hg_youban_publish_relay_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "purpose" varchar(16) NOT NULL DEFAULT '',
  "media_version" varchar(64) NOT NULL DEFAULT 'original',
  "relay_channel_id" bigint NOT NULL DEFAULT 0,
  "relay_chat_id" varchar(128) NOT NULL DEFAULT '',
  "message_id" bigint NOT NULL DEFAULT 0,
  "file_id" text NOT NULL DEFAULT '',
  "status" varchar(16) NOT NULL DEFAULT 'uploading',
  "lease_token" varchar(64) NOT NULL DEFAULT '',
  "lease_until" timestamp DEFAULT NULL,
  "last_error" text,
  "expires_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_relay_media_key" ON "hg_youban_publish_relay_media" ("tenant_id", "media_id", "purpose", "media_version");
CREATE INDEX IF NOT EXISTS "idx_ybp_relay_media_ready" ON "hg_youban_publish_relay_media" ("tenant_id", "media_id", "purpose", "status");
CREATE INDEX IF NOT EXISTS "idx_ybp_relay_media_lease" ON "hg_youban_publish_relay_media" ("status", "lease_until", "id");
