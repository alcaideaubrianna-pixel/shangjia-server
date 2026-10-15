-- 将旧版双向机器人 Token 幂等迁移到通用 Bot Token 表。
-- 旧版曾存在创建成功后 status 错误保存为 0 的问题，ready 状态应恢复为启用。
UPDATE "hg_youban_two_way_bot_bot"
SET "status" = 1, "updated_at" = NOW()
WHERE "deleted_at" IS NULL
  AND "status" = 0
  AND "setup_status" = 'ready'
  AND BTRIM("bot_token") <> '';

UPDATE "hg_youban_publish_bot" publish_bot
SET "status" = 1, "updated_at" = NOW()
FROM "hg_youban_two_way_bot_bot" two_way_bot
WHERE publish_bot."tenant_id" = two_way_bot."tenant_id"
  AND BTRIM(publish_bot."bot_token") = BTRIM(two_way_bot."bot_token")
  AND publish_bot."deleted_at" IS NULL
  AND publish_bot."remark" = '由旧版双向机器人迁移'
  AND two_way_bot."deleted_at" IS NULL
  AND two_way_bot."status" = 1
  AND two_way_bot."setup_status" = 'ready';

WITH legacy_bots AS (
  SELECT DISTINCT ON ("tenant_id", BTRIM("bot_token"))
    "tenant_id",
    "account_id",
    "name",
    "bot_username",
    BTRIM("bot_token") AS "bot_token",
    "status",
    "created_at",
    "updated_at"
  FROM "hg_youban_two_way_bot_bot"
  WHERE "deleted_at" IS NULL
    AND BTRIM("bot_token") <> ''
  ORDER BY "tenant_id", BTRIM("bot_token"), "status" DESC, "id" DESC
)
INSERT INTO "hg_youban_publish_bot" (
  "tenant_id",
  "bot_name",
  "bot_username",
  "bot_token",
  "remark",
  "status",
  "created_by",
  "updated_by",
  "created_at",
  "updated_at"
)
SELECT
  legacy."tenant_id",
  COALESCE(NULLIF(BTRIM(legacy."name"), ''), NULLIF(BTRIM(legacy."bot_username"), ''), '迁移机器人'),
  TRIM(LEADING '@' FROM BTRIM(legacy."bot_username")),
  legacy."bot_token",
  '由旧版双向机器人迁移',
  CASE WHEN legacy."status" = 1 THEN 1 ELSE 2 END,
  legacy."account_id",
  legacy."account_id",
  COALESCE(legacy."created_at", NOW()),
  COALESCE(legacy."updated_at", NOW())
FROM legacy_bots legacy
WHERE NOT EXISTS (
  SELECT 1
  FROM "hg_youban_publish_bot" current_bot
  WHERE current_bot."tenant_id" = legacy."tenant_id"
    AND BTRIM(current_bot."bot_token") = legacy."bot_token"
    AND current_bot."deleted_at" IS NULL
);
