-- 频道队首查询索引。部署时使用 CONCURRENTLY，禁止放入事务。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_channel_head_active
ON hg_youban_publish_tg_job
    (tenant_id, channel_id, target_chat_id, status, dispatch_status, next_retry_at, created_at, id)
INCLUDE (priority, send_phase, operation_no)
WHERE status IN ('pending','failed_retry','confirming','sending')
   OR (status='failed' AND send_phase='display_confirmed');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_channel_head_queued
ON hg_youban_publish_tg_job
    (tenant_id, channel_id, target_chat_id, dispatch_status, created_at, id)
INCLUDE (status, priority, send_phase, operation_no)
WHERE dispatch_status IN ('queued','processing');
