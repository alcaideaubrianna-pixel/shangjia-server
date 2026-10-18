CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_process_outbox" (
  "id" BIGSERIAL PRIMARY KEY,
  "event_id" bigint NOT NULL,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "status" varchar(16) NOT NULL DEFAULT 'pending',
  "available_at" timestamp NOT NULL,
  "lease_owner" varchar(128) NOT NULL DEFAULT '',
  "lease_expires_at" timestamp NULL,
  "attempt_count" integer NOT NULL DEFAULT 0,
  "last_error" text,
  "delivered_at" timestamp NULL,
  "created_at" timestamp NULL,
  "updated_at" timestamp NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_process_outbox_event" ON "hg_youban_publish_collect_process_outbox" ("event_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_process_outbox_due" ON "hg_youban_publish_collect_process_outbox" ("available_at", "id") WHERE "status" = 'pending';
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_process_outbox_lease" ON "hg_youban_publish_collect_process_outbox" ("lease_expires_at", "id") WHERE "status" = 'processing';

CREATE TABLE IF NOT EXISTS "hg_youban_publish_ai_ops_repair_plan" (
  "id" BIGSERIAL PRIMARY KEY,
  "plan_id" varchar(64) NOT NULL,
  "action" varchar(64) NOT NULL,
  "payload_json" text NOT NULL,
  "status" varchar(16) NOT NULL DEFAULT 'pending',
  "candidate_count" integer NOT NULL DEFAULT 0,
  "applied_count" integer NOT NULL DEFAULT 0,
  "failed_ids_json" text,
  "confirm_token_hash" varchar(128) NOT NULL DEFAULT '',
  "error_message" text,
  "expires_at" timestamp NOT NULL,
  "started_at" timestamp NULL,
  "finished_at" timestamp NULL,
  "created_at" timestamp NULL,
  "updated_at" timestamp NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_ai_ops_repair_plan" ON "hg_youban_publish_ai_ops_repair_plan" ("plan_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_ai_ops_repair_plan_status" ON "hg_youban_publish_ai_ops_repair_plan" ("status", "expires_at", "id");
