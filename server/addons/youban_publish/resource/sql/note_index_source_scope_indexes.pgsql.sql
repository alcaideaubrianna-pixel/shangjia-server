-- Autocommit required: never wrap CREATE INDEX CONCURRENTLY in a transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_content_profile_live_source_id
    ON hg_content_profile (source_type,id) INCLUDE (profile_no) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_note_index_tenant_source_created
    ON hg_youban_publish_note_index (tenant_id,is_collected,created_at DESC,id DESC)
    INCLUDE (account_id,profile_id) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_note_index_account_source_created
    ON hg_youban_publish_note_index (account_id,is_collected,created_at DESC,id DESC)
    INCLUDE (tenant_id,profile_id) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_note_index_tenant_source_published
    ON hg_youban_publish_note_index (tenant_id,is_collected,(COALESCE(published_at,'1970-01-01'::timestamp)) DESC,id DESC)
    INCLUDE (account_id,profile_id) WHERE deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_note_index_account_source_published
    ON hg_youban_publish_note_index (account_id,is_collected,(COALESCE(published_at,'1970-01-01'::timestamp)) DESC,id DESC)
    INCLUDE (tenant_id,profile_id) WHERE deleted_at IS NULL;
