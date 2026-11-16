-- Execute before deploying code that writes recovery_of_job_id.
-- Existing and non-recovery jobs retain NULL; no historical backfill is performed.
ALTER TABLE "hg_youban_publish_tg_job"
    ADD COLUMN IF NOT EXISTS "recovery_of_job_id" bigint DEFAULT NULL;

-- Run outside a transaction, after the column has been added.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tg_job_recovery_of_status
    ON "hg_youban_publish_tg_job" (recovery_of_job_id, status)
    WHERE recovery_of_job_id IS NOT NULL;
