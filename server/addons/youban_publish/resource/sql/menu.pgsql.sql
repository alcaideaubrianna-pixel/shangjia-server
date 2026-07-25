INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  root."id", '上架系统', 'youbanPublish', 'youbanPublish', 'icon-park-outline:upload-one', 2,
  '', '', '', '/addons/youbanPublish/index', 1, '', 0,
  0, '', 1, 2, 0, 2, 'tr_' || root."id"::text || ' ',
  32, '悦伴上架系统插件后台', 1, NOW(), NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanPublish');

UPDATE "hg_admin_menu"
SET "title" = '上架系统',
    "pid" = root."id",
    "path" = 'youbanPublish',
    "icon" = 'icon-park-outline:upload-one',
    "type" = 2,
    "redirect" = '',
    "permissions" = '',
    "permission_name" = '',
    "component" = '/addons/youbanPublish/index',
    "always_show" = 1,
    "hidden" = 2,
    "level" = 2,
    "tree" = 'tr_' || root."id"::text || ' ',
    "sort" = 32,
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE "hg_admin_menu"."name" = 'youbanPublish';

WITH parent AS (
  SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanPublish' LIMIT 1
), buttons AS (
  SELECT *
  FROM (VALUES
    ('租户管理', 'youbanPublishTenant', '/youban_publish/publish/tenant/list,/youban_publish/publish/tenant/save,/youban_publish/publish/tenant/delete', 10),
    ('账号管理', 'youbanPublishAccount', '/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/resetPwd,/youban_publish/publish/account/delete', 20),
    ('上架任务', 'youbanPublishTask', '/youban_publish/publish/task/list,/youban_publish/publish/task/save,/youban_publish/publish/task/submit,/youban_publish/publish/task/cancel', 30),
    ('笔记资料', 'youbanPublishProfile', '/youban_publish/publish/profile/list,/youban_publish/publish/profile/view,/youban_publish/publish/profile/edit,/youban_publish/publish/profile/delete,/youban_publish/publish/profile/review', 34),
    ('导入任务', 'youbanPublishImportTask', '/youban_publish/publish/importTask/list,/youban_publish/publish/importTask/create,/youban_publish/publish/importTask/view,/youban_publish/publish/importTask/start,/youban_publish/publish/importTask/cancel,/youban_publish/publish/importTask/retry,/youban_publish/publish/importTask/scan,/youban_publish/publish/importTask/repair', 35),
    ('导入记录', 'youbanPublishImportRun', '/youban_publish/publish/importRun/list,/youban_publish/publish/importRun/create,/youban_publish/publish/importRun/delete,/youban_publish/publish/importRun/cancel,/youban_publish/publish/importRun/logs,/youban_publish/publish/importRun/clearLogs', 36),
    ('资料媒体', 'youbanPublishMedia', '/youban_publish/publish/media/list,/youban_publish/publish/media/delete', 40),
    ('标签审核', 'youbanPublishTag', '/youban_publish/publish/tag/list,/youban_publish/publish/tag/save,/youban_publish/publish/tag/delete', 50),
    ('机器人配置', 'youbanPublishBot', '/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete', 60),
    ('插件配置', 'youbanPublishConfig', '/youban_publish/publish/config/get,/youban_publish/publish/config/update', 70)
  ) AS t("title", "name", "permissions", "sort")
)
INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  parent."id", buttons."title", buttons."name", '', '', 3, '', buttons."permissions",
  '', '', 0, 'youbanPublish', 0, 0, '', 0, 1, 0, 3,
  parent."tree" || 'tr_' || parent."id"::text || ' ',
  buttons."sort", '上架系统按钮权限', 1, NOW(), NOW()
FROM parent
CROSS JOIN buttons
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" m WHERE m."name" = buttons."name");

WITH parent AS (
  SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanPublish' LIMIT 1
), buttons AS (
  SELECT *
  FROM (VALUES
    ('租户管理', 'youbanPublishTenant', '/youban_publish/publish/tenant/list,/youban_publish/publish/tenant/save,/youban_publish/publish/tenant/delete', 10),
    ('账号管理', 'youbanPublishAccount', '/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/resetPwd,/youban_publish/publish/account/delete', 20),
    ('上架任务', 'youbanPublishTask', '/youban_publish/publish/task/list,/youban_publish/publish/task/save,/youban_publish/publish/task/submit,/youban_publish/publish/task/cancel', 30),
    ('笔记资料', 'youbanPublishProfile', '/youban_publish/publish/profile/list,/youban_publish/publish/profile/view,/youban_publish/publish/profile/edit,/youban_publish/publish/profile/delete,/youban_publish/publish/profile/review', 34),
    ('导入任务', 'youbanPublishImportTask', '/youban_publish/publish/importTask/list,/youban_publish/publish/importTask/create,/youban_publish/publish/importTask/view,/youban_publish/publish/importTask/start,/youban_publish/publish/importTask/cancel,/youban_publish/publish/importTask/retry,/youban_publish/publish/importTask/scan,/youban_publish/publish/importTask/repair', 35),
    ('导入记录', 'youbanPublishImportRun', '/youban_publish/publish/importRun/list,/youban_publish/publish/importRun/create,/youban_publish/publish/importRun/delete,/youban_publish/publish/importRun/cancel,/youban_publish/publish/importRun/logs,/youban_publish/publish/importRun/clearLogs', 36),
    ('资料媒体', 'youbanPublishMedia', '/youban_publish/publish/media/list,/youban_publish/publish/media/delete', 40),
    ('标签审核', 'youbanPublishTag', '/youban_publish/publish/tag/list,/youban_publish/publish/tag/save,/youban_publish/publish/tag/delete', 50),
    ('机器人配置', 'youbanPublishBot', '/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete', 60),
    ('插件配置', 'youbanPublishConfig', '/youban_publish/publish/config/get,/youban_publish/publish/config/update', 70)
  ) AS t("title", "name", "permissions", "sort")
)
UPDATE "hg_admin_menu" m
SET "pid" = parent."id",
    "title" = buttons."title",
    "type" = 3,
    "permissions" = buttons."permissions",
    "active_menu" = 'youbanPublish',
    "hidden" = 1,
    "level" = 3,
    "tree" = parent."tree" || 'tr_' || parent."id"::text || ' ',
    "sort" = buttons."sort",
    "status" = 1,
    "updated_at" = NOW()
FROM parent, buttons
WHERE m."name" = buttons."name";

INSERT INTO "hg_admin_role_menu" ("role_id", "menu_id")
SELECT r."id", m."id"
FROM "hg_admin_role" r
JOIN "hg_admin_menu" m ON m."name" IN (
  'youbanPublish',
  'youbanPublishTenant',
  'youbanPublishAccount',
  'youbanPublishTask',
  'youbanPublishProfile',
  'youbanPublishImportTask',
  'youbanPublishImportRun',
  'youbanPublishMedia',
  'youbanPublishTag',
  'youbanPublishBot',
  'youbanPublishConfig'
)
WHERE r."id" IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM "hg_admin_role_menu" rm WHERE rm."role_id" = r."id" AND rm."menu_id" = m."id"
  );

DELETE FROM "hg_admin_role_menu" rm
USING "hg_admin_menu" m
WHERE m."id" = rm."menu_id"
  AND (
    m."name" IN ('YoubanChat', 'YoubanChatWorkbench', 'YoubanChatConversationView', 'YoubanChatBot', 'YoubanChatBinding', 'YoubanChatOperator', 'YoubanChatFeature')
    OR m."title" IN ('客服会话', '客服工作台', '客服会话详情')
    OR m."component" = '/addons/youban_chat/index'
    OR m."permissions" LIKE '/youban_chat/%'
  );

UPDATE "hg_admin_menu"
SET "status" = 2,
    "hidden" = 1,
    "updated_at" = NOW()
WHERE "name" IN ('YoubanChat', 'YoubanChatWorkbench', 'YoubanChatConversationView', 'YoubanChatBot', 'YoubanChatBinding', 'YoubanChatOperator', 'YoubanChatFeature')
   OR "title" IN ('客服会话', '客服工作台', '客服会话详情')
   OR "component" = '/addons/youban_chat/index'
   OR "permissions" LIKE '/youban_chat/%';

UPDATE "hg_admin_menu"
SET "pid" = root."id",
    "title" = '插件管理',
    "path" = 'addons',
    "type" = 2,
    "redirect" = '',
    "permissions" = '/addons/selects,/addons/list',
    "component" = '/develop/addons/index',
    "always_show" = 1,
    "hidden" = 0,
    "level" = 2,
    "tree" = 'tr_' || root."id"::text || ' ',
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE "hg_admin_menu"."name" = 'develop_addons';
