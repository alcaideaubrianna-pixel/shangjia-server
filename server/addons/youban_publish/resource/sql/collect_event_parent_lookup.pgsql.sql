-- Execute explicitly outside a transaction, never from application startup.
-- Check free disk, concurrent DDL, existing definition and validity first.
-- A cancelled concurrent build can leave an INVALID index: do not rely on
-- IF NOT EXISTS to repair it. Inspect/drop only that invalid artifact first.
SET lock_timeout = '3s';
SET statement_timeout = '15min';
SET maintenance_work_mem = '128MB';
SET max_parallel_maintenance_workers = 0;

-- Leading parent ID supports single-parent, parent IN, parent+role and
-- primary-key OR parent lookup. No status predicate: terminal and active
-- children are both used by replay, completion and review queries.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_parent_role_id
ON public.hg_youban_publish_collect_event
    (material_parent_event_id, material_role, id);

-- Verify indisvalid/indisready before scoped EXPLAIN (ANALYZE, BUFFERS).
-- Rollback, only when needed:
-- DROP INDEX CONCURRENTLY public.idx_ybp_collect_event_parent_role_id;
