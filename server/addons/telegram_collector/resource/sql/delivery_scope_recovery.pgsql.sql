-- Apply outside a transaction; do not run from startup hooks.
-- These indexes replace global source ranking with an explicit source lane.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tg_delivery_scope_pool_recovery
ON hg_tg_collector_delivery
    (tenant_id, account_id, source_id, scheduler_pool, priority DESC, created_at, id)
WHERE status IN ('pending', 'failed_retry');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tg_delivery_scope_lease_recovery
ON hg_tg_collector_delivery (tenant_id, account_id, source_id, lease_until, id)
WHERE status='processing' AND lease_until IS NOT NULL;
