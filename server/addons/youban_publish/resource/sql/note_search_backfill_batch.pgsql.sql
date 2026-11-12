-- Run this statement repeatedly. Change :last_id to the max id returned by the
-- final SELECT after every batch. Each execution is bounded to 500 rows and
-- is safe to resume after a timeout.
WITH batch AS (
  SELECT id, tenant_id, account_id, profile_id, is_collected, status,
         profile_no, title, summary, plain_text, deleted_at
  FROM hg_youban_publish_note_index
  WHERE id > :last_id
  ORDER BY id
  LIMIT 500
)
INSERT INTO hg_youban_publish_note_search
  (tenant_id, account_id, profile_id, is_collected, status, search_text, deleted_at, updated_at)
SELECT tenant_id, account_id, profile_id, is_collected, status,
       concat_ws(' ', profile_no, title, summary, plain_text), deleted_at, now()
FROM batch
ON CONFLICT (tenant_id, account_id, profile_id) DO UPDATE SET
  is_collected = EXCLUDED.is_collected,
  status = EXCLUDED.status,
  search_text = EXCLUDED.search_text,
  deleted_at = EXCLUDED.deleted_at,
  updated_at = EXCLUDED.updated_at;

SELECT max(id) AS next_last_id
FROM (
  SELECT id FROM hg_youban_publish_note_index
  WHERE id > :last_id ORDER BY id LIMIT 500
) batch_cursor;
