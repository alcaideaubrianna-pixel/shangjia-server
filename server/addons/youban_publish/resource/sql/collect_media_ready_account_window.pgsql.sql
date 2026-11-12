-- 采集 media_ready 验证窗口账户级索引；必须低峰单独执行 CONCURRENTLY。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_account_window
ON hg_youban_publish_collect_event
(tenant_id, account_id, source_id, source_chat_id, status, updated_at, id)
WHERE processed_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_media_account_ready_window
ON hg_youban_publish_collect_event_media
(tenant_id, account_id, source_id, source_chat_id, cache_status, updated_at, id)
WHERE cache_status='ready';
