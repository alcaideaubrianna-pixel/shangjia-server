-- 公告移动端展示扩展字段。
-- 可重复执行：字段和索引存在时不会重复创建。

SET @schema := DATABASE();

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND column_name = 'is_banner') = 0,
  'ALTER TABLE `hg_admin_notice` ADD COLUMN `is_banner` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''是否Banner'' AFTER `receiver`',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND column_name = 'banner_img') = 0,
  'ALTER TABLE `hg_admin_notice` ADD COLUMN `banner_img` varchar(500) DEFAULT NULL COMMENT ''Banner图片'' AFTER `is_banner`',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND column_name = 'banner_url') = 0,
  'ALTER TABLE `hg_admin_notice` ADD COLUMN `banner_url` varchar(500) DEFAULT NULL COMMENT ''Banner链接'' AFTER `banner_img`',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND column_name = 'publish_at') = 0,
  'ALTER TABLE `hg_admin_notice` ADD COLUMN `publish_at` datetime DEFAULT NULL COMMENT ''定时发布时间'' AFTER `banner_url`',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND column_name = 'expire_at') = 0,
  'ALTER TABLE `hg_admin_notice` ADD COLUMN `expire_at` datetime DEFAULT NULL COMMENT ''过期时间'' AFTER `publish_at`',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND index_name = 'idx_admin_notice_public') = 0,
  'ALTER TABLE `hg_admin_notice` ADD INDEX `idx_admin_notice_public` (`type`, `status`, `publish_at`, `expire_at`, `sort`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_admin_notice' AND index_name = 'idx_admin_notice_banner') = 0,
  'ALTER TABLE `hg_admin_notice` ADD INDEX `idx_admin_notice_banner` (`is_banner`, `status`, `publish_at`, `expire_at`, `sort`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
