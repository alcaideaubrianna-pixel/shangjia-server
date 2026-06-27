CREATE TABLE IF NOT EXISTS `hg_content_channel` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `source_channel_id` bigint(20) NOT NULL COMMENT 'FeiNiu频道ID',
  `tg_chat_id` varchar(128) DEFAULT NULL COMMENT 'TG Chat ID',
  `title` varchar(255) DEFAULT NULL COMMENT '频道标题',
  `username` varchar(255) DEFAULT NULL COMMENT '频道用户名',
  `invite_link` varchar(512) DEFAULT NULL COMMENT '邀请链接',
  `source_type` varchar(32) NOT NULL DEFAULT 'feiniu' COMMENT '来源类型',
  `public_status` varchar(32) NOT NULL DEFAULT 'hidden' COMMENT '前台公开状态',
  `auth_status` varchar(32) NOT NULL DEFAULT 'none' COMMENT '授权状态',
  `remark` varchar(500) DEFAULT NULL COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_content_channel_source` (`source_type`, `source_channel_id`),
  KEY `idx_content_channel_status` (`status`, `public_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容_来源频道';

CREATE TABLE IF NOT EXISTS `hg_content_profile` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `profile_no` varchar(64) NOT NULL COMMENT '资料编号',
  `source_type` varchar(32) NOT NULL DEFAULT 'feiniu' COMMENT '来源类型',
  `source_note_id` bigint(20) DEFAULT NULL COMMENT 'FeiNiu笔记ID',
  `source_note_uuid` varchar(64) DEFAULT NULL COMMENT 'FeiNiu笔记UUID',
  `source_key` varchar(255) DEFAULT NULL COMMENT '来源唯一键',
  `source_text_hash` varchar(64) DEFAULT NULL COMMENT '来源文本哈希',
  `channel_id` bigint(20) DEFAULT NULL COMMENT '本地来源频道ID',
  `duplicate_of_id` bigint(20) DEFAULT NULL COMMENT '重复资料ID',
  `title` varchar(255) DEFAULT NULL COMMENT '标题',
  `summary` text COMMENT '摘要',
  `plain_text` text COMMENT '正文纯文本',
  `html_text` text COMMENT 'HTML正文',
  `source_category_code` varchar(64) DEFAULT NULL COMMENT 'FeiNiu分类编码',
  `days_with_escort` int(11) DEFAULT NULL COMMENT '陪伴天数',
  `expected_living_cost` int(11) DEFAULT NULL COMMENT '期望生活费',
  `can_fly_to_province` tinyint(1) NOT NULL DEFAULT '0' COMMENT '可飞外省',
  `can_go_abroad` tinyint(1) NOT NULL DEFAULT '0' COMMENT '可出国',
  `can_overnight` tinyint(1) NOT NULL DEFAULT '0' COMMENT '可过夜',
  `can_cohabitate` tinyint(1) NOT NULL DEFAULT '0' COMMENT '可同居',
  `has_health_check` tinyint(1) NOT NULL DEFAULT '0' COMMENT '有体检',
  `is_full_month` tinyint(1) NOT NULL DEFAULT '0' COMMENT '满月',
  `is_virgin` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否处',
  `accept_sm` tinyint(1) NOT NULL DEFAULT '0' COMMENT '接受SM',
  `no_condom_after_check` tinyint(1) NOT NULL DEFAULT '0' COMMENT '体检后无套',
  `allow_creampie` tinyint(1) NOT NULL DEFAULT '0' COMMENT '可内射',
  `has_tattoo` tinyint(1) NOT NULL DEFAULT '0' COMMENT '有纹身',
  `is_favorite` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否收藏',
  `source_edited_at` datetime DEFAULT NULL COMMENT 'FeiNiu编辑时间',
  `group_params` text COMMENT '分组参数',
  `tag_params` text COMMENT '标签参数',
  `text_block_count` int(11) NOT NULL DEFAULT '0' COMMENT '文本块数',
  `storage_policy` varchar(32) DEFAULT NULL COMMENT '存储策略',
  `source_remark` varchar(500) DEFAULT NULL COMMENT 'FeiNiu备注',
  `source_create_by` varchar(64) DEFAULT NULL COMMENT 'FeiNiu创建者',
  `source_update_by` varchar(64) DEFAULT NULL COMMENT 'FeiNiu更新者',
  `source_created_at` datetime DEFAULT NULL COMMENT 'FeiNiu创建时间',
  `source_updated_at` datetime DEFAULT NULL COMMENT 'FeiNiu更新时间',
  `province` varchar(64) DEFAULT NULL COMMENT '省份',
  `city` varchar(64) DEFAULT NULL COMMENT '城市',
  `age` int(11) DEFAULT NULL COMMENT '年龄',
  `height` int(11) DEFAULT NULL COMMENT '身高',
  `weight` int(11) DEFAULT NULL COMMENT '体重',
  `cup_size` varchar(32) DEFAULT NULL COMMENT '资料标签',
  `has_verification_video` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否有验证视频',
  `member_only_video` tinyint(1) NOT NULL DEFAULT '1' COMMENT '视频是否仅会员可见',
  `cover_media_id` bigint(20) DEFAULT NULL COMMENT '封面媒体ID',
  `image_count` int(11) NOT NULL DEFAULT '0' COMMENT '图片数',
  `video_count` int(11) NOT NULL DEFAULT '0' COMMENT '视频数',
  `visibility` varchar(32) NOT NULL DEFAULT 'private' COMMENT '可见性',
  `review_status` varchar(32) NOT NULL DEFAULT 'approved' COMMENT '审核状态',
  `import_status` varchar(32) NOT NULL DEFAULT 'imported' COMMENT '导入状态',
  `admin_remark` varchar(500) DEFAULT NULL COMMENT '后台备注',
  `home_recommend` tinyint(1) NOT NULL DEFAULT '0' COMMENT '首页推荐',
  `home_sort` int(11) NOT NULL DEFAULT '0' COMMENT '首页推荐排序',
  `published_at` datetime DEFAULT NULL COMMENT '发布时间',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_content_profile_no` (`profile_no`),
  UNIQUE KEY `uk_content_profile_source_note` (`source_type`, `source_note_id`),
  KEY `idx_content_profile_public` (`status`, `visibility`, `review_status`, `published_at`),
  KEY `idx_content_profile_city` (`province`, `city`),
  KEY `idx_content_profile_duplicate` (`duplicate_of_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容_资料';

CREATE TABLE IF NOT EXISTS `hg_sys_ip_location_cache` (
  `ip` varchar(64) NOT NULL COMMENT 'IP地址',
  `country` varchar(64) DEFAULT NULL COMMENT '国家',
  `region` varchar(128) DEFAULT NULL COMMENT '地区',
  `province` varchar(128) DEFAULT NULL COMMENT '省份',
  `province_code` bigint(20) NOT NULL DEFAULT '0' COMMENT '省份编码',
  `city` varchar(128) DEFAULT NULL COMMENT '城市',
  `city_code` bigint(20) NOT NULL DEFAULT '0' COMMENT '城市编码',
  `area` varchar(255) DEFAULT NULL COMMENT '区域',
  `area_code` bigint(20) NOT NULL DEFAULT '0' COMMENT '区域编码',
  `expires_at` datetime NOT NULL COMMENT '过期时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`ip`),
  KEY `idx_sys_ip_location_cache_expires` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统_IP归属地缓存';

CREATE TABLE IF NOT EXISTS `hg_content_media` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `profile_id` bigint(20) NOT NULL COMMENT '资料ID',
  `source_asset_id` bigint(20) DEFAULT NULL COMMENT 'FeiNiu资源ID',
  `duplicate_of_media_id` bigint(20) DEFAULT NULL COMMENT '重复媒体ID',
  `media_type` varchar(16) NOT NULL COMMENT '媒体类型',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `original_storage_path` varchar(1024) DEFAULT NULL COMMENT '原始存储路径',
  `display_storage_path` varchar(1024) DEFAULT NULL COMMENT '展示存储路径',
  `preview_storage_path` varchar(1024) DEFAULT NULL COMMENT '预览存储路径',
  `binary_md5` varchar(64) DEFAULT NULL COMMENT '文件MD5',
  `perceptual_hash` varchar(64) DEFAULT NULL COMMENT '感知哈希',
  `width` int(11) DEFAULT NULL COMMENT '宽度',
  `height` int(11) DEFAULT NULL COMMENT '高度',
  `duration` int(11) DEFAULT NULL COMMENT '时长',
  `process_status` varchar(32) NOT NULL DEFAULT 'raw' COMMENT '处理状态',
  `encrypt_status` varchar(32) NOT NULL DEFAULT 'none' COMMENT '加密状态',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_content_media_profile_asset` (`profile_id`, `source_asset_id`),
  KEY `idx_content_media_profile` (`profile_id`, `sort_index`),
  KEY `idx_content_media_duplicate` (`duplicate_of_media_id`),
  KEY `idx_content_media_md5` (`binary_md5`),
  KEY `idx_content_media_phash` (`perceptual_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容_资料媒体';

CREATE TABLE IF NOT EXISTS `hg_content_source_map` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `profile_id` bigint(20) NOT NULL COMMENT '资料ID',
  `source_type` varchar(32) NOT NULL COMMENT '来源类型',
  `source_key` varchar(255) NOT NULL COMMENT '来源唯一键',
  `source_channel_id` bigint(20) DEFAULT NULL COMMENT '来源频道ID',
  `source_message_id` bigint(20) DEFAULT NULL COMMENT '来源消息ID',
  `source_grouped_id` bigint(20) DEFAULT NULL COMMENT '来源媒体组ID',
  `source_text_hash` varchar(64) DEFAULT NULL COMMENT '来源文本哈希',
  `raw_text` text COMMENT '原始文本',
  `raw_message_json` text COMMENT '原始消息JSON',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_content_source_key` (`source_key`),
  KEY `idx_content_source_profile` (`profile_id`),
  KEY `idx_content_source_text_hash` (`source_channel_id`, `source_text_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容_来源映射';

CREATE TABLE IF NOT EXISTS `hg_content_import_checkpoint` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `source_name` varchar(64) NOT NULL COMMENT '来源名称',
  `last_source_note_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '最后导入来源笔记ID',
  `last_success_at` datetime DEFAULT NULL COMMENT '最后成功时间',
  `last_error` varchar(500) DEFAULT NULL COMMENT '最后错误',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_content_import_source` (`source_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容_导入游标';

CREATE TABLE IF NOT EXISTS `hg_content_import_run` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `source_name` varchar(64) NOT NULL COMMENT '来源名称',
  `trigger_type` varchar(32) NOT NULL DEFAULT 'manual' COMMENT '触发方式',
  `batch_size` int(11) NOT NULL DEFAULT '0' COMMENT '批量数量',
  `scanned` int(11) NOT NULL DEFAULT '0' COMMENT '扫描数量',
  `imported` int(11) NOT NULL DEFAULT '0' COMMENT '导入数量',
  `duplicate` int(11) NOT NULL DEFAULT '0' COMMENT '重复数量',
  `media_imported` int(11) NOT NULL DEFAULT '0' COMMENT '媒体导入数量',
  `last_source_note_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '最后来源笔记ID',
  `status` varchar(32) NOT NULL DEFAULT 'running' COMMENT '运行状态',
  `error_message` varchar(500) DEFAULT NULL COMMENT '错误信息',
  `started_at` datetime DEFAULT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `cost_ms` int(11) NOT NULL DEFAULT '0' COMMENT '耗时毫秒',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_content_import_run_source_time` (`source_name`, `started_at`),
  KEY `idx_content_import_run_status` (`status`, `started_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容_导入运行记录';

-- 后台笔记管理查询索引。全部幂等执行，支持快速按审核、发布、地区、来源、频道、时间筛选。
SET @schema := DATABASE();

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'admin_remark') = 0,
  'ALTER TABLE `hg_content_profile` ADD COLUMN `admin_remark` varchar(500) DEFAULT NULL COMMENT ''后台备注'' AFTER `import_status`',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'html_text') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `html_text` text COMMENT ''HTML正文'' AFTER `plain_text`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_category_code') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_category_code` varchar(64) DEFAULT NULL COMMENT ''FeiNiu分类编码'' AFTER `html_text`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'days_with_escort') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `days_with_escort` int(11) DEFAULT NULL COMMENT ''陪伴天数'' AFTER `source_category_code`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'expected_living_cost') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `expected_living_cost` int(11) DEFAULT NULL COMMENT ''期望生活费'' AFTER `days_with_escort`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'can_fly_to_province') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `can_fly_to_province` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''可飞外省'' AFTER `expected_living_cost`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'can_go_abroad') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `can_go_abroad` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''可出国'' AFTER `can_fly_to_province`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'can_overnight') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `can_overnight` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''可过夜'' AFTER `can_go_abroad`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'can_cohabitate') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `can_cohabitate` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''可同居'' AFTER `can_overnight`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'has_health_check') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `has_health_check` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''有体检'' AFTER `can_cohabitate`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'is_full_month') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `is_full_month` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''满月'' AFTER `has_health_check`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'is_virgin') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `is_virgin` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''是否处'' AFTER `is_full_month`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'accept_sm') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `accept_sm` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''接受SM'' AFTER `is_virgin`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'no_condom_after_check') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `no_condom_after_check` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''体检后无套'' AFTER `accept_sm`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'allow_creampie') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `allow_creampie` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''可内射'' AFTER `no_condom_after_check`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'has_tattoo') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `has_tattoo` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''有纹身'' AFTER `allow_creampie`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'is_favorite') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `is_favorite` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''是否收藏'' AFTER `has_tattoo`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_edited_at') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_edited_at` datetime DEFAULT NULL COMMENT ''FeiNiu编辑时间'' AFTER `is_favorite`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'group_params') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `group_params` text COMMENT ''分组参数'' AFTER `source_edited_at`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'tag_params') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `tag_params` text COMMENT ''标签参数'' AFTER `group_params`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'text_block_count') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `text_block_count` int(11) NOT NULL DEFAULT ''0'' COMMENT ''文本块数'' AFTER `tag_params`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'storage_policy') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `storage_policy` varchar(32) DEFAULT NULL COMMENT ''存储策略'' AFTER `text_block_count`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_remark') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_remark` varchar(500) DEFAULT NULL COMMENT ''FeiNiu备注'' AFTER `storage_policy`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_create_by') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_create_by` varchar(64) DEFAULT NULL COMMENT ''FeiNiu创建者'' AFTER `source_remark`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_update_by') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_update_by` varchar(64) DEFAULT NULL COMMENT ''FeiNiu更新者'' AFTER `source_create_by`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_created_at') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_created_at` datetime DEFAULT NULL COMMENT ''FeiNiu创建时间'' AFTER `source_update_by`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_updated_at') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `source_updated_at` datetime DEFAULT NULL COMMENT ''FeiNiu更新时间'' AFTER `source_created_at`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'source_attributes_json') > 0, 'ALTER TABLE `hg_content_profile` DROP COLUMN `source_attributes_json`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'home_recommend') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `home_recommend` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''首页推荐'' AFTER `admin_remark`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET @sql := IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND column_name = 'home_sort') = 0, 'ALTER TABLE `hg_content_profile` ADD COLUMN `home_sort` int(11) NOT NULL DEFAULT ''0'' COMMENT ''首页推荐排序'' AFTER `home_recommend`', 'SELECT 1'); PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

UPDATE `hg_content_profile` SET `review_status` = 'approved' WHERE `source_type` = 'feiniu' AND `review_status` = 'pending';

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_status') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_status` (`review_status`, `visibility`, `import_status`, `status`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_public_latest') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_public_latest` (`status`, `review_status`, `import_status`, `visibility`, `source_created_at`, `source_note_id`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_public_area_latest') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_public_area_latest` (`status`, `review_status`, `import_status`, `visibility`, `province`, `city`, `source_created_at`, `source_note_id`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_created') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_created` (`created_at`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_area') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_area` (`province`, `city`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_video') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_video` (`video_count`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_channel') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_channel` (`channel_id`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_age') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_age` (`age`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_height') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_height` (`height`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_weight') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_weight` (`weight`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_days') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_days` (`days_with_escort`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_cost') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_cost` (`expected_living_cost`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_admin_flags') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_admin_flags` (`can_fly_to_province`, `can_go_abroad`, `can_overnight`, `has_health_check`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'idx_content_profile_home_recommend') = 0,
  'ALTER TABLE `hg_content_profile` ADD INDEX `idx_content_profile_home_recommend` (`home_recommend`, `home_sort`, `status`, `review_status`, `visibility`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_profile' AND index_name = 'ft_content_profile_keyword') = 0,
  'ALTER TABLE `hg_content_profile` ADD FULLTEXT INDEX `ft_content_profile_keyword` (`profile_no`, `title`, `summary`, `plain_text`, `province`, `city`, `cup_size`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_channel' AND index_name = 'idx_content_channel_source_query') = 0,
  'ALTER TABLE `hg_content_channel` ADD INDEX `idx_content_channel_source_query` (`source_channel_id`, `id`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = @schema AND table_name = 'hg_content_channel' AND index_name = 'idx_content_channel_username') = 0,
  'ALTER TABLE `hg_content_channel` ADD INDEX `idx_content_channel_username` (`username`)',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
