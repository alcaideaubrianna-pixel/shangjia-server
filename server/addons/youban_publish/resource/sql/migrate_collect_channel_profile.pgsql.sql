-- Run after upgrade_online.pgsql.sql. This backfill is intentionally separate
-- because it scans historical dispatch/event rows and may take time online.
INSERT INTO "hg_youban_publish_collect_channel_profile"
  ("tenant_id","account_id","source_id","rule_id","channel_id","profile_id","created_at","updated_at")
SELECT d."tenant_id",d."account_id",d."source_id",d."rule_id",
  CASE WHEN e."source_chat_id" LIKE '-100%' THEN e."source_chat_id" ELSE '-100' || e."source_chat_id" END,
  d."profile_id",COALESCE(d."created_at",CURRENT_TIMESTAMP),COALESCE(d."updated_at",CURRENT_TIMESTAMP)
FROM "hg_youban_publish_collect_dispatch" d
JOIN "hg_youban_publish_collect_event" e
  ON e."id"=d."event_id"
 AND e."tenant_id"=d."tenant_id"
 AND e."account_id"=d."account_id"
WHERE d."profile_id">0
  AND e."source_chat_id"<>''
  AND d."rule_id">0
ON CONFLICT ("tenant_id","account_id","rule_id","channel_id","profile_id") DO UPDATE SET
  "source_id"=EXCLUDED."source_id",
  "updated_at"=GREATEST("hg_youban_publish_collect_channel_profile"."updated_at",EXCLUDED."updated_at");
