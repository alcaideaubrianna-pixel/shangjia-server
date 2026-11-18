-- Materialize the existing account-wide deny on source relations before rollout.
-- Keep the account-scope row so future sources still inherit the deny.
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '15s';
WITH blocked AS MATERIALIZED (
    SELECT a.tenant_id, a.account_id, regexp_replace(c.telegram_chat_id, '^-100', '') AS chat
    FROM hg_youban_publish_collect_account_channel a
    JOIN hg_youban_publish_collect_channel c ON c.id = a.channel_id AND c.tenant_id = a.tenant_id
    WHERE a.source_id = 0 AND a.disabled_reason = 'intro_fee_channel_blacklist'
      AND a.deleted_at IS NULL AND c.deleted_at IS NULL
)
UPDATE hg_youban_publish_collect_account_channel a
SET status = 'disabled', collect_enabled = 0, updated_at = localtimestamp
FROM hg_youban_publish_collect_channel c, blocked b
WHERE a.channel_id = c.id AND c.tenant_id = a.tenant_id
  AND a.tenant_id = b.tenant_id AND a.account_id = b.account_id
  AND regexp_replace(c.telegram_chat_id, '^-100', '') = b.chat
  AND a.deleted_at IS NULL AND c.deleted_at IS NULL
  AND (a.status IS DISTINCT FROM 'disabled' OR a.collect_enabled IS DISTINCT FROM 0)
RETURNING a.id, a.tenant_id, a.account_id, a.source_id;
COMMIT;
