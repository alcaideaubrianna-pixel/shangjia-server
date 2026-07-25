CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_remark_trgm" ON "hg_youban_publish_tenant" USING gin ("remark" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_ybp_account_username_trgm" ON "hg_youban_publish_account" USING gin ("username" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_ybp_task_title_trgm" ON "hg_youban_publish_task" USING gin ("title" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_ybp_task_plain_text_trgm" ON "hg_youban_publish_task" USING gin ("plain_text" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_content_profile_source_uuid" ON "hg_content_profile" ("source_note_uuid") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_task_profile_status" ON "hg_youban_publish_task" ("profile_id", "status", "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_active" ON "hg_youban_publish_media" ("task_id", "deleted_at", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_content_profile_title_trgm" ON "hg_content_profile" USING gin ("title" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_content_profile_plain_text_trgm" ON "hg_content_profile" USING gin ("plain_text" gin_trgm_ops);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_content_profile_no" ON "hg_content_profile" ("profile_no");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_tenant" ON "hg_youban_publish_tg_job_log" ("tenant_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_account" ON "hg_youban_publish_tg_job_log" ("tenant_id", "account_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_created" ON "hg_youban_publish_tg_job_log" ("created_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_task_collect_order" ON "hg_youban_publish_task" ("collect_source_id", "collect_source_chat_id", "collect_source_message_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_collect_order" ON "hg_youban_publish_tg_job" ("channel_id", "target_chat_id", "collect_source_id", "collect_source_chat_id", "collect_source_message_id", "status", "id");
UPDATE "hg_youban_publish_task" t SET
  "collect_event_id"=e."id",
  "collect_source_id"=e."source_id",
  "collect_source_chat_id"=e."source_chat_id",
  "collect_source_message_id"=e."source_message_id"
FROM "hg_youban_publish_collect_event" e
WHERE t."tenant_id"=e."tenant_id"
  AND t."account_id"=e."account_id"
  AND t."collect_source_message_id"=0
  AND t."client_request_id" LIKE ('collect:' || e."source_unique_key" || ':%');
UPDATE "hg_youban_publish_tg_job" j SET
  "collect_event_id"=t."collect_event_id",
  "collect_source_id"=t."collect_source_id",
  "collect_source_chat_id"=t."collect_source_chat_id",
  "collect_source_message_id"=t."collect_source_message_id"
FROM "hg_youban_publish_task" t
WHERE j."task_id"=t."id" AND j."collect_source_message_id"=0 AND t."collect_source_message_id">0;
CREATE INDEX IF NOT EXISTS "idx_ybp_media_similar_tenant" ON "hg_youban_publish_media" ("tenant_id", "media_type", "account_id", "profile_id", "id") WHERE "deleted_at" IS NULL AND "perceptual_hash" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_similar_account" ON "hg_youban_publish_media" ("account_id", "media_type", "profile_id", "id") WHERE "deleted_at" IS NULL AND "perceptual_hash" <> '';
