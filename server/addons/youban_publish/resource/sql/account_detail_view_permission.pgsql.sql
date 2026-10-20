-- Targeted permission migration; avoids rerunning the full plugin upgrade.
-- Run explicitly before deploying the Web role. Safe to rerun.
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '10s';

ALTER TABLE public.hg_youban_publish_account_setting
  ADD COLUMN IF NOT EXISTS profile_detail_view_enabled smallint NOT NULL DEFAULT 0;

COMMIT;
