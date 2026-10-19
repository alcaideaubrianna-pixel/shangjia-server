-- 在线执行，不能放入事务：psql -v ON_ERROR_STOP=1 -f 本文件
-- 媒体组回执按频道、媒体组及最新记录查找，避免扫描整个频道历史。
SET lock_timeout = '30s';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_bot_message_source_group_match
ON hg_youban_publish_bot_message_source (chat_id, media_group_id, id DESC)
INCLUDE (matched_attempt_id)
WHERE matched_attempt_id > 0;
