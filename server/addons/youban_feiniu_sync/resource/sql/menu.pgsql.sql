INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  root."id", 'FeiNiu同步', 'youbanFeiniuSync', 'youbanFeiniuSync', 'icon-park-outline:database-sync', 2,
  '', '', '', '/addons/youbanFeiniuSync/index', 1, '', 0,
  0, '', 1, 2, 0, 2, 'tr_' || root."id"::text || ' ',
  33, 'FeiNiu数据同步插件后台', 1, NOW(), NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" WHERE "name" = 'youbanFeiniuSync');

UPDATE "hg_admin_menu"
SET "title" = 'FeiNiu同步',
    "pid" = root."id",
    "path" = 'youbanFeiniuSync',
    "icon" = 'icon-park-outline:database-sync',
    "type" = 2,
    "redirect" = '',
    "permissions" = '',
    "permission_name" = '',
    "component" = '/addons/youbanFeiniuSync/index',
    "always_show" = 1,
    "hidden" = 2,
    "level" = 2,
    "tree" = 'tr_' || root."id"::text || ' ',
    "sort" = 33,
    "remark" = 'FeiNiu数据同步插件后台',
    "status" = 1,
    "updated_at" = NOW()
FROM (SELECT "id" FROM "hg_admin_menu" WHERE "name" = 'addons' LIMIT 1) root
WHERE "hg_admin_menu"."name" = 'youbanFeiniuSync';

WITH parent AS (
  SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanFeiniuSync' LIMIT 1
), buttons AS (
  SELECT * FROM (VALUES
    ('大盘权限', 'youbanFeiniuSyncDashboardPerm', '/youban_feiniu_sync/sync/options/tenants,/youban_feiniu_sync/sync/options/adminAccounts,/youban_feiniu_sync/sync/dashboard,/youban_feiniu_sync/sync/dashboard/summary,/youban_feiniu_sync/sync/dashboard/trend,/youban_feiniu_sync/sync/dashboard/channelRank,/youban_feiniu_sync/sync/dashboard/recentRuns', 10, 'FeiNiu同步大盘权限'),
    ('配置权限', 'youbanFeiniuSyncConfigPerm', '/youban_feiniu_sync/sync/config/list,/youban_feiniu_sync/sync/config/view,/youban_feiniu_sync/sync/config/save,/youban_feiniu_sync/sync/config/delete,/youban_feiniu_sync/sync/config/autoSync,/youban_feiniu_sync/sync/config/test', 20, 'FeiNiu同步配置权限'),
    ('频道权限', 'youbanFeiniuSyncChannelPerm', '/youban_feiniu_sync/sync/channel/list,/youban_feiniu_sync/sync/channel/clear', 30, 'FeiNiu同步频道权限'),
    ('运行权限', 'youbanFeiniuSyncRunPerm', '/youban_feiniu_sync/sync/run/list,/youban_feiniu_sync/sync/run/view,/youban_feiniu_sync/sync/run/items,/youban_feiniu_sync/sync/run/start', 40, 'FeiNiu同步运行权限')
  ) AS t("title", "name", "permissions", "sort", "remark")
)
INSERT INTO "hg_admin_menu" (
  "pid", "title", "name", "path", "icon", "type", "redirect", "permissions",
  "permission_name", "component", "always_show", "active_menu", "is_root",
  "is_frame", "frame_src", "keep_alive", "hidden", "affix", "level", "tree",
  "sort", "remark", "status", "created_at", "updated_at"
)
SELECT
  parent."id", buttons."title", buttons."name", '', '', 3, '', buttons."permissions",
  '', '', 0, 'youbanFeiniuSync', 0, 0, '', 0, 1, 0, 3,
  parent."tree" || 'tr_' || parent."id"::text || ' ', buttons."sort", buttons."remark", 1, NOW(), NOW()
FROM parent
CROSS JOIN buttons
WHERE NOT EXISTS (SELECT 1 FROM "hg_admin_menu" m WHERE m."name" = buttons."name");

WITH parent AS (
  SELECT "id", "tree" FROM "hg_admin_menu" WHERE "name" = 'youbanFeiniuSync' LIMIT 1
), buttons AS (
  SELECT * FROM (VALUES
    ('大盘权限', 'youbanFeiniuSyncDashboardPerm', '/youban_feiniu_sync/sync/options/tenants,/youban_feiniu_sync/sync/options/adminAccounts,/youban_feiniu_sync/sync/dashboard,/youban_feiniu_sync/sync/dashboard/summary,/youban_feiniu_sync/sync/dashboard/trend,/youban_feiniu_sync/sync/dashboard/channelRank,/youban_feiniu_sync/sync/dashboard/recentRuns', 10, 'FeiNiu同步大盘权限'),
    ('配置权限', 'youbanFeiniuSyncConfigPerm', '/youban_feiniu_sync/sync/config/list,/youban_feiniu_sync/sync/config/view,/youban_feiniu_sync/sync/config/save,/youban_feiniu_sync/sync/config/delete,/youban_feiniu_sync/sync/config/autoSync,/youban_feiniu_sync/sync/config/test', 20, 'FeiNiu同步配置权限'),
    ('频道权限', 'youbanFeiniuSyncChannelPerm', '/youban_feiniu_sync/sync/channel/list,/youban_feiniu_sync/sync/channel/clear', 30, 'FeiNiu同步频道权限'),
    ('运行权限', 'youbanFeiniuSyncRunPerm', '/youban_feiniu_sync/sync/run/list,/youban_feiniu_sync/sync/run/view,/youban_feiniu_sync/sync/run/items,/youban_feiniu_sync/sync/run/start', 40, 'FeiNiu同步运行权限'),
    ('同步管理', 'youbanFeiniuSyncManage', '/youban_feiniu_sync/sync/dashboard', 1, 'FeiNiu同步兼容权限')
  ) AS t("title", "name", "permissions", "sort", "remark")
)
UPDATE "hg_admin_menu" m
SET "pid" = parent."id",
    "title" = buttons."title",
    "type" = 3,
    "permissions" = buttons."permissions",
    "active_menu" = 'youbanFeiniuSync',
    "hidden" = 1,
    "level" = 3,
    "tree" = parent."tree" || 'tr_' || parent."id"::text || ' ',
    "sort" = buttons."sort",
    "remark" = buttons."remark",
    "status" = 1,
    "updated_at" = NOW()
FROM parent, buttons
WHERE m."name" = buttons."name";

INSERT INTO "hg_admin_role_menu" ("role_id", "menu_id")
SELECT r."id", m."id"
FROM "hg_admin_role" r
JOIN "hg_admin_menu" m ON m."name" IN (
  'youbanFeiniuSync',
  'youbanFeiniuSyncDashboardPerm',
  'youbanFeiniuSyncConfigPerm',
  'youbanFeiniuSyncChannelPerm',
  'youbanFeiniuSyncRunPerm',
  'youbanFeiniuSyncManage'
)
WHERE r."id" IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM "hg_admin_role_menu" rm WHERE rm."role_id" = r."id" AND rm."menu_id" = m."id"
  );
