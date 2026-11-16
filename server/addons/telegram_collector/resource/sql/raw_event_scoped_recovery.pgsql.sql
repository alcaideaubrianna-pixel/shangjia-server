-- Execute outside a transaction. No production execution performed.
-- Raw Bot scopes may have account_id=0 and source_id=0.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tgc_event_scope_received_recovery
ON hg_tg_collector_event (tenant_id,account_id,source_id,status,updated_at,id) WHERE status='received';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tgc_event_scope_retry_recovery
ON hg_tg_collector_event (tenant_id,account_id,source_id,status,next_run_at,id) WHERE status='failed_retry';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tgc_event_scope_lease_recovery
ON hg_tg_collector_event (tenant_id,account_id,source_id,status,lease_until,id) WHERE status='processing' AND lease_until IS NOT NULL;
