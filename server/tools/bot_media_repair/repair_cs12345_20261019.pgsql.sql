-- Bounded repair: tenant 80/account 620, profiles O38468/C66215 only.
-- All 22 COS objects were checked: HTTP 200, matching size and attachment ETag.
-- Preserves publication state, processing state and existing Telegram tasks.
BEGIN;
SET LOCAL lock_timeout = '3s';
SET LOCAL statement_timeout = '20s';
SELECT pg_advisory_xact_lock(hashtext('youban_publish:media_sync:profile:681235'));
SELECT pg_advisory_xact_lock(hashtext('youban_publish:media_sync:profile:681236'));
DO $repair$
DECLARE n integer;
BEGIN
  IF (SELECT count(*) FROM hg_content_profile p
      JOIN hg_youban_publish_profile_state ps ON ps.profile_id=p.id
      WHERE p.id IN (681235,681236) AND p.deleted_at IS NULL
        AND ps.tenant_id=80 AND ps.account_id=620 AND ps.deleted_at IS NULL) <> 2 THEN
    RAISE EXCEPTION 'Profile ownership changed; abort';
  END IF;
  SELECT count(*) INTO n FROM hg_youban_publish_media m
  WHERE m.tenant_id=80 AND m.account_id=620 AND m.profile_id IN (681235,681236)
    AND m.id BETWEEN 5175590 AND 5175611 AND m.deleted_at IS NULL
    AND EXISTS (SELECT 1 FROM hg_sys_attachment a
                WHERE a.path=m.storage_path AND a.size=m.size AND a.drive='cos' AND a.md5<>'');
  IF n <> 22 THEN RAISE EXCEPTION 'Expected 22 exact attachment matches; got %',n; END IF;

  UPDATE hg_youban_publish_media m
  SET attachment_id=a.id,original_attachment_id=a.id,md5=a.md5,updated_at=NOW()
  FROM hg_sys_attachment a
  WHERE m.tenant_id=80 AND m.account_id=620 AND m.profile_id IN (681235,681236)
    AND m.id BETWEEN 5175590 AND 5175611 AND m.deleted_at IS NULL
    AND a.id=(SELECT max(a2.id) FROM hg_sys_attachment a2
              WHERE a2.path=m.storage_path AND a2.size=m.size AND a2.drive='cos' AND a2.md5<>'');
  GET DIAGNOSTICS n=ROW_COUNT;
  IF n <> 22 THEN RAISE EXCEPTION 'Expected 22 updated media; got %',n; END IF;

  INSERT INTO hg_content_media
    (profile_id,source_asset_id,media_type,purpose,sort_index,original_storage_path,
     display_storage_path,preview_storage_path,binary_md5,perceptual_hash,
     process_status,encrypt_status,status,created_at,updated_at)
  SELECT m.profile_id,m.original_attachment_id,m.media_type,m.purpose,m.sort_index,
    m.original_storage_path,m.storage_path,
    CASE WHEN m.media_type='video' AND COALESCE(m.poster_storage_path,'')<>''
         THEN m.poster_storage_path ELSE m.storage_path END,
    m.md5,m.perceptual_hash,'raw','none',1,NOW(),NOW()
  FROM hg_youban_publish_media m
  WHERE m.tenant_id=80 AND m.account_id=620 AND m.profile_id IN (681235,681236)
    AND m.id BETWEEN 5175590 AND 5175611 AND m.deleted_at IS NULL
    AND NOT EXISTS (SELECT 1 FROM hg_content_media c
                    WHERE c.profile_id=m.profile_id AND c.source_asset_id=m.original_attachment_id
                      AND c.media_type=m.media_type AND c.purpose=m.purpose
                      AND c.sort_index=m.sort_index AND c.deleted_at IS NULL);

  UPDATE hg_content_profile p
  SET cover_media_id=(SELECT c.id FROM hg_content_media c
                       WHERE c.profile_id=p.id AND c.media_type='image' AND c.deleted_at IS NULL
                       ORDER BY c.sort_index,c.id LIMIT 1),updated_at=NOW()
  WHERE p.id IN (681235,681236) AND p.deleted_at IS NULL;
END $repair$;
COMMIT;
