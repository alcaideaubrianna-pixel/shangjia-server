-- Explicit pre-deployment migration. Never run DDL from API/worker startup.
ALTER TABLE hg_youban_publish_tg_job
    ADD COLUMN IF NOT EXISTS send_deadline_at timestamp NULL,
    ADD COLUMN IF NOT EXISTS timeout_parent_job_id bigint NULL,
    ADD COLUMN IF NOT EXISTS timeout_cleanup_at timestamp NULL;
-- Execute statements individually, outside a transaction. Build replacements
-- before removing old indexes, so normal operation uniqueness is never lost.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uk_ybp_tg_job_operation_generation
    ON hg_youban_publish_tg_job(task_id,operation_no,channel_id,COALESCE(timeout_parent_job_id,0))
    WHERE operation_no<>'';
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uk_ybp_tg_job_profile_operation_generation
    ON hg_youban_publish_tg_job(profile_id,operation_no,channel_id,COALESCE(timeout_parent_job_id,0))
    WHERE task_id IS NULL AND profile_id>0 AND operation_no<>'';
DROP INDEX CONCURRENTLY IF EXISTS uk_ybp_tg_job_operation_channel;
DROP INDEX CONCURRENTLY IF EXISTS uk_ybp_tg_job_profile_operation_channel;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_tg_job_timeout_parent
    ON hg_youban_publish_tg_job(timeout_parent_job_id)
    WHERE timeout_parent_job_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tg_job_send_deadline
    ON hg_youban_publish_tg_job(send_deadline_at,id)
    WHERE status IN ('pending','sending','failed_retry','unknown') AND send_deadline_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tg_job_timeout_cleanup
    ON hg_youban_publish_tg_job(timeout_cleanup_at,id)
    WHERE send_phase='timeout_cancelled' AND timeout_cleanup_at IS NOT NULL;
