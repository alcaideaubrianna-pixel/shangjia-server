SET @now := NOW();
SET @addonsId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'addons' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @addonsId, '上架系统', 'youbanPublish', 'youbanPublish', 'icon-park-outline:upload-one', '2', '', '', '', '/addons/youbanPublish/index', '1', '', '0', '0', '', '1', '2', '0', '2', CONCAT('tr_', @addonsId, ' '), '32', '悦伴上架系统插件后台', '1', @now, @now
WHERE @addonsId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublish');

UPDATE `hg_admin_menu`
SET `title` = '上架系统',
    `pid` = @addonsId,
    `path` = 'youbanPublish',
    `icon` = 'icon-park-outline:upload-one',
    `type` = '2',
    `redirect` = '',
    `permissions` = '',
    `permission_name` = '',
    `component` = '/addons/youbanPublish/index',
    `always_show` = '1',
    `hidden` = '2',
    `level` = '2',
    `tree` = CONCAT('tr_', @addonsId, ' '),
    `sort` = '32',
    `status` = '1',
    `updated_at` = @now
WHERE @addonsId IS NOT NULL AND `name` = 'youbanPublish';

SET @publishId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'youbanPublish' LIMIT 1);
SET @publishTree := (SELECT `tree` FROM `hg_admin_menu` WHERE `name` = 'youbanPublish' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '租户管理', 'youbanPublishTenant', '', '', '3', '', '/youban_publish/publish/tenant/list,/youban_publish/publish/tenant/save,/youban_publish/publish/tenant/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '10', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishTenant');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '账号管理', 'youbanPublishAccount', '', '', '3', '', '/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/resetPwd,/youban_publish/publish/account/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '20', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishAccount');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '笔记资料', 'youbanPublishProfile', '', '', '3', '', '/youban_publish/publish/profile/list,/youban_publish/publish/profile/view,/youban_publish/publish/profile/edit,/youban_publish/publish/profile/delete,/youban_publish/publish/profile/review', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '34', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishProfile');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '导入任务', 'youbanPublishImportTask', '', '', '3', '', '/youban_publish/publish/importTask/list,/youban_publish/publish/importTask/create,/youban_publish/publish/importTask/view,/youban_publish/publish/importTask/start,/youban_publish/publish/importTask/cancel,/youban_publish/publish/importTask/retry,/youban_publish/publish/importTask/scan,/youban_publish/publish/importTask/repair', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '35', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishImportTask');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '导入记录', 'youbanPublishImportRun', '', '', '3', '', '/youban_publish/publish/importRun/list,/youban_publish/publish/importRun/create,/youban_publish/publish/importRun/delete,/youban_publish/publish/importRun/cancel,/youban_publish/publish/importRun/logs,/youban_publish/publish/importRun/clearLogs', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '36', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishImportRun');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '资料媒体', 'youbanPublishMedia', '', '', '3', '', '/youban_publish/publish/media/list,/youban_publish/publish/media/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '40', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishMedia');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '标签审核', 'youbanPublishTag', '', '', '3', '', '/youban_publish/publish/tag/list,/youban_publish/publish/tag/save,/youban_publish/publish/tag/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '50', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishTag');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '机器人配置', 'youbanPublishBot', '', '', '3', '', '/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '60', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishBot');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '插件配置', 'youbanPublishConfig', '', '', '3', '', '/youban_publish/publish/config/get,/youban_publish/publish/config/update', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '70', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishConfig');

UPDATE `hg_admin_menu`
SET `pid` = @publishId,
    `title` = CASE `name`
        WHEN 'youbanPublishTenant' THEN '租户管理'
        WHEN 'youbanPublishAccount' THEN '账号管理'
        WHEN 'youbanPublishProfile' THEN '笔记资料'
        WHEN 'youbanPublishImportTask' THEN '导入任务'
        WHEN 'youbanPublishImportRun' THEN '导入记录'
        WHEN 'youbanPublishMedia' THEN '资料媒体'
        WHEN 'youbanPublishTag' THEN '标签审核'
        WHEN 'youbanPublishBot' THEN '机器人配置'
        WHEN 'youbanPublishConfig' THEN '插件配置'
        ELSE `title`
    END,
    `type` = '3',
    `permissions` = CASE `name`
        WHEN 'youbanPublishTenant' THEN '/youban_publish/publish/tenant/list,/youban_publish/publish/tenant/save,/youban_publish/publish/tenant/delete'
        WHEN 'youbanPublishAccount' THEN '/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/resetPwd,/youban_publish/publish/account/delete'
        WHEN 'youbanPublishProfile' THEN '/youban_publish/publish/profile/list,/youban_publish/publish/profile/view,/youban_publish/publish/profile/edit,/youban_publish/publish/profile/delete,/youban_publish/publish/profile/review'
        WHEN 'youbanPublishImportTask' THEN '/youban_publish/publish/importTask/list,/youban_publish/publish/importTask/create,/youban_publish/publish/importTask/view,/youban_publish/publish/importTask/start,/youban_publish/publish/importTask/cancel,/youban_publish/publish/importTask/retry,/youban_publish/publish/importTask/scan,/youban_publish/publish/importTask/repair'
        WHEN 'youbanPublishImportRun' THEN '/youban_publish/publish/importRun/list,/youban_publish/publish/importRun/create,/youban_publish/publish/importRun/delete,/youban_publish/publish/importRun/cancel,/youban_publish/publish/importRun/logs,/youban_publish/publish/importRun/clearLogs'
        WHEN 'youbanPublishMedia' THEN '/youban_publish/publish/media/list,/youban_publish/publish/media/delete'
        WHEN 'youbanPublishTag' THEN '/youban_publish/publish/tag/list,/youban_publish/publish/tag/save,/youban_publish/publish/tag/delete'
        WHEN 'youbanPublishBot' THEN '/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete'
        WHEN 'youbanPublishConfig' THEN '/youban_publish/publish/config/get,/youban_publish/publish/config/update'
        ELSE `permissions`
    END,
    `active_menu` = 'youbanPublish',
    `hidden` = '1',
    `level` = '3',
    `tree` = CONCAT(@publishTree, 'tr_', @publishId, ' '),
    `sort` = CASE `name`
        WHEN 'youbanPublishTenant' THEN '10'
        WHEN 'youbanPublishAccount' THEN '20'
        WHEN 'youbanPublishProfile' THEN '34'
        WHEN 'youbanPublishImportTask' THEN '35'
        WHEN 'youbanPublishImportRun' THEN '36'
        WHEN 'youbanPublishMedia' THEN '40'
        WHEN 'youbanPublishTag' THEN '50'
        WHEN 'youbanPublishBot' THEN '60'
        WHEN 'youbanPublishConfig' THEN '70'
        ELSE `sort`
    END,
    `remark` = '上架系统按钮权限',
    `status` = '1',
    `updated_at` = @now
WHERE @publishId IS NOT NULL AND `name` IN (
  'youbanPublishTenant',
  'youbanPublishAccount',
  'youbanPublishProfile',
  'youbanPublishImportTask',
  'youbanPublishImportRun',
  'youbanPublishMedia',
  'youbanPublishTag',
  'youbanPublishBot',
  'youbanPublishConfig'
);

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN (
  'youbanPublish',
  'youbanPublishTenant',
  'youbanPublishAccount',
  'youbanPublishProfile',
  'youbanPublishImportTask',
  'youbanPublishImportRun',
  'youbanPublishMedia',
  'youbanPublishTag',
  'youbanPublishBot',
  'youbanPublishConfig'
)
WHERE r.`id` IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM `hg_admin_role_menu` rm WHERE rm.`role_id` = r.`id` AND rm.`menu_id` = m.`id`
  );

DELETE rm FROM `hg_admin_role_menu` rm
JOIN `hg_admin_menu` m ON m.`id` = rm.`menu_id`
WHERE m.`name` IN ('YoubanChat', 'YoubanChatWorkbench', 'YoubanChatConversationView', 'YoubanChatBot', 'YoubanChatBinding', 'YoubanChatOperator', 'YoubanChatFeature')
   OR m.`title` IN ('客服会话', '客服工作台', '客服会话详情')
   OR m.`component` = '/addons/youban_chat/index'
   OR m.`permissions` LIKE '/youban_chat/%';

UPDATE `hg_admin_menu`
SET `status` = '2',
    `hidden` = '1',
    `updated_at` = @now
WHERE `name` IN ('YoubanChat', 'YoubanChatWorkbench', 'YoubanChatConversationView', 'YoubanChatBot', 'YoubanChatBinding', 'YoubanChatOperator', 'YoubanChatFeature')
   OR `title` IN ('客服会话', '客服工作台', '客服会话详情')
   OR `component` = '/addons/youban_chat/index'
   OR `permissions` LIKE '/youban_chat/%';

UPDATE `hg_admin_menu`
SET `pid` = @addonsId,
    `title` = '插件管理',
    `path` = 'addons',
    `type` = '2',
    `redirect` = '',
    `permissions` = '/addons/selects,/addons/list',
    `component` = '/develop/addons/index',
    `always_show` = '1',
    `hidden` = '0',
    `level` = '2',
    `tree` = CONCAT('tr_', @addonsId, ' '),
    `status` = '1',
    `updated_at` = @now
WHERE @addonsId IS NOT NULL AND `name` = 'develop_addons';

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` = 'develop_addons'
WHERE (
    r.`id` IN (1, 2)
    OR EXISTS (
        SELECT 1
        FROM `hg_admin_role_menu` rm_root
        JOIN `hg_admin_menu` root ON root.`id` = rm_root.`menu_id`
        WHERE rm_root.`role_id` = r.`id`
          AND root.`name` = 'addons'
    )
)
AND NOT EXISTS (
    SELECT 1
    FROM `hg_admin_role_menu` rm
    WHERE rm.`role_id` = r.`id`
      AND rm.`menu_id` = m.`id`
);

DELETE rm FROM `hg_admin_role_menu` rm
JOIN `hg_admin_menu` m ON m.`id` = rm.`menu_id`
JOIN `hg_admin_menu` example ON example.`name` = 'hgexample'
WHERE m.`id` = example.`id`
   OR m.`tree` LIKE CONCAT(example.`tree`, 'tr_', example.`id`, ' %');

UPDATE `hg_admin_menu` m
JOIN (SELECT `id`, `tree` FROM `hg_admin_menu` WHERE `name` = 'hgexample' LIMIT 1) example
SET m.`status` = '2',
    m.`hidden` = '1',
    m.`updated_at` = @now
WHERE m.`name` = 'hgexample'
   OR m.`tree` LIKE CONCAT(example.`tree`, 'tr_', example.`id`, ' %');

UPDATE `hg_admin_menu` AS m
JOIN (
    SELECT `id`
    FROM `hg_admin_menu`
    WHERE `name` = 'addons'
    LIMIT 1
) AS root
SET m.`pid` = root.`id`,
    m.`title` = '插件管理',
    m.`path` = 'addons',
    m.`type` = '2',
    m.`redirect` = '',
    m.`permissions` = '/addons/selects,/addons/list',
    m.`component` = '/develop/addons/index',
    m.`always_show` = '1',
    m.`hidden` = '0',
    m.`level` = '2',
    m.`tree` = CONCAT('tr_', root.`id`, ' '),
    m.`status` = '1',
    m.`updated_at` = @now
WHERE m.`name` = 'develop_addons';

UPDATE `hg_admin_menu`
SET `redirect` = '/develop/addons/index',
    `updated_at` = @now
WHERE `name` = 'addons';
