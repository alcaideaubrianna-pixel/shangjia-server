\set ON_ERROR_STOP on
WITH bot_bad AS (
    SELECT e.id AS bot_event_id,e.tenant_id,e.account_id,e.source_grouped_id,d.profile_id
    FROM hg_youban_publish_collect_event e
    JOIN hg_youban_publish_collect_dispatch d ON d.event_id=e.id AND d.profile_id>0
    WHERE e.source_id=54 AND e.source_type='bot' AND e.media_count=1
      AND e.source_grouped_id<>'' AND e.created_at<TIMESTAMP '2026-08-12 19:04:37'
), complete_source AS (
    SELECT b.*,src.account_event_id,src.source_media_count,
           ROW_NUMBER() OVER (PARTITION BY b.profile_id ORDER BY src.source_media_count DESC,src.account_event_id) candidate_rank
    FROM bot_bad b
    JOIN LATERAL (
        SELECT e.id account_event_id,e.media_count source_media_count
        FROM hg_youban_publish_collect_event e
        WHERE e.source_type='account' AND e.source_grouped_id=b.source_grouped_id AND e.media_count>1
          AND (SELECT COUNT(*) FROM hg_youban_publish_collect_event_media em WHERE em.event_id=e.id AND em.cache_status='ready' AND em.storage_path<>'')=e.media_count
        ORDER BY e.media_count DESC,e.id LIMIT 1
    ) src ON TRUE
), current_media AS (
    SELECT profile_id,COUNT(*) media_count FROM hg_youban_publish_media WHERE deleted_at IS NULL GROUP BY profile_id
)
SELECT s.profile_id,p.profile_no,s.bot_event_id,s.account_event_id,
       COALESCE(cm.media_count,0) current_media,s.source_media_count,
       SUM(CASE WHEN em.media_type IN ('photo','image') THEN 1 ELSE 0 END) source_images,
       SUM(CASE WHEN em.media_type='video' THEN 1 ELSE 0 END) source_videos,
       s.source_grouped_id
FROM complete_source s
JOIN hg_content_profile p ON p.id=s.profile_id AND p.deleted_at IS NULL
LEFT JOIN current_media cm ON cm.profile_id=s.profile_id
JOIN hg_youban_publish_collect_event_media em ON em.event_id=s.account_event_id
WHERE s.candidate_rank=1 AND COALESCE(cm.media_count,0)=1 AND s.source_media_count>1
GROUP BY s.profile_id,p.profile_no,s.bot_event_id,s.account_event_id,cm.media_count,s.source_media_count,s.source_grouped_id
ORDER BY s.profile_id;
