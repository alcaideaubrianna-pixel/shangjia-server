SET @now := NOW();
SET @addonsId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'addons' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @addonsId, 'Bot 管理', 'youbanBot', 'youbanBot', 'icon-park-outline:robot-one', '2', '', '', '', '/addons/youbanBot/index', '1', '', '0', '0', '', '1', '2', '0', '2', CONCAT('tr_', @addonsId, ' '), '34', '全局机器人插件后台', '1', @now, @now
WHERE @addonsId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanBot');

UPDATE `hg_admin_menu`
SET `title` = 'Bot 管理',
    `pid` = @addonsId,
    `path` = 'youbanBot',
    `icon` = 'icon-park-outline:robot-one',
    `type` = '2',
    `redirect` = '',
    `permissions` = '',
    `permission_name` = '',
    `component` = '/addons/youbanBot/index',
    `always_show` = '1',
    `hidden` = '2',
    `level` = '2',
    `tree` = CONCAT('tr_', @addonsId, ' '),
    `sort` = '34',
    `remark` = '全局机器人插件后台',
    `status` = '1',
    `updated_at` = @now
WHERE @addonsId IS NOT NULL AND `name` = 'youbanBot';

SET @botId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'youbanBot' LIMIT 1);
SET @botTree := (SELECT `tree` FROM `hg_admin_menu` WHERE `name` = 'youbanBot' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @botId, 'Bot 配置', 'youbanBotConfig', '', '', '3', '', '/youban_bot/bot/list,/youban_bot/bot/save,/youban_bot/bot/delete,/youban_bot/bot/refresh,/youban_bot/bot/restart,/youban_bot/bot/feature/list,/youban_bot/bot/feature/save,/youban_bot/bot/user/list,/youban_bot/bot/user/superAdmin,/youban_bot/bot/message/list,/youban_bot/bot/message/send', '', '', '0', 'youbanBot', '0', '0', '', '0', '1', '0', '3', CONCAT(@botTree, 'tr_', @botId, ' '), '10', '全局机器人按钮权限', '1', @now, @now
WHERE @botId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanBotConfig');

UPDATE `hg_admin_menu`
SET `pid` = @botId,
    `title` = 'Bot 配置',
    `type` = '3',
    `permissions` = '/youban_bot/bot/list,/youban_bot/bot/save,/youban_bot/bot/delete,/youban_bot/bot/refresh,/youban_bot/bot/restart,/youban_bot/bot/feature/list,/youban_bot/bot/feature/save,/youban_bot/bot/user/list,/youban_bot/bot/user/superAdmin,/youban_bot/bot/message/list,/youban_bot/bot/message/send',
    `active_menu` = 'youbanBot',
    `hidden` = '1',
    `level` = '3',
    `tree` = CONCAT(@botTree, 'tr_', @botId, ' '),
    `sort` = '10',
    `remark` = '全局机器人按钮权限',
    `status` = '1',
    `updated_at` = @now
WHERE @botId IS NOT NULL AND `name` = 'youbanBotConfig';

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @botId, '推送记录', 'youbanBotBroadcastRecords', 'broadcast-records', '', '2', '', '/youban_bot/bot/broadcast/list,/youban_bot/bot/broadcast/recipient/list', '', '/addons/youbanBot/broadcast-records', '0', 'youbanBot', '0', '0', '', '0', '2', '0', '3', CONCAT(@botTree, 'tr_', @botId, ' '), '9', '全局Bot推送记录', '1', @now, @now
WHERE @botId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanBotBroadcastRecords');

UPDATE `hg_admin_menu`
SET `title` = '推送记录', `path` = 'broadcast-records',
    `permissions` = '/youban_bot/bot/broadcast/list,/youban_bot/bot/broadcast/recipient/list',
    `component` = '/addons/youbanBot/broadcast-records', `active_menu` = 'youbanBot',
    `hidden` = '2', `status` = '1', `updated_at` = @now
WHERE `name` = 'youbanBotBroadcastRecords';

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN ('youbanBot', 'youbanBotConfig', 'youbanBotBroadcastRecords')
WHERE r.`id` IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM `hg_admin_role_menu` rm WHERE rm.`role_id` = r.`id` AND rm.`menu_id` = m.`id`
  );
