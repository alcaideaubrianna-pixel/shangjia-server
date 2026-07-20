SET @now := NOW();
SET @addonsId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'addons' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @addonsId, 'FeiNiu同步', 'youbanFeiniuSync', 'youbanFeiniuSync', 'icon-park-outline:database-sync', '2', '', '', '', '/addons/youbanFeiniuSync/index', '1', '', '0', '0', '', '1', '2', '0', '2', CONCAT('tr_', @addonsId, ' '), '33', 'FeiNiu数据同步插件后台', '1', @now, @now
WHERE @addonsId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSync');

UPDATE `hg_admin_menu`
SET `title` = 'FeiNiu同步',
    `pid` = @addonsId,
    `path` = 'youbanFeiniuSync',
    `icon` = 'icon-park-outline:database-sync',
    `type` = '2',
    `redirect` = '',
    `permissions` = '',
    `permission_name` = '',
    `component` = '/addons/youbanFeiniuSync/index',
    `always_show` = '1',
    `hidden` = '2',
    `level` = '2',
    `tree` = CONCAT('tr_', @addonsId, ' '),
    `sort` = '33',
    `remark` = 'FeiNiu数据同步插件后台',
    `status` = '1',
    `updated_at` = @now
WHERE @addonsId IS NOT NULL AND `name` = 'youbanFeiniuSync';

SET @syncId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSync' LIMIT 1);
SET @syncTree := (SELECT `tree` FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSync' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @syncId, '大盘权限', 'youbanFeiniuSyncDashboardPerm', '', '', '3', '', '/youban_feiniu_sync/sync/options/tenants,/youban_feiniu_sync/sync/options/adminAccounts,/youban_feiniu_sync/sync/dashboard,/youban_feiniu_sync/sync/dashboard/summary,/youban_feiniu_sync/sync/dashboard/trend,/youban_feiniu_sync/sync/dashboard/channelRank,/youban_feiniu_sync/sync/dashboard/recentRuns', '', '', '0', 'youbanFeiniuSync', '0', '0', '', '0', '1', '0', '3', CONCAT(@syncTree, 'tr_', @syncId, ' '), '10', 'FeiNiu同步大盘权限', '1', @now, @now
WHERE @syncId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSyncDashboardPerm');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @syncId, '配置权限', 'youbanFeiniuSyncConfigPerm', '', '', '3', '', '/youban_feiniu_sync/sync/config/list,/youban_feiniu_sync/sync/config/view,/youban_feiniu_sync/sync/config/save,/youban_feiniu_sync/sync/config/delete,/youban_feiniu_sync/sync/config/autoSync,/youban_feiniu_sync/sync/config/test', '', '', '0', 'youbanFeiniuSync', '0', '0', '', '0', '1', '0', '3', CONCAT(@syncTree, 'tr_', @syncId, ' '), '20', 'FeiNiu同步配置权限', '1', @now, @now
WHERE @syncId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSyncConfigPerm');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @syncId, '频道权限', 'youbanFeiniuSyncChannelPerm', '', '', '3', '', '/youban_feiniu_sync/sync/options/tenants,/youban_feiniu_sync/sync/options/accounts,/youban_feiniu_sync/sync/channel/list,/youban_feiniu_sync/sync/channel/clear,/youban_feiniu_sync/sync/channel/copy,/youban_feiniu_sync/sync/channel/disable', '', '', '0', 'youbanFeiniuSync', '0', '0', '', '0', '1', '0', '3', CONCAT(@syncTree, 'tr_', @syncId, ' '), '30', 'FeiNiu同步频道权限', '1', @now, @now
WHERE @syncId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSyncChannelPerm');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @syncId, '运行权限', 'youbanFeiniuSyncRunPerm', '', '', '3', '', '/youban_feiniu_sync/sync/run/list,/youban_feiniu_sync/sync/run/view,/youban_feiniu_sync/sync/run/items,/youban_feiniu_sync/sync/run/start', '', '', '0', 'youbanFeiniuSync', '0', '0', '', '0', '1', '0', '3', CONCAT(@syncTree, 'tr_', @syncId, ' '), '40', 'FeiNiu同步运行权限', '1', @now, @now
WHERE @syncId IS NOT NULL AND NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'youbanFeiniuSyncRunPerm');

UPDATE `hg_admin_menu`
SET `pid` = @syncId,
    `type` = '3',
    `active_menu` = 'youbanFeiniuSync',
    `hidden` = '1',
    `level` = '3',
    `tree` = CONCAT(@syncTree, 'tr_', @syncId, ' '),
    `status` = '1',
    `updated_at` = @now,
    `title` = CASE `name`
        WHEN 'youbanFeiniuSyncDashboardPerm' THEN '大盘权限'
        WHEN 'youbanFeiniuSyncConfigPerm' THEN '配置权限'
        WHEN 'youbanFeiniuSyncChannelPerm' THEN '频道权限'
        WHEN 'youbanFeiniuSyncRunPerm' THEN '运行权限'
        WHEN 'youbanFeiniuSyncManage' THEN '同步管理'
        ELSE `title`
    END,
    `permissions` = CASE `name`
        WHEN 'youbanFeiniuSyncDashboardPerm' THEN '/youban_feiniu_sync/sync/options/tenants,/youban_feiniu_sync/sync/options/adminAccounts,/youban_feiniu_sync/sync/dashboard,/youban_feiniu_sync/sync/dashboard/summary,/youban_feiniu_sync/sync/dashboard/trend,/youban_feiniu_sync/sync/dashboard/channelRank,/youban_feiniu_sync/sync/dashboard/recentRuns'
        WHEN 'youbanFeiniuSyncConfigPerm' THEN '/youban_feiniu_sync/sync/config/list,/youban_feiniu_sync/sync/config/view,/youban_feiniu_sync/sync/config/save,/youban_feiniu_sync/sync/config/delete,/youban_feiniu_sync/sync/config/autoSync,/youban_feiniu_sync/sync/config/test'
        WHEN 'youbanFeiniuSyncChannelPerm' THEN '/youban_feiniu_sync/sync/options/tenants,/youban_feiniu_sync/sync/options/accounts,/youban_feiniu_sync/sync/channel/list,/youban_feiniu_sync/sync/channel/clear,/youban_feiniu_sync/sync/channel/copy,/youban_feiniu_sync/sync/channel/disable'
        WHEN 'youbanFeiniuSyncRunPerm' THEN '/youban_feiniu_sync/sync/run/list,/youban_feiniu_sync/sync/run/view,/youban_feiniu_sync/sync/run/items,/youban_feiniu_sync/sync/run/start'
        WHEN 'youbanFeiniuSyncManage' THEN '/youban_feiniu_sync/sync/dashboard'
        ELSE `permissions`
    END,
    `sort` = CASE `name`
        WHEN 'youbanFeiniuSyncDashboardPerm' THEN '10'
        WHEN 'youbanFeiniuSyncConfigPerm' THEN '20'
        WHEN 'youbanFeiniuSyncChannelPerm' THEN '30'
        WHEN 'youbanFeiniuSyncRunPerm' THEN '40'
        WHEN 'youbanFeiniuSyncManage' THEN '1'
        ELSE `sort`
    END
WHERE @syncId IS NOT NULL AND `name` IN ('youbanFeiniuSyncDashboardPerm','youbanFeiniuSyncConfigPerm','youbanFeiniuSyncChannelPerm','youbanFeiniuSyncRunPerm','youbanFeiniuSyncManage');

INSERT INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN ('youbanFeiniuSync', 'youbanFeiniuSyncDashboardPerm', 'youbanFeiniuSyncConfigPerm', 'youbanFeiniuSyncChannelPerm', 'youbanFeiniuSyncRunPerm', 'youbanFeiniuSyncManage')
WHERE r.`id` IN (1, 2)
  AND NOT EXISTS (
    SELECT 1 FROM `hg_admin_role_menu` rm WHERE rm.`role_id` = r.`id` AND rm.`menu_id` = m.`id`
  );
