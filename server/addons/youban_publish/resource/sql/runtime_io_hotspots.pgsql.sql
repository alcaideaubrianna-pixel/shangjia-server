-- Explicit maintenance only: run with psql, outside a transaction, before deploying
-- the COALESCE window query. Build one index at a time; do not run at app startup.
-- IF NOT EXISTS does not repair an INVALID index left by an interrupted build.
-- Deployment order: build indexes individually; verify pg_index.indisvalid and
-- indisready, then deploy collector/publish/background query changes. Monitor
-- build I/O and disk headroom; these indexes add storage and write overhead.
-- Roll back application queries first if necessary; do not drop old indexes
-- as part of this migration. The time-range index narrows the candidate set;
-- ORDER BY source_message_id may still require sorting within that range.
SET lock_timeout = '3s';
SET statement_timeout = '20min';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_permission_retention_v2
ON public.hg_youban_publish_tg_job (updated_at, id)
WHERE status='superseded'
  AND last_dispatch_error='频道权限暂停，历史未发送任务已终止；如需补投请主动重新上架';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_scoped_parent_v2
ON public.hg_youban_publish_collect_event
(tenant_id, account_id, source_id, material_parent_event_id, material_role, source_chat_id, id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_scoped_window_v2
ON public.hg_youban_publish_collect_event
(tenant_id, source_id, account_id, (COALESCE(received_at,created_at)), source_message_id, id);

RESET statement_timeout;
RESET lock_timeout;
