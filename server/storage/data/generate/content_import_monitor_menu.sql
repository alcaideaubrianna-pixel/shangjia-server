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
SELECT NULL, @contentId, '同步监控', 'ContentImportMonitor', 'importMonitor', '', '2', '', '', '', '/content/importMonitor/index', '1', '', '0', '0', '', '0', '0', '0', '2', CONCAT('tr_', @contentId, ' '), '10', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportMonitor');

UPDATE `hg_admin_menu` SET `permissions` = '' WHERE `name` = 'ContentImportMonitor';

SET @monitorId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'ContentImportMonitor' LIMIT 1);

-- 页面：笔记管理
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @contentId, '笔记管理', 'ContentNote', 'note', '', '2', '', '', '', '/content/note/index', '1', '', '0', '0', '', '0', '0', '0', '2', CONCAT('tr_', @contentId, ' '), '20', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNote');

SET @noteId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'ContentNote' LIMIT 1);
SET @announcementId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'ContentAnnouncement' LIMIT 1);

-- 页面：公告展示
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @contentId, '公告展示', 'ContentAnnouncement', 'announcement', '', '2', '', '', '', '/content/announcement/index', '1', '', '0', '0', '', '0', '0', '0', '2', CONCAT('tr_', @contentId, ' '), '30', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentAnnouncement');

SET @announcementId := (SELECT `id` FROM `hg_admin_menu` WHERE `name` = 'ContentAnnouncement' LIMIT 1);

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @announcementId, 'APP公告列表', 'AppAnnouncementList', '', '', '3', '', '/appAnnouncement/list', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @announcementId, ' '), '10', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'AppAnnouncementList');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @announcementId, '编辑APP公告', 'AppAnnouncementEdit', '', '', '3', '', '/appAnnouncement/edit', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @announcementId, ' '), '20', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'AppAnnouncementEdit');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @announcementId, '更新APP公告状态', 'AppAnnouncementStatus', '', '', '3', '', '/appAnnouncement/status', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @announcementId, ' '), '30', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'AppAnnouncementStatus');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @announcementId, '删除APP公告', 'AppAnnouncementDelete', '', '', '3', '', '/appAnnouncement/delete', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @announcementId, ' '), '40', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'AppAnnouncementDelete');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @announcementId, 'APP公告最大排序', 'AppAnnouncementMaxSort', '', '', '3', '', '/appAnnouncement/maxSort', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @announcementId, ' '), '50', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'AppAnnouncementMaxSort');

-- 权限：笔记列表
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '笔记列表', 'ContentNoteList', '', '', '3', '', '/contentNote/list', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '10', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteList');

-- 权限：笔记详情
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '笔记详情', 'ContentNoteView', '', '', '3', '', '/contentNote/view', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '20', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteView');

-- 权限：修改笔记
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '修改笔记', 'ContentNoteEdit', '', '', '3', '', '/contentNote/edit', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '30', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteEdit');

-- 权限：修改媒体
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '修改媒体', 'ContentNoteMediaEdit', '', '', '3', '', '/contentNote/mediaEdit', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '40', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteMediaEdit');

-- 权限：批量删除笔记
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '批量删除笔记', 'ContentNoteBatchDelete', '', '', '3', '', '/contentNote/batchDelete', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '50', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteBatchDelete');

-- 权限：批量审核笔记
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '批量审核笔记', 'ContentNoteBatchReview', '', '', '3', '', '/contentNote/batchReview', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '60', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteBatchReview');

-- 权限：批量状态笔记
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '批量状态笔记', 'ContentNoteBatchStatus', '', '', '3', '', '/contentNote/batchStatus', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '70', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteBatchStatus');

-- 权限：批量备注笔记
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @noteId, '批量备注笔记', 'ContentNoteBatchRemark', '', '', '3', '', '/contentNote/batchRemark', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @noteId, ' '), '80', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentNoteBatchRemark');

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

-- 权限：自动同步开关
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @monitorId, '自动同步开关', 'ContentImportAutoSync', '', '', '3', '', '/contentImport/autoSync', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @monitorId, ' '), '40', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportAutoSync');

-- 权限：审核配置
INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @monitorId, '审核配置', 'ContentImportReviewConfig', '', '', '3', '', '/contentImport/reviewConfig', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @monitorId, ' '), '50', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportReviewConfig');

INSERT INTO `hg_admin_menu` (`id`, `pid`, `title`, `name`, `path`, `icon`, `type`, `redirect`, `permissions`, `permission_name`, `component`, `always_show`, `active_menu`, `is_root`, `is_frame`, `frame_src`, `keep_alive`, `hidden`, `affix`, `level`, `tree`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT NULL, @monitorId, '保存审核配置', 'ContentImportSaveReviewConfig', '', '', '3', '', '/contentImport/saveReviewConfig', '', '', '1', '', '0', '0', '', '0', '1', '0', '3', CONCAT('tr_', @contentId, ' tr_', @monitorId, ' '), '60', '', '1', @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_admin_menu` WHERE `name` = 'ContentImportSaveReviewConfig');

-- 自动同步任务：每 30 分钟执行一次。
INSERT INTO `hg_sys_cron` (`group_id`, `title`, `name`, `params`, `pattern`, `policy`, `count`, `sort`, `remark`, `status`, `created_at`, `updated_at`)
SELECT 1, 'FeiNiu 内容自动同步', 'content_import_feiniu', '', '0 */30 * * * *', 1, 0, 20, '每 30 分钟从 FeiNiu_bot 增量同步资料', 1, @now, @now
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_cron` WHERE `name` = 'content_import_feiniu');

-- 默认授权给超级管理员和管理员。
INSERT IGNORE INTO `hg_admin_role_menu` (`role_id`, `menu_id`)
SELECT r.`id`, m.`id`
FROM `hg_admin_role` r
JOIN `hg_admin_menu` m ON m.`name` IN ('Content', 'ContentAnnouncement', 'AppAnnouncementList', 'AppAnnouncementEdit', 'AppAnnouncementStatus', 'AppAnnouncementDelete', 'AppAnnouncementMaxSort', 'ContentNote', 'ContentNoteList', 'ContentNoteView', 'ContentNoteEdit', 'ContentNoteMediaEdit', 'ContentNoteBatchDelete', 'ContentNoteBatchReview', 'ContentNoteBatchStatus', 'ContentNoteBatchRemark', 'ContentImportMonitor', 'ContentImportOverview', 'ContentImportRunList', 'ContentImportRunFeiNiu', 'ContentImportAutoSync', 'ContentImportReviewConfig', 'ContentImportSaveReviewConfig')
WHERE r.`id` IN (1, 2);

COMMIT;
