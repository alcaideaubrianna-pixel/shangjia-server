BEGIN;

ALTER TABLE "hg_youban_publish_collect_source"
ADD COLUMN IF NOT EXISTS "bot_collect_scope" varchar(16) NOT NULL DEFAULT 'chat';

CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_bot_scope"
ON "hg_youban_publish_collect_source" ("bot_id", "bot_collect_scope", "source_chat_id", "status");

SELECT column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'hg_youban_publish_collect_source'
  AND column_name = 'bot_collect_scope';

COMMIT;
