#!/usr/bin/env bash
set -euo pipefail

# Read-only production diagnostic. Configure PG* through the environment.
SAMPLE_SIZE="${SAMPLE_SIZE:-5000}"
psql -X -v ON_ERROR_STOP=1 -P pager=off <<SQL
WITH sample AS (
  SELECT p.id, p.profile_no, p.source_key, p.source_note_id,
         e.source_chat_id, e.source_message_id,
         COALESCE(NULLIF(s.source_username, ''), '') AS source_username
  FROM hg_content_profile p
  LEFT JOIN hg_youban_publish_collect_event e
    ON e.source_unique_key = p.source_key
  LEFT JOIN hg_youban_publish_collect_source s
    ON s.id = e.source_id
  WHERE p.source_type = 'youban_collect'
    AND p.deleted_at IS NULL
  ORDER BY random()
  LIMIT ${SAMPLE_SIZE}
)
SELECT
  COUNT(*) AS sampled,
  COUNT(*) FILTER (WHERE profile_no <> '') AS with_profile_no,
  COUNT(*) FILTER (WHERE source_key <> '') AS with_source_key,
  COUNT(*) FILTER (WHERE source_note_id > 0 OR source_message_id > 0) AS with_message_id,
  COUNT(*) FILTER (WHERE source_chat_id <> '') AS with_source_chat_id,
  COUNT(*) FILTER (WHERE source_username <> '') AS with_source_username,
  COUNT(*) FILTER (WHERE source_message_id > 0 AND (source_username <> '' OR source_chat_id <> '')) AS source_link_candidates
FROM sample;

WITH sample AS (
  SELECT p.id, p.profile_no, p.source_key, p.source_note_id,
         e.source_chat_id, e.source_message_id,
         COALESCE(NULLIF(s.source_username, ''), '') AS source_username
  FROM hg_content_profile p
  LEFT JOIN hg_youban_publish_collect_event e
    ON e.source_unique_key = p.source_key
  LEFT JOIN hg_youban_publish_collect_source s
    ON s.id = e.source_id
  WHERE p.source_type = 'youban_collect'
    AND p.deleted_at IS NULL
  ORDER BY random()
  LIMIT ${SAMPLE_SIZE}
)
SELECT id, profile_no, source_key, source_note_id, source_chat_id,
       source_message_id, source_username
FROM sample
WHERE profile_no = ''
   OR source_key = ''
   OR (source_note_id = 0 AND source_message_id = 0)
   OR source_message_id = 0
ORDER BY id
LIMIT 100;
SQL
