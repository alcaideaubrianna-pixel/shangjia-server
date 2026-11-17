-- Run explicitly outside a transaction; never execute on application startup.
-- Supports tenant/profile pages and tenant/account/profile pages, ordered by time.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_record_log_profile_time
    ON hg_youban_publish_record_log (tenant_id, profile_id, created_at DESC, id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_record_log_account_profile_time
    ON hg_youban_publish_record_log (tenant_id, account_id, profile_id, created_at DESC, id DESC);
