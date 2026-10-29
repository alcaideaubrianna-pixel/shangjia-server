-- Run separately before enabling the new scan entry on production.
-- CONCURRENTLY must run outside a transaction; do not add to plugin upgrade SQL.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_message_bot_file
ON hg_youban_publish_tg_message (bot_id, tg_file_id, account_id)
WHERE tg_file_id <> '' AND deleted_at IS NULL AND status = 'sent';

-- Batch Bot scan identity lookup. Keep tenant/account before the large IN list
-- so PostgreSQL can use an index-only scan and avoid filtering the send ledger.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_tg_message_identity_scope
ON hg_youban_publish_tg_message (tenant_id, account_id, tg_file_unique_id, profile_id)
WHERE tg_file_unique_id <> '' AND deleted_at IS NULL AND status = 'sent';
