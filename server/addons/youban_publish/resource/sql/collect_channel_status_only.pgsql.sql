-- Run before deploying status-only channel authorization.
-- Freeze administrative channel changes during normalization and rollout.
-- Preserve previous source-scoped authorization, including explicit rejection.
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '30s';
UPDATE hg_youban_publish_collect_account_channel
SET status = CASE
    WHEN lower(trim(status)) IN ('enabled', 'approved')
        AND collect_enabled = 1
        AND lower(trim(coalesce(approval_status, ''))) <> 'rejected'
    THEN 'enabled'
    ELSE 'disabled'
END
WHERE status IS DISTINCT FROM CASE
    WHEN lower(trim(status)) IN ('enabled', 'approved')
        AND collect_enabled = 1
        AND lower(trim(coalesce(approval_status, ''))) <> 'rejected'
    THEN 'enabled'
    ELSE 'disabled'
END;
ALTER TABLE hg_youban_publish_collect_account_channel ALTER COLUMN status SET DEFAULT 'disabled';
ALTER TABLE hg_youban_publish_collect_account_channel
    ADD CONSTRAINT ck_ybp_collect_account_channel_status
    CHECK (status IS NOT NULL AND status IN ('enabled', 'disabled'));
COMMIT;
