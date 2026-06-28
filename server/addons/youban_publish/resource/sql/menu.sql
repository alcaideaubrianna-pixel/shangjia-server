SET @now := NOW();
SET @addonsId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'addons' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @addonsId, '上架系统', 'youbanPublish', 'youbanPublish', 'icon-park-outline:upload-one', '2', '', '/youban_publish/publish/merchant/list,/youban_publish/publish/merchant/save,/youban_publish/publish/merchant/delete,/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/delete,/youban_publish/publish/task/list,/youban_publish/publish/task/save,/youban_publish/publish/task/submit,/youban_publish/publish/task/cancel,/youban_publish/publish/media/list,/youban_publish/publish/media/delete,/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete,/youban_publish/publish/config/get,/youban_publish/publish/config/update', '', '/addons/youbanPublish/index', '1', '', '0', '0', '', '1', '2', '0', '2', CONCAT('tr_', @addonsId, ' '), '32', '悦伴上架系统插件后台', '1', @now, @now
WHERE @addonsId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanPublish');

UPDATE `hg_admin_menu`
SET `title` = '上架系统',
    `pid` = @addonsId,
    `path` = 'youbanPublish',
    `icon` = 'icon-park-outline:upload-one',
    `type` = '2',
    `redirect` = '',
    `permissions` = '/youban_publish/publish/merchant/list,/youban_publish/publish/merchant/save,/youban_publish/publish/merchant/delete,/youban_publish/publish/account/list,/youban_publish/publish/account/save,/youban_publish/publish/account/delete,/youban_publish/publish/task/list,/youban_publish/publish/task/save,/youban_publish/publish/task/submit,/youban_publish/publish/task/cancel,/youban_publish/publish/media/list,/youban_publish/publish/media/delete,/youban_publish/publish/bot/list,/youban_publish/publish/bot/save,/youban_publish/publish/bot/delete,/youban_publish/publish/config/get,/youban_publish/publish/config/update',
    `component` = '/addons/youbanPublish/index',
    `always_show` = '1',
    `hidden` = '2',
    `level` = '2',
    `tree` = CONCAT('tr_', @addonsId, ' '),
    `sort` = '32',
    `status` = '1',
    `updated_at` = @now
WHERE @addonsId IS NOT NULL AND `name` = 'youbanPublish';

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` = 'youbanPublish'
WHERE r.`id` IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM `hg_admin_role_menu` rm WHERE rm.`role_id` = r.`id` AND rm.`menu_id` = m.`id`
  );
