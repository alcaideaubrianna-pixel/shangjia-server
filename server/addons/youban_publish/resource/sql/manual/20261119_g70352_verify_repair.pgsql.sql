-- Explicit incident repair: source 3736052878/9059 belongs to display /9056.
-- Reuse the cached asset of that exact message, never another Bot's file_id.
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '8s';
SELECT id FROM hg_youban_publish_collect_event
WHERE id IN (20180612,20180629) ORDER BY id FOR UPDATE;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_event d
        JOIN hg_youban_publish_collect_event original ON original.id = 20180610
        JOIN hg_youban_publish_collect_event v ON v.id = 20180629
        JOIN hg_youban_publish_collect_event donor ON donor.id = 20180690
        JOIN hg_youban_publish_collect_dispatch branch ON branch.event_id = d.id
        WHERE d.id = 20180612 AND d.tenant_id = 78 AND d.account_id = 615
          AND d.source_id = 351 AND d.source_chat_id = '-1003736052878'
          AND d.source_message_id = 9056 AND d.material_role = 'display'
          AND original.source_chat_id = '3736052878'
          AND original.source_message_id = d.source_message_id
          AND original.raw_text = d.raw_text
          AND v.tenant_id = d.tenant_id AND v.account_id = d.account_id
          AND v.source_id = d.source_id AND v.source_chat_id = d.source_chat_id
          AND v.source_message_id = 9059
          AND v.material_parent_event_id IN (0,d.id)
          AND donor.material_parent_event_id = original.id
          AND donor.source_chat_id = original.source_chat_id
          AND donor.source_message_id = v.source_message_id
          AND donor.material_role = 'verify'
          AND branch.tenant_id = 78 AND branch.account_id = 615
          AND branch.rule_id = 338 AND branch.profile_id = 843948
    ) THEN RAISE EXCEPTION 'G70352 source or profile identity changed'; END IF;
    IF NOT EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_source
        WHERE id = 351 AND tenant_id = 78 AND account_id = 615
          AND bot_id = 64 AND status = 1 AND collect_enabled = 1
          AND channel_blacklisted = 0 AND deleted_at IS NULL
    ) THEN RAISE EXCEPTION 'G70352 source not enabled'; END IF;
END $$;

UPDATE hg_youban_publish_collect_event_media m
SET source_ref_type = 'url',source_file_id = '',source_message_ref = donor.file_url,
    file_url = donor.file_url,storage_path = donor.storage_path,
    poster_url = donor.poster_url,file_md5 = donor.file_md5,
    file_phash = donor.file_phash,source_size = donor.source_size,
    cache_status = 'ready',cache_hit = 1,error_message = '',next_retry_at = NULL,
    updated_at = localtimestamp
FROM hg_youban_publish_collect_event_media donor
WHERE m.id = 22428639 AND m.event_id = 20180629
  AND m.tenant_id = 78 AND m.account_id = 615 AND m.source_id = 351
  AND m.source_chat_id = '-1003736052878' AND m.source_message_id = 9059
  AND m.media_type = 'video' AND m.cache_status IN ('pending','ready')
  AND donor.id = 22428740 AND donor.event_id = 20180690
  AND donor.source_chat_id = '3736052878' AND donor.source_message_id = 9059
  AND donor.media_type = 'video' AND donor.cache_status = 'ready'
  AND donor.storage_path <> '' AND donor.file_url <> '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_event_media
        WHERE id = 22428639 AND cache_status = 'ready' AND file_url <> ''
          AND storage_path <> '' AND source_file_id = ''
    ) THEN RAISE EXCEPTION 'G70352 cached verification repair incomplete'; END IF;
END $$;
UPDATE hg_youban_publish_collect_event
SET material_role = 'verify',material_parent_event_id = 20180612,
    material_group_status = 'paired',status = 'media_ready',
    processed_at = NULL,error_message = '',updated_at = localtimestamp
WHERE id = 20180629 AND tenant_id = 78 AND account_id = 615 AND source_id = 351;
UPDATE hg_youban_publish_collect_event
SET material_group_status = 'paired',status = 'media_ready',
    processed_at = NULL,error_message = '',updated_at = localtimestamp
WHERE id = 20180612 AND tenant_id = 78 AND account_id = 615 AND source_id = 351;
COMMIT;
