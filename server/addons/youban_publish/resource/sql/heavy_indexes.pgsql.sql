CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_profile_note_order" ON "hg_content_profile" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_profile_cover" ON "hg_youban_publish_media" ("profile_id", "sort_index", "id") WHERE "deleted_at" IS NULL AND ("media_type" IS NULL OR "media_type" = '' OR "media_type" <> 'video');
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_tenant_updated" ON "hg_youban_publish_note_index" ("tenant_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_account_updated" ON "hg_youban_publish_note_index" ("account_id", "updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_updated_cursor" ON "hg_youban_publish_note_index" ("updated_at" DESC, "id" DESC) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_profile" ON "hg_youban_publish_note_index" ("profile_id") WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_title_trgm" ON "hg_youban_publish_note_index" USING gin ("title" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_note_index_plain_text_trgm" ON "hg_youban_publish_note_index" USING gin ("plain_text" gin_trgm_ops) WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_profile_source_uuid" ON "hg_content_profile" ("source_note_uuid") WHERE "deleted_at" IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_phash_lsh_lookup" ON "hg_youban_publish_media_phash_lsh" ("tenant_id", "media_type", "bucket_pos", "bucket_value", "account_id", "profile_id", "media_id") INCLUDE ("hash_value");
DROP INDEX CONCURRENTLY IF EXISTS "idx_ybp_media_phash_lsh_search";
