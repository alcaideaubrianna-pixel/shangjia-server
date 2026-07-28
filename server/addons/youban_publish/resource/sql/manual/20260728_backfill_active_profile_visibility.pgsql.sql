BEGIN;

SELECT COUNT(*) AS pending_profile_count
FROM "hg_content_profile" AS p
WHERE p."status" = 1
  AND p."visibility" <> 'public'
  AND p."deleted_at" IS NULL
  AND EXISTS (
    SELECT 1
    FROM "hg_youban_publish_profile_state" AS ps
    WHERE ps."profile_id" = p."id"
      AND ps."deleted_at" IS NULL
  );

UPDATE "hg_content_profile" AS p
SET "visibility" = 'public'
WHERE p."status" = 1
  AND p."visibility" <> 'public'
  AND p."deleted_at" IS NULL
  AND EXISTS (
    SELECT 1
    FROM "hg_youban_publish_profile_state" AS ps
    WHERE ps."profile_id" = p."id"
      AND ps."deleted_at" IS NULL
  );

SELECT COUNT(*) AS remaining_profile_count
FROM "hg_content_profile" AS p
WHERE p."status" = 1
  AND p."visibility" <> 'public'
  AND p."deleted_at" IS NULL
  AND EXISTS (
    SELECT 1
    FROM "hg_youban_publish_profile_state" AS ps
    WHERE ps."profile_id" = p."id"
      AND ps."deleted_at" IS NULL
  );

COMMIT;
