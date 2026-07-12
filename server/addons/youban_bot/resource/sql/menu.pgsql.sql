INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  root."id", 'Bot 管理', 'youbanBot', 'youbanBot', 'icon-park-outline:robot-one', 2,
  '', '', '', '/addons/youbanBot/index', 1, '', 0,
  0, '', 1, 2, 0, 2, 'tr_' || root."id"::text || ' ',
  34, '全局机器人插件后台', 1, NOW(), NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanBot');

UPDATE "hg_admin_menu"
SET "title" = 'Bot 管理',
    "pid" = root."id",
    "path" = 'youbanBot',
    "icon" = 'icon-park-outline:robot-one',
    "type" = 2,
    "redirect" = '',
    "permissions" = '',
    "permission_name" = '',
    "component" = '/addons/youbanBot/index',
    "always_show" = 1,
    "active_menu" = '',
    "is_root" = 0,
    "is_frame" = 0,
    "frame_src" = '',
    "keep_alive" = 1,
    "hidden" = 2,
    "affix" = 0,
    "level" = 2,
    "tree" = 'tr_' || root."id"::text || ' ',
    "sort" = 34,
    "remark" = '全局机器人插件后台',
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE "hg_admin_menu"."name" = 'youbanBot';

INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  parent."id", 'Bot 配置', 'youbanBotConfig', '', '', 3, '',
  '/youban_bot/bot/list,/youban_bot/bot/save,/youban_bot/bot/delete,/youban_bot/bot/refresh,/youban_bot/bot/restart,/youban_bot/bot/feature/list,/youban_bot/bot/feature/save,/youban_bot/bot/user/list,/youban_bot/bot/user/superAdmin,/youban_bot/bot/message/list,/youban_bot/bot/message/send',
  '', '', 0, 'youbanBot', 0, 0, '', 0, 1, 0, 3,
  parent."tree" || 'tr_' || parent."id"::text || ' ',
  10, '全局机器人按钮权限', 1, NOW(), NOW()
FROM (SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanBot' LIMIT 1) parent
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanBotConfig');

UPDATE "hg_admin_menu"
SET "pid" = parent."id",
    "title" = 'Bot 配置',
    "type" = 3,
    "permissions" = '/youban_bot/bot/list,/youban_bot/bot/save,/youban_bot/bot/delete,/youban_bot/bot/refresh,/youban_bot/bot/restart,/youban_bot/bot/feature/list,/youban_bot/bot/feature/save,/youban_bot/bot/user/list,/youban_bot/bot/user/superAdmin,/youban_bot/bot/message/list,/youban_bot/bot/message/send',
    "component" = '',
    "active_menu" = 'youbanBot',
    "hidden" = 1,
    "level" = 3,
    "tree" = parent."tree" || 'tr_' || parent."id"::text || ' ',
    "sort" = 10,
    "remark" = '全局机器人按钮权限',
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanBot' LIMIT 1) parent
WHERE "hg_admin_menu"."name" = 'youbanBotConfig';

INSERT INTO "hg_admin_role_menu" ("role_id", "menu_id")
SELECT r."id", m."id"
FROM "hg_admin_role" r
JOIN "hg_admin_menu" m ON m."name" IN ('youbanBot', 'youbanBotConfig')
WHERE r."id" IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM "hg_admin_role_menu" rm WHERE rm."role_id" = r."id" AND rm."menu_id" = m."id"
  );
