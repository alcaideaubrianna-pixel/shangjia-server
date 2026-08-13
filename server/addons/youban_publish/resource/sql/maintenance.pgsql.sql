CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_remark_trgm" ON "hg_youban_publish_tenant" USING gin ("remark" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_ybp_account_username_trgm" ON "hg_youban_publish_account" USING gin ("username" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_content_profile_source_uuid" ON "hg_content_profile" ("source_note_uuid") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_content_profile_title_trgm" ON "hg_content_profile" USING gin ("title" gin_trgm_ops);
CREATE INDEX IF NOT EXISTS "idx_content_profile_profile_no_trgm" ON "hg_content_profile" USING gin ("profile_no" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_content_profile_summary_trgm" ON "hg_content_profile" USING gin ("summary" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_content_profile_plain_text_trgm" ON "hg_content_profile" USING gin ("plain_text" gin_trgm_ops);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_content_profile_no" ON "hg_content_profile" ("profile_no");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_tenant" ON "hg_youban_publish_tg_job_log" ("tenant_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_account" ON "hg_youban_publish_tg_job_log" ("tenant_id", "account_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_log_created" ON "hg_youban_publish_tg_job_log" ("created_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_collect_order" ON "hg_youban_publish_tg_job" ("channel_id", "target_chat_id", "collect_source_id", "collect_source_chat_id", "collect_source_message_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_similar_tenant" ON "hg_youban_publish_media" ("tenant_id", "media_type", "account_id", "profile_id", "id") WHERE "deleted_at" IS NULL AND "perceptual_hash" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_similar_account" ON "hg_youban_publish_media" ("account_id", "media_type", "profile_id", "id") WHERE "deleted_at" IS NULL AND "perceptual_hash" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_media_md5_scope" ON "hg_youban_publish_media" ("account_id", "md5", "tenant_id", "profile_id", "id") WHERE "deleted_at" IS NULL AND "md5" <> '';
