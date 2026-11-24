-- Account deletion and transfer previews count live media by tenant and account.
-- Keep the index partial so deleted media does not inflate the index or count path.
CREATE INDEX IF NOT EXISTS idx_ybp_media_tenant_account_live
    ON hg_youban_publish_media (tenant_id, account_id)
    WHERE deleted_at IS NULL;
