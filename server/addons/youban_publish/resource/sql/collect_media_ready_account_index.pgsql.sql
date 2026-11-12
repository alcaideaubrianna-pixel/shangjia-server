-- 采集验证窗口按账户、采集源和消息游标读取，避免 media_ready 事件扫描全局历史。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_media_window_account
ON hg_youban_publish_collect_event (tenant_id, account_id, source_id, source_chat_id, source_message_id, id)
WHERE status IN ('pending','group_collect','waiting_order','prechecked','media_pending','media_ready','ignored','failed');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_media_ready_account
ON hg_youban_publish_collect_event (tenant_id, account_id, source_id, updated_at, id)
WHERE status='media_ready' AND processed_at IS NULL;
