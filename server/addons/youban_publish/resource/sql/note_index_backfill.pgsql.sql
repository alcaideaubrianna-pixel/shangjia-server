-- 资料列表读模型回填脚本。
-- 独立执行，禁止放入插件安装/升级事务；大租户建议在低峰执行。
INSERT INTO "hg_youban_publish_note_index" (
  "tenant_id", "account_id", "profile_id", "task_id", "uuid", "profile_no",
  "title", "summary", "plain_text", "tag", "province", "city", "status",
  "visibility", "review_status", "task_status", "published_at", "source_updated_at",
  "created_at", "updated_at", "deleted_at"
)
SELECT
  t."tenant_id", t."account_id", p."id", t."id", p."source_note_uuid", p."profile_no",
  p."title", p."summary", p."plain_text",
  CASE WHEN p."source_type" = 'feiniu' THEN p."tag_params" ELSE p."cup_size" END,
  p."province", p."city", p."status", p."visibility", p."review_status", t."status",
  p."published_at", COALESCE(p."updated_at", t."updated_at"), p."created_at", p."updated_at", p."deleted_at"
FROM "hg_content_profile" p
JOIN "hg_youban_publish_task" t
  ON t."profile_id" = p."id"
 AND t."deleted_at" IS NULL
 AND t."id" = (
   SELECT t2."id"
   FROM "hg_youban_publish_task" t2
   WHERE t2."profile_id" = p."id"
     AND t2."deleted_at" IS NULL
   ORDER BY t2."id" DESC
   LIMIT 1
 )
WHERE p."deleted_at" IS NULL
ON CONFLICT ("tenant_id", "account_id", "profile_id") DO UPDATE SET
  "task_id" = EXCLUDED."task_id",
  "uuid" = EXCLUDED."uuid",
  "profile_no" = EXCLUDED."profile_no",
  "title" = EXCLUDED."title",
  "summary" = EXCLUDED."summary",
  "plain_text" = EXCLUDED."plain_text",
  "tag" = EXCLUDED."tag",
  "province" = EXCLUDED."province",
  "city" = EXCLUDED."city",
  "status" = EXCLUDED."status",
  "visibility" = EXCLUDED."visibility",
  "review_status" = EXCLUDED."review_status",
  "task_status" = EXCLUDED."task_status",
  "published_at" = EXCLUDED."published_at",
  "source_updated_at" = EXCLUDED."source_updated_at",
  "created_at" = EXCLUDED."created_at",
  "updated_at" = EXCLUDED."updated_at",
  "deleted_at" = EXCLUDED."deleted_at";

ANALYZE "hg_youban_publish_note_index";
