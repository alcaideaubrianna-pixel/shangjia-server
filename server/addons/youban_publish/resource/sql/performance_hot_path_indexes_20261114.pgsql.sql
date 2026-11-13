-- 采集、循环和 TG 推送热点查询索引。
-- 只允许在生产低锁窗口以外顺序执行；索引并发创建期间不要重复执行同一条语句。
\set ON_ERROR_STOP on
SET lock_timeout = '5s';
SET statement_timeout = '15min';
SET maintenance_work_mem = '128MB';
SET max_parallel_maintenance_workers = 1;

-- 资料详情、媒体完成和循环状态都会按租户、账户、资料读取 TG Job。
-- 将租户和账户放在 profile_id 前面，避免同一个资料号跨租户时扫描历史记录。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_owner_profile_status
    ON public.hg_youban_publish_tg_job
    (tenant_id, account_id, profile_id, status, id)
    INCLUDE (channel_id, operation_no, dispatch_status, send_phase);

-- 循环历史关联按来源频道和消息顺序读取，避免从 profile_id 反查整张 Job 表。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_job_collect_owner_message
    ON public.hg_youban_publish_tg_job
    (tenant_id, account_id, collect_source_id, collect_source_chat_id,
     collect_source_message_id, status, id)
    INCLUDE (profile_id, channel_id, operation_no);

-- 采集分发的资料、事件和状态关联查询。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_dispatch_owner_profile_event
    ON public.hg_youban_publish_collect_dispatch
    (tenant_id, account_id, profile_id, event_id, status, id)
    INCLUDE (source_id);

-- 一个 dispatch 关联多个上架频道时，按租户、账户和 dispatch_id 直接定位关联行。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_dispatch_channel_owner_dispatch
    ON public.hg_youban_publish_collect_dispatch_channel
    (tenant_id, account_id, dispatch_id, channel_id);

-- 采集事件明细读取只取指定来源、状态和游标范围，支持实时优先的账户分片。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_owner_source_status
    ON public.hg_youban_publish_collect_event
    (tenant_id, account_id, source_id, status, id)
    INCLUDE (source_chat_id, source_message_id, received_at, processed_at);

-- 媒体 ready/processing 恢复按账户、来源和状态分页。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_collect_event_media_owner_source_status
    ON public.hg_youban_publish_collect_event_media
    (tenant_id, account_id, source_id, cache_status, id)
    INCLUDE (event_id, updated_at, next_retry_at);

-- 发布状态投影按租户、账户和资料定位，减少 profile_state 的回写前扫描。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_profile_state_owner_profile_status
    ON public.hg_youban_publish_profile_state
    (tenant_id, account_id, profile_id, publish_task_status)
    INCLUDE (publish_operation_no, publish_task_updated_at)
    WHERE deleted_at IS NULL;

-- 验证索引有效性：
-- SELECT indexrelid::regclass, indisvalid, indisready
-- FROM pg_index
-- WHERE indexrelid::regclass::text LIKE 'idx_ybp_%';
