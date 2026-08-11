-- 只读历史故障检查。生产执行时建议设置 statement_timeout = '3s'。

-- 同频道活动 Job 数量大于 1。
SELECT channel_id, COUNT(*) AS active_jobs
FROM hg_youban_publish_tg_job
WHERE status = 'sending' OR dispatch_status = 'processing'
GROUP BY channel_id
HAVING COUNT(*) > 1
ORDER BY active_jobs DESC;

-- 有 idle Job，但频道没有活动队头。
SELECT channel_id, COUNT(*) AS idle_jobs, MIN(updated_at) AS oldest_idle_at
FROM hg_youban_publish_tg_job j
WHERE status IN ('pending','failed_retry','unknown')
  AND COALESCE(dispatch_status, '') IN ('', 'idle')
  AND updated_at < NOW() - INTERVAL '2 minutes'
  AND NOT EXISTS (
    SELECT 1 FROM hg_youban_publish_tg_job active
    WHERE active.channel_id = j.channel_id
      AND (active.status = 'sending' OR active.dispatch_status IN ('queued','processing'))
  )
GROUP BY channel_id
ORDER BY idle_jobs DESC;

-- sending 超过任务超时时间。
SELECT id, tenant_id, channel_id, account_id, operation_no, updated_at, error_message
FROM hg_youban_publish_tg_job
WHERE status = 'sending' AND updated_at < NOW() - INTERVAL '12 minutes'
ORDER BY updated_at;

-- Bot 采集事件 bot_id=0。
SELECT id, tenant_id, account_id, source_id, source_chat_id, source_message_id, created_at
FROM hg_youban_publish_collect_event
WHERE source_type = 'bot' AND bot_id = 0
  AND created_at >= NOW() - INTERVAL '1 hour'
ORDER BY created_at DESC;

-- 已配置但长期无事件的采集源。
SELECT id, tenant_id, account_id, source_type, title, bot_id, tg_account_id, last_event_at
FROM hg_youban_publish_collect_source
WHERE collect_enabled = 1 AND status = 1 AND deleted_at IS NULL
  AND COALESCE(last_event_at, created_at) < NOW() - INTERVAL '1 hour'
ORDER BY COALESCE(last_event_at, created_at);

-- FILE_REFERENCE_EXPIRED 和视频预览图缺失。
SELECT id, event_id, media_type, cache_status, poster_url, error_message, updated_at
FROM hg_youban_publish_collect_event_media
WHERE updated_at >= NOW() - INTERVAL '1 hour'
  AND (error_message ILIKE '%FILE_REFERENCE_EXPIRED%'
       OR (media_type = 'video' AND cache_status = 'ready' AND COALESCE(poster_url, '') = ''))
ORDER BY updated_at DESC;

-- 单频道积压、最近发送速率和预计清空时间。
WITH pending AS (
  SELECT channel_id, COUNT(*)::bigint AS pending_count, MIN(created_at) AS oldest_at
  FROM hg_youban_publish_tg_job
  WHERE status IN ('pending','failed_retry','unknown')
  GROUP BY channel_id
), sent AS (
  SELECT channel_id, COUNT(*)::numeric / 10.0 AS sent_per_minute
  FROM hg_youban_publish_tg_job
  WHERE status = 'sent' AND sent_at >= NOW() - INTERVAL '10 minutes'
  GROUP BY channel_id
)
SELECT pending.channel_id, pending.pending_count, pending.oldest_at,
       COALESCE(sent.sent_per_minute, 0) AS sent_per_minute,
       COALESCE(CEIL(pending.pending_count / NULLIF(sent.sent_per_minute, 0)), -1) AS estimated_clear_minutes
FROM pending LEFT JOIN sent USING (channel_id)
ORDER BY pending.pending_count DESC
LIMIT 20;
