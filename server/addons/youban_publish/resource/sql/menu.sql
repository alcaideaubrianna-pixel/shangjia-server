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
SELECT NULL, @publishId, '商户管理', 'youbanPublishMerchant', '', '', '3', '', '/youban_publish/publish/merchant/list,/youban_publish/publish/merchant/save,/youban_publish/publish/merchant/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '10', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishMerchant');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '账号管理', 'youbanPublishAccount', '', '', '3', '', '/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '20', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishAccount');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '上架任务', 'youbanPublishTask', '', '', '3', '', '/youban_publish/publish/task/list,/youban_publish/publish/task/save,/youban_publish/publish/task/submit,/youban_publish/publish/task/cancel', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '30', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishTask');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '资料媒体', 'youbanPublishMedia', '', '', '3', '', '/youban_publish/publish/media/list,/youban_publish/publish/media/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '40', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishMedia');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '机器人配置', 'youbanPublishBot', '', '', '3', '', '/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '50', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishBot');
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @publishId, '插件配置', 'youbanPublishConfig', '', '', '3', '', '/youban_publish/publish/config/get,/youban_publish/publish/config/update', '', '', '0', 'youbanPublish', '0', '0', '', '0', '1', '0', '3', CONCAT(@publishTree, 'tr_', @publishId, ' '), '60', '上架系统按钮权限', '1', @now, @now
WHERE @publishId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublishConfig');

UPDATE `hg_admin_menu`
SET `pid` = @publishId,
    `title` = CASE `name`
        WHEN 'youbanPublishMerchant' THEN '商户管理'
        WHEN 'youbanPublishAccount' THEN '账号管理'
        WHEN 'youbanPublishTask' THEN '上架任务'
        WHEN 'youbanPublishMedia' THEN '资料媒体'
        WHEN 'youbanPublishBot' THEN '机器人配置'
        WHEN 'youbanPublishConfig' THEN '插件配置'
        ELSE `title`
    END,
    `type` = '3',
    `permissions` = CASE `name`
        WHEN 'youbanPublishMerchant' THEN '/youban_publish/publish/merchant/list,/youban_publish/publish/merchant/save,/youban_publish/publish/merchant/delete'
        WHEN 'youbanPublishAccount' THEN '/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/delete'
        WHEN 'youbanPublishTask' THEN '/youban_publish/publish/task/list,/youban_publish/publish/task/save,/youban_publish/publish/task/submit,/youban_publish/publish/task/cancel'
        WHEN 'youbanPublishMedia' THEN '/youban_publish/publish/media/list,/youban_publish/publish/media/delete'
        WHEN 'youbanPublishBot' THEN '/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete'
        WHEN 'youbanPublishConfig' THEN '/youban_publish/publish/config/get,/youban_publish/publish/config/update'
        ELSE `permissions`
    END,
    `active_menu` = 'youbanPublish',
    `hidden` = '1',
    `level` = '3',
    `tree` = CONCAT(@publishTree, 'tr_', @publishId, ' '),
    `sort` = CASE `name`
        WHEN 'youbanPublishMerchant' THEN '10'
        WHEN 'youbanPublishAccount' THEN '20'
        WHEN 'youbanPublishTask' THEN '30'
        WHEN 'youbanPublishMedia' THEN '40'
        WHEN 'youbanPublishBot' THEN '50'
        WHEN 'youbanPublishConfig' THEN '60'
        ELSE `sort`
    END,
    `remark` = '上架系统按钮权限',
    `status` = '1',
    `updated_at` = @now
WHERE @publishId IS NOT NULL AND `name` IN (
  'youbanPublishMerchant',
  'youbanPublishAccount',
  'youbanPublishTask',
  'youbanPublishMedia',
  'youbanPublishBot',
  'youbanPublishConfig'
);

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN (
  'youbanPublish',
  'youbanPublishMerchant',
  'youbanPublishAccount',
  'youbanPublishTask',
  'youbanPublishMedia',
  'youbanPublishBot',
  'youbanPublishConfig'
)
WHERE r.`id` IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM `hg_admin_role_menu` rm WHERE rm.`role_id` = r.`id` AND rm.`menu_id` = m.`id`
  );
