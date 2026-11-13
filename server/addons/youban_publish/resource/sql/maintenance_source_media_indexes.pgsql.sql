-- Explicit maintenance only: psql -X -v ON_ERROR_STOP=1 -f this-file.sql
-- Do not execute through the plugin upgrade transaction or with psql -1.
-- Concurrent builds permit writes but consume I/O; run sequentially.
-- Check pg_index.indisvalid/indisready after completion. IF NOT EXISTS does
-- not repair an existing INVALID index; inspect that condition before rerunning.
\set ON_ERROR_STOP on
SET lock_timeout = '5s';
SET statement_timeout = '15min';
SET maintenance_work_mem = '128MB';
SET max_parallel_maintenance_workers = 1;

-- MAX(received_at) becomes a bounded index lookup; source counts can use the
-- same leading key. Existing source/status and dispatch indexes stay intact.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_source_received
    ON public.hg_youban_publish_collect_event (source_id, received_at DESC);

-- Exact attachment cache lookups currently scan the entire attachment table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sys_attachment_path
    ON public.hg_sys_attachment (path);

-- 账户分片调度只读取仍待处理或租约已过期的账户，避免按 Outbox 全表聚合。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_outbox_scope_due
    ON public.hg_youban_publish_collect_process_outbox
    (tenant_id, account_id, status, available_at, lease_expires_at, id);

-- Rollback only if required, separately and outside a transaction:
-- DROP INDEX CONCURRENTLY public.idx_ybp_collect_event_source_received;
-- DROP INDEX CONCURRENTLY public.idx_sys_attachment_path;
-- DROP INDEX CONCURRENTLY public.idx_ybp_collect_outbox_scope_due;
