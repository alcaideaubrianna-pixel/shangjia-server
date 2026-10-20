-- 手动执行；CREATE INDEX CONCURRENTLY 不得放入事务或自动启动迁移。
-- 失败后先查 pg_index.indisvalid；同名无效索引需单独 DROP INDEX CONCURRENTLY 再重试。
SET lock_timeout = '3s';
SET statement_timeout = '5min';
SET max_parallel_maintenance_workers = 0;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_media_invalid_ready
ON public.hg_youban_publish_collect_event_media (id) INCLUDE (tenant_id, account_id, source_id)
WHERE cache_status = 'ready'
  AND COALESCE(TRIM(storage_path), '') = ''
  AND COALESCE(TRIM(file_url), '') = ''
  AND NOT (COALESCE(TRIM(backup_chat_id), '') <> '' AND backup_message_id > 0);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_retention
ON public.hg_youban_publish_collect_event (created_at, id)
WHERE status IN ('failed', 'ignored');
RESET lock_timeout;
RESET statement_timeout;
RESET max_parallel_maintenance_workers;
