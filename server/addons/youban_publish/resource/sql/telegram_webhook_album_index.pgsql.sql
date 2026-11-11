-- 显式部署迁移：支持先到达的无caption相册成员按频道/相册绑定；禁止角色启动时并发DDL。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_bot_message_source_album
    ON hg_youban_publish_bot_message_source(chat_id,media_group_id,message_id)
    WHERE media_group_id<>'' AND deleted_at IS NULL;
