\set ON_ERROR_STOP on

BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

CREATE TEMP TABLE repair_candidates ON COMMIT DROP AS
WITH bot_bad AS (
    SELECT e.id AS bot_event_id,
           e.tenant_id,
           e.account_id,
           e.source_grouped_id,
           d.profile_id
    FROM hg_youban_publish_collect_event e
    JOIN hg_youban_publish_collect_dispatch d
      ON d.event_id = e.id
     AND d.profile_id > 0
    WHERE e.source_id = 54
      AND e.source_type = 'bot'
      AND e.media_count = 1
      AND e.source_grouped_id <> ''
      AND e.created_at < TIMESTAMP '2026-08-12 19:04:37'
), complete_source AS (
    SELECT b.*,
           src.account_event_id,
           src.source_media_count,
           ROW_NUMBER() OVER (
               PARTITION BY b.profile_id
               ORDER BY src.source_media_count DESC, src.account_event_id ASC
           ) AS candidate_rank
    FROM bot_bad b
    JOIN LATERAL (
        SELECT e.id AS account_event_id,
               e.media_count AS source_media_count
        FROM hg_youban_publish_collect_event e
        WHERE e.source_type = 'account'
          AND e.source_grouped_id = b.source_grouped_id
          AND e.media_count > 1
          AND (
              SELECT COUNT(*)
              FROM hg_youban_publish_collect_event_media em
              WHERE em.event_id = e.id
                AND em.cache_status = 'ready'
                AND em.storage_path <> ''
          ) = e.media_count
        ORDER BY e.media_count DESC, e.id ASC
        LIMIT 1
    ) src ON TRUE
), current_media AS (
    SELECT profile_id, COUNT(*) AS media_count
    FROM hg_youban_publish_media
    WHERE deleted_at IS NULL
    GROUP BY profile_id
)
SELECT s.profile_id,
       s.tenant_id,
       s.account_id,
       s.bot_event_id,
       s.account_event_id,
       s.source_media_count,
       s.source_grouped_id
FROM complete_source s
JOIN hg_content_profile p
  ON p.id = s.profile_id
 AND p.deleted_at IS NULL
LEFT JOIN current_media cm
  ON cm.profile_id = s.profile_id
WHERE s.candidate_rank = 1
  AND COALESCE(cm.media_count, 0) = 1
  AND s.source_media_count > 1;

CREATE TEMP TABLE profile_backup ON COMMIT DROP AS
SELECT p.id,
       p.image_count,
       p.video_count,
       p.has_verification_video,
       p.updated_at
FROM hg_content_profile p
JOIN repair_candidates c ON c.profile_id = p.id;

CREATE TEMP TABLE source_media ON COMMIT DROP AS
WITH source_items AS (
    SELECT c.profile_id,c.tenant_id,c.account_id,c.account_event_id,
           em.id AS event_media_id,
           CASE WHEN em.media_type = 'photo' THEN 'image' ELSE em.media_type END AS media_type,
           'display'::varchar AS purpose,
           em.sort_index,em.file_url,em.storage_path,em.poster_url,
           em.source_size,em.file_md5,em.file_phash
    FROM repair_candidates c
    JOIN hg_youban_publish_collect_event_media em ON em.event_id = c.account_event_id
    WHERE em.cache_status = 'ready' AND em.storage_path <> ''
      AND em.media_type IN ('photo', 'image', 'video')
    UNION ALL
    SELECT c.profile_id,c.tenant_id,c.account_id,c.account_event_id,
           em.id AS event_media_id,
           'video'::varchar AS media_type,
           'verify'::varchar AS purpose,
           em.sort_index,em.file_url,em.storage_path,em.poster_url,
           em.source_size,em.file_md5,em.file_phash
    FROM repair_candidates c
    JOIN hg_youban_publish_collect_event parent ON parent.id = c.account_event_id
    JOIN hg_youban_publish_collect_event verify
      ON verify.material_parent_event_id = parent.id
     AND verify.material_role = 'verify'
    JOIN hg_youban_publish_collect_event_media em ON em.event_id = verify.id
    WHERE em.cache_status = 'ready' AND em.storage_path <> ''
      AND em.media_type IN ('video', 'document')
)
SELECT profile_id,tenant_id,account_id,account_event_id,event_media_id,media_type,purpose,
       ROW_NUMBER() OVER (PARTITION BY profile_id,purpose ORDER BY sort_index,event_media_id)::int AS sort_index,
       file_url,storage_path,poster_url,source_size,file_md5,file_phash
FROM source_items;

CREATE TEMP TABLE inserted_media ON COMMIT DROP AS
WITH inserted AS (
    INSERT INTO hg_youban_publish_media (
        tenant_id, merchant_id, account_id, profile_id, attachment_id,
        media_type, purpose, name, file_url, poster_url, poster_storage_path,
        tg_file_id, tg_thumb_file_id, storage_path, mime_type, md5,
        perceptual_hash, size, sort_index, status,
        created_by, updated_by, deleted_by, created_at, updated_at,
        tg_cache_asset_hash, tg_cache_status,
        original_attachment_id, original_file_url, original_storage_path,
        edited_attachment_id, edited_file_url, edited_storage_path,
        edit_config_json, edit_status, must_send,
        processing_status, processing_error
    )
    SELECT s.tenant_id,
           s.tenant_id,
           s.account_id,
           s.profile_id,
           0,
           s.media_type,
           s.purpose,
           'bot-group-repair-20260812-' || s.account_event_id || '-' || s.event_media_id,
           s.file_url,
           s.poster_url,
           '',
           '',
           '',
           s.storage_path,
           '',
           s.file_md5,
           s.file_phash,
           COALESCE(s.source_size, 0),
           s.sort_index,
           1,
           s.account_id,
           s.account_id,
           0,
           NOW(),
           NOW(),
           COALESCE(NULLIF(s.storage_path, ''), s.file_url),
           'invalid',
           0,
           '',
           '',
           0,
           '',
           '',
           '',
           'raw',
           0,
           'uploaded',
           ''
    FROM source_media s
    WHERE NOT EXISTS (
        SELECT 1
        FROM hg_youban_publish_media m
        WHERE m.profile_id = s.profile_id
          AND m.deleted_at IS NULL
          AND (
              (s.storage_path <> '' AND m.storage_path = s.storage_path)
              OR (s.file_md5 <> '' AND m.md5 = s.file_md5)
              OR (s.file_url <> '' AND m.file_url = s.file_url)
          )
    )
    RETURNING id, profile_id, name
)
SELECT id, profile_id, name FROM inserted;

UPDATE hg_content_profile p
SET image_count = counts.image_count,
    video_count = counts.video_count,
    has_verification_video = counts.has_verification_video,
    updated_at = NOW()
FROM (
    SELECT c.profile_id,
           COUNT(*) FILTER (
               WHERE m.media_type = 'image' AND m.deleted_at IS NULL
           )::int AS image_count,
           COUNT(*) FILTER (
               WHERE m.media_type = 'video' AND m.deleted_at IS NULL
           )::int AS video_count,
           CASE WHEN COUNT(*) FILTER (
               WHERE m.media_type = 'video'
                 AND m.purpose = 'verify'
                 AND m.deleted_at IS NULL
           ) > 0 THEN 1 ELSE 0 END AS has_verification_video
    FROM repair_candidates c
    JOIN hg_youban_publish_media m ON m.profile_id = c.profile_id
    GROUP BY c.profile_id
) counts
WHERE p.id = counts.profile_id;

SELECT COUNT(*) AS repaired_profiles FROM repair_candidates;
SELECT COUNT(*) AS inserted_media FROM inserted_media;
SELECT p.id,
       p.profile_no,
       p.image_count,
       p.video_count,
       p.has_verification_video,
       COUNT(m.id) FILTER (WHERE m.deleted_at IS NULL) AS active_media
FROM hg_content_profile p
JOIN repair_candidates c ON c.profile_id = p.id
LEFT JOIN hg_youban_publish_media m ON m.profile_id = p.id
GROUP BY p.id
ORDER BY p.id;

COMMIT;
