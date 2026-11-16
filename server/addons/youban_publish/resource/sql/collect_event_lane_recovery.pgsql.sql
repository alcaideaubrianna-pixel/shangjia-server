-- Separate manual migration, outside a transaction. This is not a startup hook.
-- The generic event recovery predicate has no processed_at condition and also
-- includes waiting_order/prechecked, so it cannot use the media-only partial index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_lane_recovery_due
ON hg_youban_publish_collect_event (tenant_id, account_id, source_id, updated_at, id)
INCLUDE (status)
WHERE status IN ('pending','group_collecting','waiting_order','prechecked','media_pending','media_ready');
