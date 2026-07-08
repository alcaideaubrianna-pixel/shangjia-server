ALTER TABLE "hg_youban_publish_collect_rule"
  ADD COLUMN IF NOT EXISTS "full_match_enabled" smallint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_collect_rule"
  ADD COLUMN IF NOT EXISTS "delete_text_json" text;

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_enabled" smallint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_mode" varchar(32) NOT NULL DEFAULT 'recent_days';

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_days" integer NOT NULL DEFAULT 30;
