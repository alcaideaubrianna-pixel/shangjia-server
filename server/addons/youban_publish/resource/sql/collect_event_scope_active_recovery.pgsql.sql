-- Execute manually outside a transaction; never from application startup.
-- Inspect free space, existing index definitions/validity and concurrent DDL first.
-- Source 23:469:95 previously read 3956 blocks and filtered 4564 rows for LIMIT 2.
-- Keep the predicate aligned with recoverCollectEvents; do not add processed_at.
SET lock_timeout = '3s';
SET statement_timeout = '15min';
SET maintenance_work_mem = '128MB';
SET max_parallel_maintenance_workers = 0;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_scope_active_recovery
ON public.hg_youban_publish_collect_event
    (tenant_id, account_id, source_id, updated_at, id)
WHERE status IN (
    'pending', 'group_collecting', 'waiting_order',
    'prechecked', 'media_pending', 'media_ready'
);

-- Verify pg_index.indisvalid/indisready and repeat scoped EXPLAIN before ANALYZE.
-- Rollback, only when explicitly needed:
-- DROP INDEX CONCURRENTLY public.idx_ybp_collect_event_scope_active_recovery;
