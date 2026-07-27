BEGIN;

LOCK TABLE "hg_youban_publish_media" IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE "ybp_media_single_owner_keep" ON COMMIT DROP AS
SELECT "id"
FROM "hg_youban_publish_media"
WHERE "deleted_at" IS NULL
  AND "task_id" IS NULL;

CREATE TEMP TABLE "ybp_media_single_owner_latest" ON COMMIT DROP AS
SELECT DISTINCT ON ("profile_id")
  "profile_id",
  "id" AS "task_id"
FROM "hg_youban_publish_task"
WHERE "profile_id" > 0
  AND "deleted_at" IS NULL
ORDER BY
  "profile_id",
  ("status" = 'published') DESC,
  "id" DESC;

INSERT INTO "ybp_media_single_owner_keep" ("id")
SELECT "m"."id"
FROM "hg_youban_publish_media" "m"
JOIN "ybp_media_single_owner_latest" "latest"
  ON "latest"."profile_id" = "m"."profile_id"
 AND "latest"."task_id" = "m"."task_id"
WHERE "m"."deleted_at" IS NULL
  AND "m"."task_id" IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM "hg_youban_publish_media" "current"
    WHERE "current"."profile_id" = "m"."profile_id"
      AND "current"."task_id" IS NULL
      AND "current"."deleted_at" IS NULL
  );

UPDATE "hg_youban_publish_media" "m"
SET "task_id" = NULL,
    "updated_at" = NOW()
WHERE "m"."id" IN (
  SELECT "id"
  FROM "ybp_media_single_owner_keep"
);

DELETE FROM "hg_youban_publish_media_phash_bucket" "bucket"
WHERE NOT EXISTS (
  SELECT 1
  FROM "hg_youban_publish_media" "m"
  WHERE "m"."id" = "bucket"."media_id"
    AND "m"."deleted_at" IS NULL
    AND "m"."task_id" IS NULL
);

DELETE FROM "hg_youban_publish_media_phash_lsh" "lsh"
WHERE NOT EXISTS (
  SELECT 1
  FROM "hg_youban_publish_media" "m"
  WHERE "m"."id" = "lsh"."media_id"
    AND "m"."deleted_at" IS NULL
    AND "m"."task_id" IS NULL
);

DELETE FROM "hg_youban_publish_media" "m"
WHERE "m"."task_id" IS NOT NULL;

DROP INDEX IF EXISTS "idx_ybp_media_task_attachment";
DROP INDEX IF EXISTS "idx_ybp_media_task_active";
DROP INDEX IF EXISTS "idx_ybp_media_task_sort";
DROP INDEX IF EXISTS "idx_ybp_media_purpose";
DROP INDEX IF EXISTS "idx_ybp_media_profile_current";

ALTER TABLE "hg_youban_publish_media"
  DROP COLUMN "task_id";

DROP INDEX IF EXISTS "idx_ybp_media_phash_bucket_lookup";
DROP INDEX IF EXISTS "idx_ybp_media_phash_lsh_search";

ALTER TABLE "hg_youban_publish_media_phash_bucket"
  DROP COLUMN "task_id";

ALTER TABLE "hg_youban_publish_media_phash_lsh"
  DROP COLUMN "task_id";

CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile_current"
  ON "hg_youban_publish_media" ("profile_id", "purpose", "sort_index", "id")
  WHERE "deleted_at" IS NULL;

CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_lookup"
  ON "hg_youban_publish_media_phash_bucket"
    ("tenant_id", "media_type", "bucket_pos", "bucket_value", "account_id", "profile_id", "media_id");

CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_lsh_search"
  ON "hg_youban_publish_media_phash_lsh"
    ("tenant_id", "media_type", "bucket_pos", "bucket_value")
  INCLUDE ("media_id", "profile_id", "account_id", "hash_value");

WITH "media_counts" AS (
  SELECT
    "profile_id",
    COUNT(*) FILTER (WHERE "media_type" = 'image') AS "image_count",
    COUNT(*) FILTER (WHERE "media_type" = 'video') AS "video_count",
    COUNT(*) FILTER (WHERE "media_type" = 'video' AND "purpose" = 'verify') AS "verification_video_count"
  FROM "hg_youban_publish_media"
  WHERE "deleted_at" IS NULL
  GROUP BY "profile_id"
)
UPDATE "hg_content_profile" "profile"
SET "image_count" = COALESCE("counts"."image_count", 0),
    "video_count" = COALESCE("counts"."video_count", 0),
    "has_verification_video" = CASE
      WHEN COALESCE("counts"."verification_video_count", 0) > 0 THEN 1
      ELSE 0
    END,
    "updated_at" = NOW()
FROM "media_counts" "counts"
WHERE "profile"."id" = "counts"."profile_id";

COMMIT;
