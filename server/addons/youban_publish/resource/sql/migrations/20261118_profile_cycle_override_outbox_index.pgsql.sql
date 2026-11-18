-- Explicit deployment step after 20261118_profile_cycle_override.pgsql.sql.
-- Run standalone with autocommit: CREATE INDEX CONCURRENTLY cannot run inside
-- BEGIN/COMMIT or a migration runner's transaction. Never execute from startup.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_cycle_override_due
  ON hg_youban_publish_tg_outbox (available_at, id)
  WHERE event_type = 'profile.cycle_override.rebuild' AND status = 'pending';
