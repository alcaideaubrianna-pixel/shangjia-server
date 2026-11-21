-- Run each statement in autocommit before deploying the new lookup.
-- Verify the definitions and pg_index.indisvalid/indisready before rollout.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_channel_cycle_due
    ON public.hg_youban_publish_tg_job (channel_id, next_cycle_at, id)
    WHERE cycle_enabled = 1 AND status IN ('sent', 'superseded') AND next_cycle_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_channel_profile_active_last_job
    ON public.hg_youban_publish_channel_profile (last_job_id, channel_id)
    WHERE status = 'active';
