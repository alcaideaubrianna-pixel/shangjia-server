INSERT INTO hg_youban_publish_note_search
  (tenant_id, account_id, profile_id, is_collected, status, search_text, deleted_at, updated_at)
SELECT tenant_id, account_id, profile_id, is_collected, status,
       concat_ws(' ', profile_no, title, summary, plain_text), deleted_at, now()
FROM hg_youban_publish_note_index
WHERE deleted_at IS NULL
ON CONFLICT (tenant_id, account_id, profile_id) DO UPDATE SET
  is_collected = EXCLUDED.is_collected,
  status = EXCLUDED.status,
  search_text = EXCLUDED.search_text,
  deleted_at = EXCLUDED.deleted_at,
  updated_at = EXCLUDED.updated_at;
ANALYZE hg_youban_publish_note_search;
