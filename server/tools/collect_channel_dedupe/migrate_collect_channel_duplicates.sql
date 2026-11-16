-- Guarded migration. It intentionally refuses to mutate data until every
-- duplicate group has an unambiguous survivor and no reference remains.
-- The transaction is rolled back on any error. Run only through run.sh --apply.
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
SELECT pg_advisory_xact_lock(hashtext('youban:collect_channel:canonicalize'));

-- This tool must run after the count projection migration. The count tables use
-- chat-id text, while the bot/account/rule tables use collect_channel.id.
DO $$
BEGIN
  IF to_regclass('hg_youban_publish_collect_channel_count') IS NULL
     OR to_regclass('hg_youban_publish_collect_channel_count_member') IS NULL THEN
    RAISE EXCEPTION 'REFUSED: collect channel count projection tables are missing';
  END IF;
END $$;

CREATE TEMP TABLE collect_channel_duplicate_plan ON COMMIT DROP AS
WITH channels AS (
  SELECT id, tenant_id, telegram_chat_id,
         regexp_replace(btrim(telegram_chat_id), '^-100', '') AS canonical_chat_id,
         (telegram_chat_id LIKE '-100%') AS is_canonical,
         deleted_at
  FROM hg_youban_publish_collect_channel
  WHERE btrim(telegram_chat_id) <> ''
), ranked AS (
  SELECT *, row_number() OVER (
    PARTITION BY tenant_id, canonical_chat_id
    ORDER BY (deleted_at IS NULL) DESC, is_canonical DESC, id
  ) AS rn,
  count(*) OVER (PARTITION BY tenant_id, canonical_chat_id) AS group_count
  FROM channels
)
SELECT tenant_id, canonical_chat_id,
       id AS survivor_id,
       group_count - 1 AS duplicate_count
FROM ranked
WHERE rn = 1
  AND group_count > 1;

-- Safety gate: this first implementation refuses automatic merging when any
-- duplicate has references. A later reviewed migration may add per-table merge
-- rules after preview has been approved.
DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n
  FROM collect_channel_duplicate_plan p
  JOIN hg_youban_publish_collect_channel c
    ON c.tenant_id = p.tenant_id
   AND regexp_replace(btrim(c.telegram_chat_id), '^-100', '') = p.canonical_chat_id
   AND c.id <> p.survivor_id
  WHERE EXISTS (SELECT 1 FROM hg_youban_publish_collect_bot_channel r WHERE r.tenant_id=c.tenant_id AND r.channel_id=c.id AND r.deleted_at IS NULL)
     OR EXISTS (SELECT 1 FROM hg_youban_publish_collect_account_channel r WHERE r.tenant_id=c.tenant_id AND r.channel_id=c.id AND r.deleted_at IS NULL)
     OR EXISTS (SELECT 1 FROM hg_youban_publish_collect_rule_channel r WHERE r.tenant_id=c.tenant_id AND r.channel_id=c.id)
     OR EXISTS (SELECT 1 FROM hg_youban_publish_collect_channel_profile r WHERE r.tenant_id=c.tenant_id AND regexp_replace(btrim(r.channel_id), '^-100', '') = p.canonical_chat_id AND r.deleted_at IS NULL)
     OR EXISTS (SELECT 1 FROM hg_youban_publish_collect_channel_count r WHERE r.tenant_id=c.tenant_id AND regexp_replace(btrim(r.channel_id), '^-100', '') = p.canonical_chat_id)
     OR EXISTS (SELECT 1 FROM hg_youban_publish_collect_channel_count_member r WHERE r.tenant_id=c.tenant_id AND regexp_replace(btrim(r.channel_id), '^-100', '') = p.canonical_chat_id)
  IF n > 0 THEN
    RAISE EXCEPTION 'REFUSED: % duplicate channel rows still have references; review preview and create an explicit per-table merge plan', n;
  END IF;
END $$;

-- Only unreferenced duplicates reach this point. Canonicalize the survivor and
-- soft-delete duplicate rows. The unique index protects against concurrent writes.
UPDATE hg_youban_publish_collect_channel c
SET telegram_chat_id = '-100' || p.canonical_chat_id,
    updated_at = CURRENT_TIMESTAMP
FROM collect_channel_duplicate_plan p
WHERE c.id = p.survivor_id
  AND c.tenant_id = p.tenant_id;

UPDATE hg_youban_publish_collect_channel c
SET deleted_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
FROM collect_channel_duplicate_plan p
WHERE c.tenant_id = p.tenant_id
  AND regexp_replace(btrim(c.telegram_chat_id), '^-100', '') = p.canonical_chat_id
  AND c.id <> p.survivor_id
  AND c.deleted_at IS NULL;

COMMIT;
