SET @now := NOW();

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, '0', '悦伴客服', 'YoubanChat', '/youban-chat', 'MessageOutlined', '1', '/youban-chat/workbench', '', '', 'LAYOUT', '1', '', '0', '0', '', '0', '0', '0', '1', '', '91', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChat');

SET @rootId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'YoubanChat' LIMIT 1);

UPDATE `hg_admin_menu`
SET `title` = '悦伴客服',
    `pid` = '0',
    `path` = '/youban-chat',
    `icon` = 'MessageOutlined',
    `type` = '1',
    `redirect` = '/youban-chat/workbench',
    `permissions` = '',
    `component` = 'LAYOUT',
    `always_show` = '1',
    `hidden` = '0',
    `level` = '1',
    `tree` = '',
    `sort` = '91',
    `status` = '1',
    `updated_at` = @now
WHERE `id` = @rootId;

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @rootId, '客服工作台', 'YoubanChatWorkbench', 'workbench', '', '2', '', '/youban_chat/chat/list', '', '/addons/youban_chat/index', '1', '', '0', '0', '', '0', '0', '0', '2', CONCAT('tr_', @rootId, ' '), '10', '', '1', @now, @now
WHERE @rootId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChatWorkbench');

SET @listId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'YoubanChatWorkbench' LIMIT 1);

UPDATE `hg_admin_menu`
SET `pid` = @rootId,
    `title` = '客服工作台',
    `path` = 'workbench',
    `type` = '2',
    `permissions` = '/youban_chat/chat/list',
    `component` = '/addons/youban_chat/index',
    `always_show` = '1',
    `hidden` = '0',
    `level` = '2',
    `tree` = CONCAT('tr_', @rootId, ' '),
    `sort` = '10',
    `status` = '1',
    `updated_at` = @now
WHERE `id` = @listId;

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @listId, '客服会话详情', 'YoubanChatConversationView', '', '', '3', '', '/youban_chat/chat/view,/youban_chat/chat/messages', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @rootId, ' tr_', @listId, ' '), '10', '', '1', @now, @now
WHERE @listId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChatConversationView');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @listId, 'Bot管理', 'YoubanChatBot', '', '', '3', '', '/youban_chat/chat/botList,/youban_chat/chat/saveBot', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @rootId, ' tr_', @listId, ' '), '25', '', '1', @now, @now
WHERE @listId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChatBot');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @listId, '频道群绑定', 'YoubanChatBinding', '', '', '3', '', '/youban_chat/chat/bindingList,/youban_chat/chat/saveBinding,/youban_chat/chat/channelOptions', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @rootId, ' tr_', @listId, ' '), '30', '', '1', @now, @now
WHERE @listId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChatBinding');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @listId, '客服绑定', 'YoubanChatOperator', '', '', '3', '', '/youban_chat/chat/operatorList,/youban_chat/chat/saveOperator', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @rootId, ' tr_', @listId, ' '), '30', '', '1', @now, @now
WHERE @listId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChatOperator');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @listId, '功能配置', 'YoubanChatFeature', '', '', '3', '', '/youban_chat/chat/featureList,/youban_chat/chat/saveFeature', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @rootId, ' tr_', @listId, ' '), '50', '', '1', @now, @now
WHERE @listId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'YoubanChatFeature');

INSERT IGNORE INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN ('YoubanChat', 'YoubanChatWorkbench', 'YoubanChatConversationView', 'YoubanChatBot', 'YoubanChatBinding', 'YoubanChatOperator', 'YoubanChatFeature')
WHERE r.`id` IN (1, 2);
