-- 内容同步监控菜单权限SQL
-- 可重复执行：已存在同名菜单时不会重复插入。

SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
SET AUTOCOMMIT = 0;
START TRANSACTION;

SET @now := now();

-- 顶级目录：内容管理
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, '0', '内容管理', 'Content', '/content', 'AppstoreOutlined', '1', '/content/importMonitor', '', '', 'LAYOUT', '1', '', '0', '0', '', '0', '0', '0', '1', '', '90', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'Content');

SET @contentId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'Content' LIMIT 1);

-- 页面：同步监控
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @contentId, '同步监控', 'ContentImportMonitor', 'importMonitor', '', '2', '', '/contentImport/runList', '', '/content/importMonitor/index', '1', '', '0', '0', '', '0', '0', '0', '2', CONCAT('tr_', @contentId, ' '), '10', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportMonitor');

SET @monitorId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'ContentImportMonitor' LIMIT 1);

-- 权限：同步概览
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @monitorId, '同步概览', 'ContentImportOverview', '', '', '3', '', '/contentImport/overview', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @monitorId, ' '), '10', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportOverview');

-- 权限：同步记录
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @monitorId, '同步记录', 'ContentImportRunList', '', '', '3', '', '/contentImport/runList', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @monitorId, ' '), '20', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportRunList');

-- 权限：手动同步
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @monitorId, '手动同步', 'ContentImportRunFeiNiu', '', '', '3', '', '/contentImport/runFeiNiu', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @monitorId, ' '), '30', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportRunFeiNiu');

-- 默认授权给超级管理员和管理员。
INSERT IGNORE INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN ('Content', 'ContentImportMonitor', 'ContentImportOverview', 'ContentImportRunList', 'ContentImportRunFeiNiu')
WHERE r.`id` IN (1, 2);

COMMIT;
