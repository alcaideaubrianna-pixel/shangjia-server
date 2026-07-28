-- 采集资料元数据回填（手动执行，不在服务启动时自动执行）
-- 1. 为资料 -> 采集分发关系增加查询索引
-- 2. 根据历史采集分发记录修复 content_profile.source_type
-- 采集源名称由接口按当前采集源表批量读取，不复制到资料表，避免源名称变更后产生脏数据

CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_dispatch_profile"
ON "hg_youban_publish_collect_dispatch" ("profile_id", "id");

UPDATE "hg_content_profile" p
SET "source_type" = 'youban_collect',
    "updated_at" = NOW()
WHERE p."deleted_at" IS NULL
  AND EXISTS (
    SELECT 1
    FROM "hg_youban_publish_collect_dispatch" d
    WHERE d."profile_id" = p."id"
  )
  AND COALESCE(p."source_type", '') <> 'youban_collect';

SELECT 'collect_profile_source_type_backfilled' AS "check_name",
       COUNT(*) AS "count"
FROM "hg_content_profile" p
WHERE p."deleted_at" IS NULL
  AND p."source_type" = 'youban_collect';

SELECT 'collect_dispatch_profile_index_ready' AS "check_name",
       COUNT(*) AS "count"
FROM "hg_youban_publish_collect_dispatch"
WHERE "profile_id" > 0;
