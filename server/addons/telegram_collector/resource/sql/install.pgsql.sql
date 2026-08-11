CREATE TABLE IF NOT EXISTS "hg_tg_collector_source" (
    "id" BIGSERIAL PRIMARY KEY,
    "tenant_id" BIGINT NOT NULL DEFAULT 0,
    "account_id" BIGINT NOT NULL DEFAULT 0,
    "bot_id" BIGINT NOT NULL DEFAULT 0,
    "source_type" VARCHAR(32) NOT NULL DEFAULT 'account',
    "chat_id" VARCHAR(64) NOT NULL DEFAULT '',
    "chat_title" VARCHAR(255) NOT NULL DEFAULT '',
    "chat_username" VARCHAR(255) NOT NULL DEFAULT '',
    "status" VARCHAR(32) NOT NULL DEFAULT 'enabled',
    "history_enabled" SMALLINT NOT NULL DEFAULT 0,
    "history_cursor" TEXT NOT NULL DEFAULT '',
    "created_at" TIMESTAMP NULL,
    "updated_at" TIMESTAMP NULL,
    CONSTRAINT "uq_tg_collector_source_identity" UNIQUE ("tenant_id", "source_type", "chat_id", "account_id", "bot_id")
);

CREATE INDEX IF NOT EXISTS "idx_tg_collector_source_status" ON "hg_tg_collector_source" ("tenant_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_tg_collector_event" (
    "id" BIGSERIAL PRIMARY KEY,
    "tenant_id" BIGINT NOT NULL DEFAULT 0,
    "source_id" BIGINT NOT NULL DEFAULT 0,
    "source_type" VARCHAR(32) NOT NULL DEFAULT '',
    "bot_key" VARCHAR(255) NOT NULL DEFAULT '',
    "account_id" BIGINT NOT NULL DEFAULT 0,
    "chat_id" VARCHAR(64) NOT NULL DEFAULT '',
    "message_id" BIGINT NOT NULL DEFAULT 0,
    "update_id" BIGINT NOT NULL DEFAULT 0,
    "event_key" VARCHAR(255) NOT NULL,
    "raw_update" JSONB NOT NULL,
    "priority" INT NOT NULL DEFAULT 0,
    "status" VARCHAR(32) NOT NULL DEFAULT 'received',
    "attempt_count" INT NOT NULL DEFAULT 0,
    "next_run_at" TIMESTAMP NULL,
    "lease_owner" VARCHAR(128) NOT NULL DEFAULT '',
    "lease_until" TIMESTAMP NULL,
    "received_at" TIMESTAMP NULL,
    "processed_at" TIMESTAMP NULL,
    "error_message" TEXT NOT NULL DEFAULT '',
    "created_at" TIMESTAMP NULL,
    "updated_at" TIMESTAMP NULL,
    CONSTRAINT "uq_tg_collector_event_key" UNIQUE ("tenant_id", "event_key")
);

CREATE INDEX IF NOT EXISTS "idx_tg_collector_event_task" ON "hg_tg_collector_event" ("status", "priority", "next_run_at", "lease_until", "id");
CREATE INDEX IF NOT EXISTS "idx_tg_collector_event_source_chat" ON "hg_tg_collector_event" ("source_id", "chat_id", "status", "next_run_at", "id");

CREATE TABLE IF NOT EXISTS "hg_tg_collector_media" (
    "id" BIGSERIAL PRIMARY KEY,
    "tenant_id" BIGINT NOT NULL DEFAULT 0,
    "fingerprint" VARCHAR(128) NOT NULL,
    "kind" VARCHAR(32) NOT NULL DEFAULT '',
    "mime_type" VARCHAR(128) NOT NULL DEFAULT '',
    "size" BIGINT NOT NULL DEFAULT 0,
    "pipeline_version" VARCHAR(64) NOT NULL DEFAULT 'v1',
    "status" VARCHAR(32) NOT NULL DEFAULT 'processing',
    "storage_path" TEXT NOT NULL DEFAULT '',
    "poster_storage_path" TEXT NOT NULL DEFAULT '',
    "phash" VARCHAR(128) NOT NULL DEFAULT '',
    "dhash" VARCHAR(128) NOT NULL DEFAULT '',
    "attempt_count" INT NOT NULL DEFAULT 0,
    "next_run_at" TIMESTAMP NULL,
    "lease_owner" VARCHAR(128) NOT NULL DEFAULT '',
    "lease_until" TIMESTAMP NULL,
    "error_message" TEXT NOT NULL DEFAULT '',
    "created_at" TIMESTAMP NULL,
    "updated_at" TIMESTAMP NULL,
    CONSTRAINT "uq_tg_collector_media_fingerprint" UNIQUE ("tenant_id", "fingerprint", "pipeline_version")
);

CREATE INDEX IF NOT EXISTS "idx_tg_collector_media_task" ON "hg_tg_collector_media" ("status", "next_run_at", "lease_until", "id");

CREATE TABLE IF NOT EXISTS "hg_tg_collector_delivery" (
    "id" BIGSERIAL PRIMARY KEY,
    "tenant_id" BIGINT NOT NULL DEFAULT 0,
    "event_id" BIGINT NOT NULL DEFAULT 0,
    "delivery_key" VARCHAR(255) NOT NULL,
    "status" VARCHAR(32) NOT NULL DEFAULT 'pending',
    "priority" INT NOT NULL DEFAULT 0,
    "payload" JSONB NOT NULL,
    "attempt_count" INT NOT NULL DEFAULT 0,
    "next_run_at" TIMESTAMP NULL,
    "lease_owner" VARCHAR(128) NOT NULL DEFAULT '',
    "lease_until" TIMESTAMP NULL,
    "error_message" TEXT NOT NULL DEFAULT '',
    "created_at" TIMESTAMP NULL,
    "updated_at" TIMESTAMP NULL,
    CONSTRAINT "uq_tg_collector_delivery_key" UNIQUE ("tenant_id", "delivery_key")
);

CREATE INDEX IF NOT EXISTS "idx_tg_collector_delivery_task" ON "hg_tg_collector_delivery" ("status", "priority", "next_run_at", "lease_until", "id");
