-- Run before deploying Web/Publish/Account/Scheduler. Additive migration;
-- existing running jobs retain legacy selection when an Attempt exists.
BEGIN;
SET LOCAL lock_timeout = '3s';
ALTER TABLE "hg_youban_publish_profile_state" ADD COLUMN IF NOT EXISTS "random_media_config" text NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "display_media_selection" text NOT NULL DEFAULT '';
COMMIT;
