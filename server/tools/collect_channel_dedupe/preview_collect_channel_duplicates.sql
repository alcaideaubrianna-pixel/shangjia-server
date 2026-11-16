-- Collect channel duplicate preview. Read-only by design.
-- Usage: psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f preview_collect_channel_duplicates.sql
\pset pager off
\echo '== duplicate canonical channel groups =='
WITH channels AS (
  SELECT id, tenant_id, telegram_chat_id,
         regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id,
         title, username, status, deleted_at
  FROM hg_youban_publish_collect_channel
  WHERE btrim(telegram_chat_id) <> ''
), duplicates AS (
  SELECT tenant_id, canonical_chat_id
  FROM channels
  GROUP BY tenant_id, canonical_chat_id
  HAVING count(*) > 1
)
SELECT c.tenant_id, c.canonical_chat_id, c.id AS channel_id,
       c.telegram_chat_id, c.title, c.username, c.status, c.deleted_at
FROM channels c
JOIN duplicates d USING (tenant_id, canonical_chat_id)
ORDER BY c.tenant_id, c.canonical_chat_id, c.id;

\echo '== reference counts by duplicate channel =='
WITH channels AS (
  SELECT id, tenant_id,
         regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
  FROM hg_youban_publish_collect_channel
  WHERE btrim(telegram_chat_id) <> ''
), duplicates AS (
  SELECT tenant_id, canonical_chat_id
  FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
), refs AS (
  SELECT 'collect_bot_channel' AS ref_table, tenant_id, channel_id::bigint AS channel_id
  FROM hg_youban_publish_collect_bot_channel WHERE deleted_at IS NULL
  UNION ALL
  SELECT 'collect_account_channel', tenant_id, channel_id
  FROM hg_youban_publish_collect_account_channel WHERE deleted_at IS NULL
  UNION ALL
  SELECT 'collect_rule_channel', tenant_id, channel_id
  FROM hg_youban_publish_collect_rule_channel
)
SELECT c.tenant_id, c.canonical_chat_id, c.id AS channel_id,
       c.telegram_chat_id, r.ref_table, count(r.channel_id) AS reference_count
FROM channels c
JOIN duplicates d USING (tenant_id, canonical_chat_id)
LEFT JOIN refs r ON r.tenant_id = c.tenant_id AND r.channel_id = c.id
GROUP BY c.tenant_id, c.canonical_chat_id, c.id, c.telegram_chat_id, r.ref_table
ORDER BY c.tenant_id, c.canonical_chat_id, c.id, r.ref_table;

\echo '== text-chat references (these use Chat ID, not collect_channel.id) =='
WITH duplicate_groups AS (
  SELECT tenant_id,
         regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
  FROM hg_youban_publish_collect_channel
  WHERE btrim(telegram_chat_id) <> ''
  GROUP BY tenant_id, regexp_replace(btrim(telegram_chat_id), '^-100', '')
  HAVING count(*) > 1
), refs AS (
  SELECT 'collect_channel_profile' AS ref_table, tenant_id,
         regexp_replace(btrim(channel_id), '^-100', '') AS canonical_chat_id,
         count(*) AS reference_count
  FROM hg_youban_publish_collect_channel_profile
  WHERE deleted_at IS NULL AND btrim(channel_id) <> ''
  GROUP BY tenant_id, regexp_replace(btrim(channel_id), '^-100', '')
  UNION ALL
  SELECT 'collect_channel_count', tenant_id,
         regexp_replace(btrim(channel_id), '^-100', ''), count(*)
  FROM hg_youban_publish_collect_channel_count
  WHERE btrim(channel_id) <> ''
  GROUP BY tenant_id, regexp_replace(btrim(channel_id), '^-100', '')
  UNION ALL
  SELECT 'collect_channel_count_member', tenant_id,
         regexp_replace(btrim(channel_id), '^-100', ''), count(*)
  FROM hg_youban_publish_collect_channel_count_member
  WHERE btrim(channel_id) <> ''
  GROUP BY tenant_id, regexp_replace(btrim(channel_id), '^-100', '')
)
SELECT r.ref_table, r.tenant_id, r.canonical_chat_id, r.reference_count
FROM refs r
JOIN duplicate_groups d USING (tenant_id, canonical_chat_id)
ORDER BY r.tenant_id, r.canonical_chat_id, r.ref_table;

\echo '== authorization conflicts within one source and canonical channel =='
WITH channels AS (
  SELECT id, tenant_id,
         regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
  FROM hg_youban_publish_collect_channel
), grouped AS (
  SELECT ac.tenant_id, ac.account_id, ac.source_id, c.canonical_chat_id,
         count(*) AS relation_count,
         count(*) FILTER (WHERE ac.collect_enabled = 1 AND ac.status = 'enabled' AND ac.approval_status = 'approved') AS allowed_count,
         count(*) FILTER (WHERE ac.status = 'disabled' OR ac.approval_status = 'rejected') AS blocked_count,
         string_agg(ac.id::text, ',' ORDER BY ac.id) AS relation_ids
  FROM hg_youban_publish_collect_account_channel ac
  JOIN channels c ON c.tenant_id = ac.tenant_id AND c.id = ac.channel_id
  WHERE ac.deleted_at IS NULL
  GROUP BY ac.tenant_id, ac.account_id, ac.source_id, c.canonical_chat_id
  HAVING count(*) > 1
)
SELECT * FROM grouped ORDER BY tenant_id, account_id, source_id, canonical_chat_id;

\echo '== count projections using a duplicate chat-id alias =='
SELECT 'count' AS ref_table, tenant_id, account_id, rule_id, channel_id,
       profile_count, ready, last_seen_at
FROM hg_youban_publish_collect_channel_count
WHERE channel_id ~ '^-?[0-9]+$'
  AND EXISTS (
    SELECT 1
    FROM hg_youban_publish_collect_channel c1
    JOIN hg_youban_publish_collect_channel c2
      ON c2.tenant_id = c1.tenant_id
     AND c2.id <> c1.id
     AND regexp_replace(btrim(c2.telegram_chat_id), '^-100', '') =
         regexp_replace(btrim(c1.telegram_chat_id), '^-100', '')
    WHERE c1.tenant_id = hg_youban_publish_collect_channel_count.tenant_id
      AND regexp_replace(btrim(c1.telegram_chat_id), '^-100', '') =
          regexp_replace(btrim(hg_youban_publish_collect_channel_count.channel_id), '^-100', '')
  )
UNION ALL
SELECT 'count_member', tenant_id, account_id, rule_id, channel_id,
       reference_count, NULL, NULL
FROM hg_youban_publish_collect_channel_count_member
WHERE channel_id ~ '^-?[0-9]+$';

\echo '== rule-channel index/data hazards =='
SELECT 'rule_channel_same_rule_channel_multiple_owners' AS issue,
       rule_id, channel_id, count(*) AS row_count,
       string_agg(DISTINCT tenant_id::text || '/' || account_id::text, ',' ORDER BY tenant_id::text || '/' || account_id::text) AS owners
FROM hg_youban_publish_collect_rule_channel
GROUP BY rule_id, channel_id
HAVING count(DISTINCT tenant_id || ':' || account_id) > 1
ORDER BY rule_id, channel_id;

\echo '== dangling references (no foreign keys are assumed) =='
SELECT 'collect_bot_channel' AS ref_table, count(*) AS dangling_count
FROM hg_youban_publish_collect_bot_channel r
LEFT JOIN hg_youban_publish_collect_channel c
  ON c.tenant_id = r.tenant_id AND c.id = r.channel_id
WHERE r.deleted_at IS NULL AND c.id IS NULL
UNION ALL
SELECT 'collect_account_channel', count(*)
FROM hg_youban_publish_collect_account_channel r
LEFT JOIN hg_youban_publish_collect_channel c
  ON c.tenant_id = r.tenant_id AND c.id = r.channel_id
WHERE r.deleted_at IS NULL AND c.id IS NULL
UNION ALL
SELECT 'collect_rule_channel', count(*)
FROM hg_youban_publish_collect_rule_channel r
LEFT JOIN hg_youban_publish_collect_channel c
  ON c.tenant_id = r.tenant_id AND c.id = r.channel_id
WHERE c.id IS NULL
UNION ALL
SELECT 'collect_channel_profile', count(*)
FROM hg_youban_publish_collect_channel_profile r
LEFT JOIN hg_youban_publish_collect_channel c
  ON c.tenant_id = r.tenant_id
 AND regexp_replace(btrim(c.telegram_chat_id), '^-100', '') =
     regexp_replace(btrim(r.channel_id), '^-100', '')
WHERE r.deleted_at IS NULL AND c.id IS NULL;

\echo '== apply blockers =='
WITH channels AS (
  SELECT id, tenant_id,
         regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id
  FROM hg_youban_publish_collect_channel
), duplicate_groups AS (
  SELECT tenant_id, canonical_chat_id
  FROM channels GROUP BY tenant_id, canonical_chat_id HAVING count(*) > 1
), refs AS (
  SELECT tenant_id, channel_id::bigint AS channel_id FROM hg_youban_publish_collect_bot_channel WHERE deleted_at IS NULL
  UNION ALL SELECT tenant_id, channel_id FROM hg_youban_publish_collect_account_channel WHERE deleted_at IS NULL
  UNION ALL SELECT tenant_id, channel_id FROM hg_youban_publish_collect_rule_channel
  UNION ALL SELECT tenant_id, CASE WHEN channel_id ~ '^-?[0-9]+$' THEN channel_id::bigint END FROM hg_youban_publish_collect_channel_profile WHERE deleted_at IS NULL
  UNION ALL SELECT tenant_id, CASE WHEN channel_id ~ '^-?[0-9]+$' THEN channel_id::bigint END FROM hg_youban_publish_collect_channel_count
  UNION ALL SELECT tenant_id, CASE WHEN channel_id ~ '^-?[0-9]+$' THEN channel_id::bigint END FROM hg_youban_publish_collect_channel_count_member
)
SELECT 'duplicate_groups=' || count(*) FILTER (WHERE d.tenant_id IS NOT NULL),
       'referenced_duplicate_rows=' || count(*) FILTER (WHERE d.tenant_id IS NOT NULL AND r.channel_id IS NOT NULL),
       'rule_channel_owner_conflicts=' || (
         SELECT count(*) FROM (
           SELECT rule_id, channel_id
           FROM hg_youban_publish_collect_rule_channel
           GROUP BY rule_id, channel_id
           HAVING count(DISTINCT tenant_id || ':' || account_id) > 1
         ) x
       )
FROM channels c
LEFT JOIN duplicate_groups d USING (tenant_id, canonical_chat_id)
LEFT JOIN refs r ON r.tenant_id = c.tenant_id AND r.channel_id = c.id;
