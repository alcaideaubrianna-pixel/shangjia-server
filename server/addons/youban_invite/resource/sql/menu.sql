SET @now := NOW();

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, '0', '邀请返现', 'youbanInvite', '/addons/youbanInvite', 'icon-park-outline:share', '1', '', '', '', '/addons/youbanInvite/index', '1', '', '0', '0', '', '1', '2', '0', '1', '', '750', '邀请返现插件后台', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanInvite');

UPDATE `hg_admin_menu`
SET `title` = '邀请返现',
    `pid` = '0',
    `path` = '/addons/youbanInvite',
    `icon` = 'icon-park-outline:share',
    `type` = '1',
    `redirect` = '',
    `permissions` = '',
    `component` = '/addons/youbanInvite/index',
    `always_show` = '1',
    `hidden` = '2',
    `level` = '1',
    `tree` = '',
    `sort` = '750',
    `status` = '1',
    `updated_at` = @now
WHERE `name` = 'youbanInvite';
