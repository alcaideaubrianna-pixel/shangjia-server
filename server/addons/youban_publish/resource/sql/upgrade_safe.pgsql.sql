ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "next_retry_at" timestamp DEFAULT NULL;

CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle_due"
  ON "hg_youban_publish_tg_job" ("next_cycle_at", "id")
  INCLUDE ("tenant_id", "account_id", "profile_id", "channel_id", "cycle_days")
  WHERE "cycle_enabled" = 1 AND "status" IN ('sent','superseded') AND "next_cycle_at" IS NOT NULL;
