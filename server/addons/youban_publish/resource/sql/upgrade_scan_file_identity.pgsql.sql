-- Run separately before enabling the new scan entry on production.
-- CONCURRENTLY must run outside a transaction; do not add to plugin upgrade SQL.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_message_bot_file
ON hg_youban_publish_tg_message (bot_id, tg_file_id, account_id)
WHERE tg_file_id <> '' AND deleted_at IS NULL AND status = 'sent';
