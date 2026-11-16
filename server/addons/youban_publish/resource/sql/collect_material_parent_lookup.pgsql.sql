-- Run outside a transaction. Apply explicitly; this file performs no runtime DDL.
-- Parent-row locking serializes group claims. This partial index supports the
-- bounded conflicting-child probe without scanning an entire source history.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_verify_parent_scope
ON hg_youban_publish_collect_event
(tenant_id, account_id, source_id, source_chat_id, material_parent_event_id, id)
INCLUDE (source_grouped_id)
WHERE material_role = 'verify';
