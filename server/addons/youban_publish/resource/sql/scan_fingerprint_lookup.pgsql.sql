-- Run explicitly outside a transaction, before deploying fingerprint-first search.
-- Keep the account-leading index for existing account-scoped callers.
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_md5_lookup"
  ON "hg_youban_publish_media" ("md5")
  INCLUDE ("profile_id", "tenant_id", "account_id", "media_type")
  WHERE "deleted_at" IS NULL AND "md5" <> '';
