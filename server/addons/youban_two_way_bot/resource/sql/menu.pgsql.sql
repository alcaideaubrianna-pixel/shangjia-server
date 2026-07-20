INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  root."id", '双向机器人', 'youbanTwoWayBot', 'youbanTwoWayBot', 'lucide:bot-message-square', 2,
  '', '', '', '/addons/youbanTwoWayBot/index', 1, '', 0,
  0, '', 1, 2, 0, 2, 'tr_' || root."id"::text || ' ',
  35, '双向机器人插件后台', 1, NOW(), NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanTwoWayBot');

UPDATE "hg_admin_menu"
SET "title" = '双向机器人',
    "pid" = root."id",
    "path" = 'youbanTwoWayBot',
    "icon" = 'lucide:bot-message-square',
    "type" = 2,
    "redirect" = '',
    "permissions" = '',
    "permission_name" = '',
    "component" = '/addons/youbanTwoWayBot/index',
    "always_show" = 1,
    "hidden" = 2,
    "level" = 2,
    "tree" = 'tr_' || root."id"::text || ' ',
    "sort" = 35,
    "remark" = '双向机器人插件后台',
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE "hg_admin_menu"."name" = 'youbanTwoWayBot';

INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  parent."id", '双向机器人列表', 'youbanTwoWayBotListPerm', '', '', 3, '',
  '/youban_two_way_bot/twoWayBot/list',
  '', '', 0, 'youbanTwoWayBot', 0, 0, '', 0, 1, 0, 3,
  parent."tree" || 'tr_' || parent."id"::text || ' ',
  10, '双向机器人列表权限', 1, NOW(), NOW()
FROM (SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanTwoWayBot' LIMIT 1) parent
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanTwoWayBotListPerm');

UPDATE "hg_admin_menu"
SET "pid" = parent."id",
    "title" = '双向机器人列表',
    "type" = 3,
    "permissions" = '/youban_two_way_bot/twoWayBot/list',
    "active_menu" = 'youbanTwoWayBot',
    "hidden" = 1,
    "level" = 3,
    "tree" = parent."tree" || 'tr_' || parent."id"::text || ' ',
    "sort" = 10,
    "remark" = '双向机器人列表权限',
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanTwoWayBot' LIMIT 1) parent
WHERE "hg_admin_menu"."name" = 'youbanTwoWayBotListPerm';

INSERT INTO "hg_admin_role_menu" ("role_id", "menu_id")
SELECT r."id", m."id"
FROM "hg_admin_role" r
JOIN "hg_admin_menu" m ON m."name" IN ('youbanTwoWayBot', 'youbanTwoWayBotListPerm')
WHERE r."id" IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM "hg_admin_role_menu" rm WHERE rm."role_id" = r."id" AND rm."menu_id" = m."id"
  );
