-- Run separately, outside a transaction. Do not execute from startup hooks.
-- Confirm existing indexes and free disk space before applying in production.
-- Source 121 EXPLAIN ANALYZE: downloading LIMIT 12 used a global cache index,
-- read 823 blocks and took 3013ms despite returning no rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_media_scope_downloading_due
ON hg_youban_publish_collect_event_media
    (tenant_id, account_id, source_id, updated_at, id)
INCLUDE (event_id)
WHERE cache_status = 'downloading';

-- Bot recovery currently scans all events in a source and sorts before LIMIT.
-- Source 121 execution exceeded a 5s statement timeout. This partial index
-- limits the candidate scan to unfinished recoverable events in that source.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_scope_media_recovery
ON hg_youban_publish_collect_event
    (tenant_id, account_id, source_id, updated_at, id)
WHERE processed_at IS NULL
  AND status IN ('media_pending', 'media_ready', 'group_collecting', 'pending', 'failed');

-- After creation, repeat scoped EXPLAIN (ANALYZE, BUFFERS) with statement_timeout.
-- No performance improvement is claimed until the new plans are verified.
