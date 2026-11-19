-- Incident: Qidi_SKS2/17632 display and /17636 verification were split
-- across receivers in the globally deduplicated message facts.
-- This is an explicitly scoped, rerunnable data repair, not a migration.
-- Reuse only cached assets for the exact same source messages. Never copy
-- another Bot's file_id, update_id, token, or protocol account credentials.
-- After commit, use the authorized requeue_events operation for 20393191.
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '8s';
SELECT id FROM hg_youban_publish_collect_event WHERE id = 20393191 FOR UPDATE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_event d
        JOIN hg_youban_publish_collect_event original ON original.id = 19353277
        JOIN hg_youban_publish_collect_dispatch branch
          ON branch.event_id = d.id AND branch.tenant_id = 78
         AND branch.account_id = 615 AND branch.rule_id = 338
         AND branch.profile_id = 844980
        WHERE d.id = 20393191 AND d.tenant_id = 78 AND d.account_id = 615
          AND d.source_id = 351 AND d.source_chat_id = '-1003974614787'
          AND d.source_message_id = 17632 AND d.material_role = 'display'
          AND d.source_grouped_id = '14359098028287761'
          AND original.source_chat_id = '3974614787'
          AND original.source_message_id = d.source_message_id
          AND original.source_grouped_id = d.source_grouped_id
          AND original.raw_text = d.raw_text
    ) THEN RAISE EXCEPTION 'display identity or destination changed'; END IF;
    IF NOT EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_source
        WHERE id = 351 AND tenant_id = 78 AND account_id = 615
          AND bot_id = 64 AND status = 1 AND collect_enabled = 1
          AND channel_blacklisted = 0 AND deleted_at IS NULL
    ) THEN RAISE EXCEPTION 'target collection source no longer enabled'; END IF;
    IF (SELECT count(*) FROM hg_youban_publish_bot_message_source
        WHERE chat_id = '-1003974614787' AND message_id BETWEEN 17632 AND 17636
          AND deleted_at IS NULL) <> 5
    THEN RAISE EXCEPTION 'shared source facts incomplete'; END IF;
    IF (SELECT count(*) FROM hg_youban_publish_collect_event_media
        WHERE event_id = 19353277 AND source_chat_id = '3974614787'
          AND source_message_id BETWEEN 17632 AND 17635
          AND media_type = 'photo' AND cache_status = 'ready'
          AND storage_path <> '' AND file_url <> '') <> 4
    THEN RAISE EXCEPTION 'exact display cached assets unavailable'; END IF;
    IF NOT EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_event v
        JOIN hg_youban_publish_collect_event_media m ON m.event_id = v.id
        WHERE v.id = 19353290 AND v.material_role = 'verify'
          AND v.material_parent_event_id = 19353277
          AND v.source_chat_id = '3974614787' AND v.source_message_id = 17636
          AND m.id = 20693396 AND m.source_message_id = 17636
          AND m.media_type = 'video' AND m.cache_status = 'ready'
          AND m.storage_path <> '' AND m.file_url <> ''
    ) THEN RAISE EXCEPTION 'exact verification cached asset unavailable'; END IF;
    IF EXISTS (
        SELECT 1 FROM hg_youban_publish_collect_event
        WHERE tenant_id = 78 AND source_id = 351
          AND source_chat_id IN ('3974614787', '-1003974614787')
          AND source_message_id = 17636
          AND (account_id <> 615 OR material_role <> 'verify'
               OR material_parent_event_id <> 20393191)
    ) THEN RAISE EXCEPTION 'verification already bound differently'; END IF;
END $$;

INSERT INTO hg_youban_publish_collect_event
    (tenant_id,account_id,source_id,source_type,bot_id,source_chat_id,
     source_message_id,source_grouped_id,source_unique_key,raw_text,
     media_count,status,material_role,material_parent_event_id,
     material_group_status,received_at,created_at,updated_at,error_message)
SELECT 78,615,351,'bot',64,'-1003974614787',17636,'',
       'shared-fact-repair:78:615:351:3974614787:17636',
       v.raw_text,1,'media_ready','verify',20393191,'paired',
       v.received_at,localtimestamp,localtimestamp,''
FROM hg_youban_publish_collect_event v
WHERE v.id = 19353290 AND NOT EXISTS (
    SELECT 1 FROM hg_youban_publish_collect_event existing
    WHERE existing.tenant_id = 78 AND existing.source_id = 351
      AND existing.source_chat_id IN ('3974614787', '-1003974614787')
      AND existing.source_message_id = 17636
)
ON CONFLICT (source_unique_key) DO NOTHING;

INSERT INTO hg_youban_publish_collect_event_media
    (tenant_id,account_id,source_id,source_type,event_id,source_chat_id,
     source_message_id,source_grouped_id,source_media_key,media_type,
     source_ref_type,source_file_id,source_message_ref,file_url,storage_path,
     poster_url,source_size,file_md5,file_phash,meta_json,sort_index,
     cache_status,cache_hit,created_at,updated_at)
SELECT 78,615,351,'bot',target.id,'-1003974614787',m.source_message_id,
       target.source_grouped_id,
       'shared-fact-repair:3974614787:' || m.source_message_id,
       m.media_type,'url','',m.file_url,m.file_url,m.storage_path,
       m.poster_url,m.source_size,m.file_md5,m.file_phash,
       jsonb_build_object('repair','shared_fact_receiver_split',
                          'cachedFromEventMediaId',m.id)::text,
       m.sort_index,'ready',1,localtimestamp,localtimestamp
FROM hg_youban_publish_collect_event_media m
JOIN hg_youban_publish_collect_event target
  ON target.tenant_id = 78 AND target.account_id = 615 AND target.source_id = 351
 AND target.source_chat_id = '-1003974614787'
 AND ((m.event_id = 19353277 AND target.id = 20393191)
      OR (m.id = 20693396 AND target.source_message_id = 17636
          AND target.material_parent_event_id = 20393191))
WHERE ((m.event_id = 19353277 AND m.source_message_id IN (17634,17635))
       OR m.id = 20693396)
  AND NOT EXISTS (
      SELECT 1 FROM hg_youban_publish_collect_event_media existing
      WHERE existing.event_id = target.id
        AND existing.source_message_id = m.source_message_id
        AND existing.media_type = m.media_type
  );

DO $$
BEGIN
    IF (SELECT count(*) FROM hg_youban_publish_collect_event_media
        WHERE event_id = 20393191 AND cache_status = 'ready'
          AND source_message_id BETWEEN 17632 AND 17635) <> 4
    THEN RAISE EXCEPTION 'display media repair incomplete'; END IF;
END $$;

-- Reopen only this approved parent. Existing dispatch identity remains intact;
-- the normal collection handler updates the owned profile and republish path.
UPDATE hg_youban_publish_collect_event
SET media_count = 4,status = 'media_ready',processed_at = NULL,
    material_group_status = 'paired',error_message = '',updated_at = localtimestamp
WHERE id = 20393191 AND tenant_id = 78 AND account_id = 615 AND source_id = 351;
COMMIT;
