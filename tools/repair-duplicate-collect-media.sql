-- 相册媒体重复缓存修复脚本
-- 默认只生成预览。通过 psql 设置 APPLY_REPAIR=1 后才会执行 UPDATE。
-- 不删除 COS 对象和原始采集事件，只重置异常媒体缓存。

BEGIN;

CREATE TEMP TABLE duplicate_collect_media_events ON COMMIT DROP AS
SELECT event_id, source_grouped_id
FROM hg_youban_publish_collect_event_media
WHERE media_type = 'photo'
  AND source_grouped_id <> ''
  AND source_file_id <> ''
  AND file_md5 <> ''
GROUP BY event_id, source_grouped_id
HAVING COUNT(*) >= 2
   AND COUNT(DISTINCT source_file_id) > 1
   AND COUNT(DISTINCT file_md5) = 1;

SELECT COUNT(*) AS duplicate_groups
FROM duplicate_collect_media_events;

SELECT COUNT(*) AS duplicate_media_rows
FROM hg_youban_publish_collect_event_media media
JOIN duplicate_collect_media_events duplicate_group
  ON duplicate_group.event_id = media.event_id
 AND duplicate_group.source_grouped_id = media.source_grouped_id;

SELECT event_id, source_grouped_id, COUNT(*) AS media_rows,
       COUNT(DISTINCT source_file_id) AS source_files,
       COUNT(DISTINCT file_md5) AS md5_values
FROM hg_youban_publish_collect_event_media media
JOIN duplicate_collect_media_events duplicate_group
  ON duplicate_group.event_id = media.event_id
 AND duplicate_group.source_grouped_id = media.source_grouped_id
GROUP BY event_id, source_grouped_id
ORDER BY event_id DESC
LIMIT 100;

-- 执行方式：APPLY_REPAIR=1 psql ... -f tools/repair-duplicate-collect-media.sql
-- 仅在显式 apply 时重置异常媒体。
DO $$
BEGIN
    IF current_setting('app.apply_repair', true) = '1' THEN
        UPDATE hg_youban_publish_collect_event_media media
        SET file_url = '',
            storage_path = '',
            poster_url = '',
            file_md5 = '',
            file_phash = '',
            meta_json = NULL,
            cache_status = 'pending',
            error_message = '重复相册媒体缓存已隔离，等待按 source_file_id 重新下载',
            download_duration_ms = 0,
            download_bytes = 0,
            download_attempts = 0,
            cache_hit = 0,
            download_error_type = '',
            next_retry_at = NULL,
            updated_at = clock_timestamp()
        FROM duplicate_collect_media_events duplicate_group
        WHERE duplicate_group.event_id = media.event_id
          AND duplicate_group.source_grouped_id = media.source_grouped_id;
    END IF;
END $$;

COMMIT;
