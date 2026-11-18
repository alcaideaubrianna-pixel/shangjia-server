-- Existing cycle_run can be large. Run in standalone psql autocommit, not -1.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_ybp_cycle_snapshot_day
    ON hg_youban_publish_cycle_run(channel_id, scheduled_day)
    WHERE upper_profile_id = -1 AND scheduled_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_cycle_run_scheduled_day
    ON hg_youban_publish_cycle_run (tenant_id, channel_id, scheduled_at)
    WHERE scheduled_at IS NOT NULL;
