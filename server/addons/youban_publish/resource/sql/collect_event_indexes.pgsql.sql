-- Execute manually through DBX in production.
-- CONCURRENTLY avoids blocking collectors; run each statement separately.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_pending_source
  ON hg_youban_publish_collect_event (tenant_id, source_id, received_at, id)
  WHERE status = 'pending' AND processed_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_pending_due
  ON hg_youban_publish_collect_event (status, received_at, id)
  WHERE status = 'pending' AND processed_at IS NULL;
