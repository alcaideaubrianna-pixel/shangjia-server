-- Dry-run/report query. Text parsing and code conversion are performed by
-- scripts/backfill_profile_regions.go because region names must be resolved
-- through hg_sys_provinces rather than hard-coded mappings.
SELECT count(*) AS missing_region_profiles
FROM hg_content_profile
WHERE (COALESCE(province,'') = '' OR COALESCE(city,'') = '')
  AND COALESCE(plain_text,'') <> ''
  AND deleted_at IS NULL;
