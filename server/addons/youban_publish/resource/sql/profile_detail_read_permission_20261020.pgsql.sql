-- Deploy before the new web role. Additive and compatible with the previous version.
-- No automatic grant: all existing accounts keep the permission disabled.
BEGIN;
SET LOCAL lock_timeout = '3s';
ALTER TABLE "hg_youban_publish_account_setting"
  ADD COLUMN IF NOT EXISTS "profile_detail_view_enabled" smallint NOT NULL DEFAULT 0;
COMMIT;
