ALTER TABLE `hg_app_announcement`
  ADD COLUMN `category_code` varchar(64) NOT NULL DEFAULT 'blog' COMMENT '文章分类编码' AFTER `content`,
  ADD COLUMN `category_name` varchar(64) NOT NULL DEFAULT '博客' COMMENT '文章分类名称' AFTER `category_code`,
  ADD COLUMN `summary` varchar(500) DEFAULT NULL COMMENT '摘要' AFTER `category_name`;

ALTER TABLE `hg_app_announcement`
  ADD KEY `idx_app_announcement_category` (`category_code`, `status`, `publish_at`, `sort`, `id`);
