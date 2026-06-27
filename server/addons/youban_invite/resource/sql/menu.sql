SET @now := NOW();
SET @addonsId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'addons' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @addonsId, '邀请返现', 'youbanInvite', 'youbanInvite', 'icon-park-outline:share', '2', '', '/youban_invite/invite/config,/youban_invite/invite/saveConfig,/youban_invite/invite/list,/youban_invite/invite/saveRecord,/youban_invite/invite/delete', '', '/addons/youbanInvite/index', '1', '', '0', '0', '', '1', '2', '0', '2', CONCAT('tr_', @addonsId, ' '), '30', '邀请返现插件后台', '1', @now, @now
WHERE @addonsId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanInvite');

UPDATE `hg_admin_menu`
SET `title` = '邀请返现',
    `pid` = @addonsId,
    `path` = 'youbanInvite',
    `icon` = 'icon-park-outline:share',
    `type` = '2',
    `redirect` = '',
    `permissions` = '/youban_invite/invite/config,/youban_invite/invite/saveConfig,/youban_invite/invite/list,/youban_invite/invite/saveRecord,/youban_invite/invite/delete',
    `component` = '/addons/youbanInvite/index',
    `always_show` = '1',
    `hidden` = '2',
    `level` = '2',
    `tree` = CONCAT('tr_', @addonsId, ' '),
    `sort` = '30',
    `status` = '1',
    `updated_at` = @now
WHERE @addonsId IS NOT NULL AND `name` = 'youbanInvite';

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` = 'youbanInvite'
WHERE r.`id` IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM `hg_admin_role_menu` rm WHERE rm.`role_id` = r.`id` AND rm.`menu_id` = m.`id`
  );
