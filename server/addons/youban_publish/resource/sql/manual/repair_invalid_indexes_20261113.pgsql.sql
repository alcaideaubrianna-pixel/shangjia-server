-- 线上无效索引修复脚本（必须使用 PostgreSQL 自动提交模式逐条执行）。
--
-- 背景：历史升级脚本使用普通 CREATE INDEX，超时取消后留下 indisvalid=false
-- 的索引。CREATE INDEX IF NOT EXISTS 不会修复这种索引，因此必须先并发删除，
-- 再并发重建。每条语句单独执行，不能包在事务内，也不要并发执行多条。
-- 建议在数据库负载较低时执行；执行期间通过 pg_stat_progress_create_index 观察。

SET lock_timeout = '2s';
SET statement_timeout = '30min';

DROP INDEX CONCURRENTLY IF EXISTS idx_ybp_tg_job_account_channel_order;
CREATE INDEX CONCURRENTLY idx_ybp_tg_job_account_channel_order
    ON hg_youban_publish_tg_job (tenant_id, account_id, channel_id, created_at, id)
    INCLUDE (status, dispatch_status, next_retry_at, priority);

DROP INDEX CONCURRENTLY IF EXISTS idx_ybp_tg_outbox_event_status_due;
CREATE INDEX CONCURRENTLY idx_ybp_tg_outbox_event_status_due
    ON hg_youban_publish_tg_outbox (event_type, status, available_at, id)
    INCLUDE (job_id, attempt_count);

DROP INDEX CONCURRENTLY IF EXISTS idx_ybp_tg_outbox_event_status_lease;
CREATE INDEX CONCURRENTLY idx_ybp_tg_outbox_event_status_lease
    ON hg_youban_publish_tg_outbox (event_type, status, lease_expires_at, id)
    INCLUDE (job_id, attempt_count);

DROP INDEX CONCURRENTLY IF EXISTS idx_ybp_profile_state_missing_operation_ccnew;
CREATE INDEX CONCURRENTLY idx_ybp_profile_state_missing_operation_ccnew
    ON hg_youban_publish_profile_state (tenant_id, account_id, profile_id)
    WHERE deleted_at IS NULL AND publish_task_status = '';

DROP INDEX CONCURRENTLY IF EXISTS idx_ybp_tg_job_missing_operation_state_ccnew;
CREATE INDEX CONCURRENTLY idx_ybp_tg_job_missing_operation_state_ccnew
    ON hg_youban_publish_tg_job (tenant_id, account_id, id)
    WHERE task_id IS NULL
      AND operation_no <> ''
      AND status IN ('pending', 'sending', 'failed_retry', 'unknown');

DROP INDEX CONCURRENTLY IF EXISTS idx_ybp_profile_fingerprint_indexed_profile;
CREATE INDEX CONCURRENTLY idx_ybp_profile_fingerprint_indexed_profile
    ON hg_youban_publish_profile_fingerprint (profile_id)
    WHERE layer = '_indexed';

-- 采集 media_ready 按账户读取索引。旧脚本误用了不存在的 status 列，
-- 这里使用事件关联键，确保账户分片查询先缩小媒体范围。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_media_account_ready_v2
    ON hg_youban_publish_collect_event_media (tenant_id, account_id, event_id, updated_at, id)
    WHERE cache_status = 'ready';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_media_ready_account_v2
    ON hg_youban_publish_collect_event (tenant_id, account_id, source_id, updated_at, id)
    WHERE status = 'media_ready' AND processed_at IS NULL;
