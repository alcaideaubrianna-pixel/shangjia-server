CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Task-log retention indexes are built concurrently to avoid blocking online writes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_tg_job_log_created" ON "hg_youban_publish_tg_job_log" ("created_at", "id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_import_run_log_created" ON "hg_youban_publish_import_run_log" ("created_at", "id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_event_log_created" ON "hg_youban_publish_collect_event_log" ("created_at", "id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_history_log_created" ON "hg_youban_publish_collect_history_log" ("created_at", "id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_cycle_run_log_created" ON "hg_youban_publish_cycle_run_log" ("created_at", "id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_lazysheep_tggo_push_log_created" ON "hg_addon_lazysheep_tggo_push_log" ("created_at", "id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_lazysheep_tggo_webhook_log_created" ON "hg_addon_lazysheep_tggo_webhook_log" ("created_at", "id");

CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_profile_note_order" ON "hg_content_profile" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_profile_cover" ON "hg_youban_publish_media" ("profile_id", "sort_index", "id") WHERE "deleted_at" IS NULL AND ("media_type" IS NULL OR "media_type" = '' OR "media_type" <> 'video');
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_processing_queue" ON "hg_youban_publish_media" ("processing_status", "updated_at") WHERE "deleted_at" IS NULL AND "processing_status" IN ('uploaded', 'processing', 'failed');
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_tenant_updated" ON "hg_youban_publish_note_index" ("tenant_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_account_updated" ON "hg_youban_publish_note_index" ("account_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_updated_cursor" ON "hg_youban_publish_note_index" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_tenant_created" ON "hg_youban_publish_note_index" ("tenant_id", "created_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_account_created" ON "hg_youban_publish_note_index" ("account_id", "created_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_scope_created" ON "hg_youban_publish_note_index" ("tenant_id", "account_id", "created_at" DESC, "id" DESC, "profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_tenant_published" ON "hg_youban_publish_note_index" ("tenant_id", (COALESCE("published_at", '1970-01-01'::timestamp)) DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_account_published" ON "hg_youban_publish_note_index" ("account_id", (COALESCE("published_at", '1970-01-01'::timestamp)) DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_profile" ON "hg_youban_publish_note_index" ("profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_title_trgm" ON "hg_youban_publish_note_index" USING gin ("title" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_profile_no_trgm" ON "hg_youban_publish_note_index" USING gin ("profile_no" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_summary_trgm" ON "hg_youban_publish_note_index" USING gin ("summary" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_plain_text_trgm" ON "hg_youban_publish_note_index" USING gin ("plain_text" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_profile_source_uuid" ON "hg_content_profile" ("source_note_uuid") WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_phash_lsh_lookup" ON "hg_youban_publish_media_phash_lsh" ("tenant_id", "media_type", "bucket_pos", "bucket_value", "account_id", "profile_id", "media_id") INCLUDE ("hash_value");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_tg_message_file_unique" ON "hg_youban_publish_tg_message" ("tg_file_unique_id", "account_id", "id" DESC) WHERE "tg_file_unique_id" <> '' AND "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_profile_state_dashboard" ON "hg_youban_publish_profile_state" ("tenant_id", "publish_task_status", "profile_id") WHERE "deleted_at" IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS "idx_ybp_media_phash_lsh_search";
CREATE EXTENSION IF NOT EXISTS vector;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_fingerprint_hnsw" ON "hg_youban_publish_media_fingerprint" USING hnsw ("phash_bits" bit_hamming_ops) WITH (m=8, ef_construction=64);
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_fingerprint_exact_scope" ON "hg_youban_publish_media_fingerprint" ("phash_bits", "tenant_id", "account_id", "profile_id", "media_id") WHERE "deleted_at" IS NULL AND "media_type" = 'image' AND "phash_bits" IS NOT NULL;
