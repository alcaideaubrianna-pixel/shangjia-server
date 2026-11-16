-- Immutable cleanup context. Apply before enabling durable cleanup dispatch.
ALTER TABLE "hg_youban_publish_tg_outbox" ADD COLUMN IF NOT EXISTS "operation_no" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_outbox" ADD COLUMN IF NOT EXISTS "payload" text NOT NULL DEFAULT '';
-- Legacy rows stay empty; never backfill from current profile state.
