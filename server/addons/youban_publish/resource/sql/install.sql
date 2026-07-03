CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '租户名称',
  `contact_name` varchar(128) NOT NULL DEFAULT '' COMMENT '联系人',
  `contact_phone` varchar(64) NOT NULL DEFAULT '' COMMENT '联系电话',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tenant_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架租户';
CREATE TABLE IF NOT EXISTS `hg_youban_publish_merchant` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '商家名称',
  `contact_name` varchar(128) NOT NULL DEFAULT '' COMMENT '联系人',
  `contact_phone` varchar(64) NOT NULL DEFAULT '' COMMENT '联系电话',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_merchant_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架商家兼容表';
INSERT INTO `hg_youban_publish_tenant` (`id`, `name`, `contact_name`, `contact_phone`, `remark`, `status`, `created_by`, `updated_by`, `deleted_by`, `created_at`, `updated_at`, `deleted_at`)
SELECT m.`id`, m.`name`, m.`contact_name`, m.`contact_phone`, m.`remark`, m.`status`, m.`created_by`, m.`updated_by`, m.`deleted_by`, m.`created_at`, m.`updated_at`, m.`deleted_at`
FROM `hg_youban_publish_merchant` m
WHERE NOT EXISTS (SELECT 1 FROM `hg_youban_publish_tenant` t WHERE t.`id` = m.`id`);
CREATE TABLE IF NOT EXISTS `hg_youban_publish_account` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `parent_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '父账号ID',
  `account_type` varchar(32) NOT NULL DEFAULT 'uploader' COMMENT '账号类型',
  `nickname` varchar(128) NOT NULL DEFAULT '' COMMENT '昵称',
  `username` varchar(128) NOT NULL DEFAULT '' COMMENT '用户名',
  `password_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '密码hash',
  `salt` varchar(16) NOT NULL DEFAULT '' COMMENT '密码盐',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `daily_publish_limit` int(11) NOT NULL DEFAULT '0' COMMENT '每日上架额度',
  `can_direct_publish` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否可直接发布',
  `allowed_channel_json` text COMMENT '可发布频道JSON',
  `allowed_region_json` text COMMENT '可发布地区JSON',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_account_tenant` (`tenant_id`,`account_type`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架账号';
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `password_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '密码hash' AFTER `username`, ADD COLUMN `salt` varchar(16) NOT NULL DEFAULT '' COMMENT '密码盐' AFTER `password_hash`;
UPDATE `hg_youban_publish_account` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
ALTER TABLE `hg_youban_publish_account` ADD KEY `idx_ybp_account_tenant` (`tenant_id`,`account_type`,`status`);
CREATE TABLE IF NOT EXISTS `hg_youban_publish_task` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `client_request_id` varchar(128) NOT NULL DEFAULT '' COMMENT '客户端幂等ID',
  `title` varchar(255) NOT NULL DEFAULT '' COMMENT '标题',
  `province` varchar(64) NOT NULL DEFAULT '' COMMENT '省份',
  `city` varchar(64) NOT NULL DEFAULT '' COMMENT '城市',
  `plain_text` text COMMENT '正文',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
  `channel_id_json` text COMMENT '推送频道ID JSON',
  `customer_remark` text COMMENT '客服备注',
  `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否防扫图处理',
  `tg_push_enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否推送TG',
  `tg_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT 'TG状态',
  `status` varchar(32) NOT NULL DEFAULT 'draft' COMMENT '任务状态',
  `error_message` text COMMENT '错误信息',
  `submitted_at` datetime DEFAULT NULL COMMENT '提交时间',
  `published_at` datetime DEFAULT NULL COMMENT '发布时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_task_tenant_client_request` (`tenant_id`,`client_request_id`),
  KEY `idx_ybp_task_tenant_status` (`tenant_id`,`status`,`id`),
  KEY `idx_ybp_task_account_status` (`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架任务';

ALTER TABLE `hg_youban_publish_task` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_task` ADD COLUMN `channel_id_json` text COMMENT '推送频道ID JSON' AFTER `media_count`, ADD COLUMN `customer_remark` text COMMENT '客服备注' AFTER `channel_id_json`, ADD COLUMN `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否防扫图处理' AFTER `customer_remark`;
UPDATE `hg_youban_publish_task` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_tenant_client_request` (`tenant_id`,`client_request_id`);
ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_tenant_status` (`tenant_id`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_account_setting` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `enable_suffix` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用发送后缀',
  `suffix_content` text COMMENT '发送后缀内容',
  `enable_title_mark` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否启用编号标识',
  `mark_mode` varchar(16) NOT NULL DEFAULT 'nickname' COMMENT '标识模式',
  `number_source` varchar(16) NOT NULL DEFAULT 'sequence' COMMENT '编号来源',
  `custom_mark_text` varchar(128) NOT NULL DEFAULT '' COMMENT '自定义标识文字',
  `mark_position` varchar(16) NOT NULL DEFAULT 'bottom' COMMENT '显示位置',
  `default_recycle_days` int(11) NOT NULL DEFAULT '0' COMMENT '默认循环天数',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_account_setting_account` (`tenant_id`,`account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架账号推送设置';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'HotGo附件ID',
  `media_type` varchar(16) NOT NULL DEFAULT 'image' COMMENT '媒体类型',
  `purpose` varchar(16) NOT NULL DEFAULT 'display' COMMENT '用途：display展示 verify验证',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT '文件名',
  `file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '访问地址',
  `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面',
  `poster_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面存储路径',
  `tg_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram文件ID',
  `tg_thumb_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram缩略图文件ID',
  `storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径',
  `mime_type` varchar(128) NOT NULL DEFAULT '' COMMENT 'MIME',
  `md5` varchar(64) NOT NULL DEFAULT '' COMMENT 'MD5',
  `perceptual_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希',
  `size` bigint(20) NOT NULL DEFAULT '0' COMMENT '大小',
  `sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_media_task_attachment` (`task_id`,`attachment_id`),
  KEY `idx_ybp_media_task_sort` (`task_id`,`sort_index`,`id`),
  KEY `idx_ybp_media_profile` (`profile_id`,`id`),
  KEY `idx_ybp_media_phash` (`perceptual_hash`),
  KEY `idx_ybp_media_purpose` (`task_id`,`purpose`,`sort_index`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架媒体';

ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `perceptual_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希' AFTER `md5`, ADD COLUMN `purpose` varchar(16) NOT NULL DEFAULT 'display' COMMENT '用途：display展示 verify验证' AFTER `media_type`, ADD COLUMN `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面' AFTER `file_url`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `poster_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面存储路径' AFTER `poster_url`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tg_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram文件ID' AFTER `poster_url`, ADD COLUMN `tg_thumb_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'Telegram缩略图文件ID' AFTER `tg_file_id`;
UPDATE `hg_youban_publish_media` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media_face` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体ID',
  `face_index` int(11) NOT NULL DEFAULT '0' COMMENT '图片内人脸序号',
  `bbox_json` text NOT NULL COMMENT '人脸框JSON',
  `embedding_model` varchar(64) NOT NULL DEFAULT '' COMMENT '特征模型',
  `embedding_vector` longtext NOT NULL COMMENT '人脸向量',
  `feature_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '特征哈希',
  `quality_score` decimal(10,4) NOT NULL DEFAULT '0.0000' COMMENT '质量分',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_media_face_media` (`media_id`,`face_index`),
  KEY `idx_ybp_media_face_profile` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_media_face_feature` (`feature_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架媒体人脸特征';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_anti_scan_cache` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `image_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希',
  `config_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '防扫图配置哈希',
  `provider` varchar(32) NOT NULL DEFAULT '' COMMENT '视觉服务商',
  `face_count` int(11) NOT NULL DEFAULT '0' COMMENT '人脸数量',
  `face_json` longtext NOT NULL COMMENT '人脸检测响应',
  `segment_json` longtext NOT NULL COMMENT '人像分割响应',
  `original_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '原图地址',
  `preview_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '预览图地址',
  `warnings_json` text NOT NULL COMMENT '预览告警',
  `cloud_raw_saved` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否已保存云识别结果',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_anti_scan_image` (`image_hash`),
  KEY `idx_ybp_anti_scan_config` (`image_hash`,`config_hash`),
  KEY `idx_ybp_anti_scan_provider` (`provider`,`cloud_raw_saved`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴防扫图预览缓存';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tag` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name` varchar(64) NOT NULL DEFAULT '' COMMENT '标签名称',
  `review_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '审核状态',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `use_count` int(11) NOT NULL DEFAULT '0' COMMENT '使用数量',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tag_name_deleted` (`name`,`deleted_at`),
  KEY `idx_ybp_tag_review_status` (`review_status`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架标签';

ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `name` varchar(64) NOT NULL DEFAULT '' COMMENT '标签名称' AFTER `id`, ADD COLUMN `review_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '审核状态' AFTER `name`, ADD COLUMN `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态' AFTER `review_status`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `use_count` int(11) NOT NULL DEFAULT '0' COMMENT '使用数量' AFTER `status`, ADD COLUMN `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人' AFTER `use_count`, ADD COLUMN `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人' AFTER `created_by`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人' AFTER `updated_by`, ADD COLUMN `created_at` datetime DEFAULT NULL COMMENT '创建时间' AFTER `deleted_by`, ADD COLUMN `updated_at` datetime DEFAULT NULL COMMENT '更新时间' AFTER `created_at`, ADD COLUMN `deleted_at` datetime DEFAULT NULL COMMENT '删除时间' AFTER `updated_at`;
ALTER TABLE `hg_youban_publish_tag` ADD UNIQUE KEY `uk_ybp_tag_name_deleted` (`name`,`deleted_at`);
ALTER TABLE `hg_youban_publish_tag` ADD KEY `idx_ybp_tag_review_status` (`review_status`,`status`,`id`);

INSERT INTO `hg_youban_publish_tag` (`name`, `review_status`, `status`, `use_count`, `created_by`, `updated_by`, `deleted_by`, `created_at`, `updated_at`, `deleted_at`)
SELECT seed.`name`, 'approved', 1, 0, 0, 0, 0, NOW(), NOW(), NULL
FROM (
  SELECT '颜值' AS `name` UNION ALL SELECT '穿搭' UNION ALL SELECT '美食' UNION ALL SELECT '探店' UNION ALL SELECT '旅行' UNION ALL
  SELECT '运动' UNION ALL SELECT '健身' UNION ALL SELECT '摄影' UNION ALL SELECT '音乐' UNION ALL SELECT '舞蹈' UNION ALL
  SELECT '日常' UNION ALL SELECT '生活' UNION ALL SELECT '情感' UNION ALL SELECT '职场' UNION ALL SELECT '学习' UNION ALL
  SELECT '数码' UNION ALL SELECT '游戏' UNION ALL SELECT '电影' UNION ALL SELECT '宠物' UNION ALL SELECT '家居'
) seed
WHERE NOT EXISTS (SELECT 1 FROM `hg_youban_publish_tag`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_job` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID', `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID', `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID', `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID',
  `asynq_task_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Asynq任务ID', `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试次数', `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间',
  `sent_at` datetime DEFAULT NULL COMMENT '发送成功时间', `cycle_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架',
  `cycle_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环天数', `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环发布时间',
  `next_cycle_at` datetime DEFAULT NULL COMMENT '下次循环时间', `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间', `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_job_task_channel` (`task_id`,`channel_id`),
  KEY `idx_ybp_tg_job_status_retry` (`status`,`next_retry_at`,`id`),
  KEY `idx_ybp_tg_job_task` (`task_id`),
  KEY `idx_ybp_tg_job_cycle` (`cycle_enabled`,`next_cycle_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG发布任务';

ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `task_id`, ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID' AFTER `profile_id`, ADD COLUMN `asynq_task_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Asynq任务ID' AFTER `tg_message_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `sent_at` datetime DEFAULT NULL COMMENT '发送成功时间' AFTER `next_retry_at`, ADD COLUMN `cycle_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架' AFTER `sent_at`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `cycle_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环天数' AFTER `cycle_enabled`, ADD COLUMN `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环发布时间' AFTER `cycle_days`, ADD COLUMN `next_cycle_at` datetime DEFAULT NULL COMMENT '下次循环时间' AFTER `cycle_publish_time`;
UPDATE `hg_youban_publish_tg_job` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
ALTER TABLE `hg_youban_publish_tg_job` ADD UNIQUE KEY `uk_ybp_tg_job_task_channel` (`task_id`,`channel_id`);
ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_cycle` (`cycle_enabled`,`next_cycle_at`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_message` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `job_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG任务ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID', `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID', `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID', `media_group_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `media_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '媒体ID', `purpose` varchar(16) NOT NULL DEFAULT '' COMMENT 'display/verify',
  `tg_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT 'TG文件ID', `status` varchar(32) NOT NULL DEFAULT 'sent' COMMENT '状态',
  `sent_at` datetime DEFAULT NULL COMMENT '发送时间', `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间', `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_message_job` (`job_id`,`status`,`id`),
  KEY `idx_ybp_tg_message_task` (`task_id`,`id`),
  KEY `idx_ybp_tg_message_profile` (`tenant_id`,`account_id`,`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG消息记录';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_job_log` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `job_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG任务ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID', `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID', `action` varchar(32) NOT NULL DEFAULT '' COMMENT '动作',
  `status` varchar(32) NOT NULL DEFAULT '' COMMENT '状态', `message` text COMMENT '日志内容',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_job_log_job` (`job_id`,`id`),
  KEY `idx_ybp_tg_job_log_task` (`task_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG任务日志';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_daily_stat` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键', `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID', `stat_date` date NOT NULL COMMENT '统计日期',
  `new_profile_count` int(11) NOT NULL DEFAULT '0' COMMENT '新资料数量', `published_count` int(11) NOT NULL DEFAULT '0' COMMENT '上架成功数量',
  `failed_count` int(11) NOT NULL DEFAULT '0' COMMENT '失败数量', `down_count` int(11) NOT NULL DEFAULT '0' COMMENT '下架数量',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间', `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_daily_stat_account_date` (`tenant_id`,`account_id`,`stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴每日上架统计';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_bot` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID，0表示全局默认',
  `bot_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot名称',
  `bot_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot用户名',
  `bot_token` varchar(255) NOT NULL DEFAULT '' COMMENT 'Bot Token',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_bot_tenant` (`tenant_id`,`status`,`id`),
  KEY `idx_ybp_bot_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架Bot配置';

ALTER TABLE `hg_youban_publish_bot` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID，0表示全局默认' AFTER `id`;
ALTER TABLE `hg_youban_publish_bot` ADD KEY `idx_ybp_bot_tenant` (`tenant_id`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_login` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `login_token` varchar(128) NOT NULL DEFAULT '' COMMENT '登录令牌',
  `qr_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '二维码地址',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `session_key` varchar(255) NOT NULL DEFAULT '' COMMENT '会话存储键',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `expires_at` datetime DEFAULT NULL COMMENT '过期时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_login_token` (`login_token`),
  KEY `idx_ybp_tg_login_account` (`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG扫码登录';

ALTER TABLE `hg_youban_publish_tg_login` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`;
ALTER TABLE `hg_youban_publish_tg_login` ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
UPDATE `hg_youban_publish_tg_login` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_account` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建账号ID',
  `display_name` varchar(128) NOT NULL DEFAULT '' COMMENT '显示名称',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名',
  `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓',
  `telegram_phone` varchar(64) NOT NULL DEFAULT '' COMMENT 'TG手机号',
  `telegram_is_bot` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否Bot',
  `session_key` varchar(255) NOT NULL DEFAULT '' COMMENT '会话存储键',
  `login_token` varchar(128) NOT NULL DEFAULT '' COMMENT '登录令牌',
  `qr_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '二维码地址',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `error_message` text COMMENT '错误信息',
  `last_login_at` datetime DEFAULT NULL COMMENT '最后授权时间',
  `expires_at` datetime DEFAULT NULL COMMENT '二维码过期时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_account_tenant` (`tenant_id`,`status`,`id`),
  KEY `idx_ybp_tg_account_login` (`login_token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG账号';

ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名' AFTER `telegram_username`;
ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓' AFTER `telegram_first_name`;
ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_phone` varchar(64) NOT NULL DEFAULT '' COMMENT 'TG手机号' AFTER `telegram_last_name`;
ALTER TABLE `hg_youban_publish_tg_account` ADD COLUMN `telegram_is_bot` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否Bot' AFTER `telegram_phone`;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `channel_title` varchar(128) NOT NULL DEFAULT '' COMMENT '频道名称',
  `channel_username` varchar(128) NOT NULL DEFAULT '' COMMENT '频道用户名',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `publish_direction` varchar(16) NOT NULL DEFAULT 'up' COMMENT '上架/下架频道',
  `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架',
  `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数',
  `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间',
  `is_default_selected` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否默认选中',
  `bot_id_json` text COMMENT '绑定Bot ID JSON',
  `remark` varchar(500) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `last_refresh_status` varchar(32) NOT NULL DEFAULT '' COMMENT '最近刷新状态',
  `last_refresh_message` varchar(500) NOT NULL DEFAULT '' COMMENT '最近刷新信息',
  `last_refresh_at` datetime DEFAULT NULL COMMENT '最近刷新时间',
  `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人',
  `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人',
  `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_channel_tenant_account` (`tenant_id`,`tg_account_id`,`status`,`id`),
  KEY `idx_ybp_channel_tenant_direction` (`tenant_id`,`publish_direction`,`status`,`id`),
  KEY `idx_ybp_channel_chat` (`tenant_id`,`target_chat_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架频道配置';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架' AFTER `publish_direction`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数' AFTER `cycle_publish_enabled`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间' AFTER `cycle_publish_days`;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `is_default_selected` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否默认选中' AFTER `cycle_publish_time`;
ALTER TABLE `hg_youban_publish_channel` ADD KEY `idx_ybp_channel_tenant_direction` (`tenant_id`,`publish_direction`,`status`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG账号ID',
  `channel_id` varchar(128) NOT NULL DEFAULT '' COMMENT '频道ID',
  `access_hash` varchar(128) NOT NULL DEFAULT '' COMMENT 'AccessHash',
  `channel_title` varchar(128) NOT NULL DEFAULT '' COMMENT '频道名称',
  `channel_username` varchar(128) NOT NULL DEFAULT '' COMMENT '频道用户名',
  `is_broadcast` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否频道',
  `is_megagroup` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否群组',
  `can_post_messages` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号可发频道消息',
  `can_invite_users` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号可邀请用户',
  `can_add_admins` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号可添加管理员',
  `last_sync_at` datetime DEFAULT NULL COMMENT '最后同步时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tg_channel_account_channel` (`tenant_id`,`tg_account_id`,`channel_id`),
  KEY `idx_ybp_tg_channel_search` (`tenant_id`,`tg_account_id`,`channel_title`,`channel_username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架TG账号频道缓存';

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Telegram App ID', 'int', 'appId', '0', '0', 10, '扫码登录使用的 Telegram API ID，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='appId');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Telegram App Hash', 'string', 'appHash', '', '', 20, '扫码登录使用的 Telegram App Hash，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='appHash');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', '代理地址', 'string', 'proxyUrl', '', '', 30, '本地开发可配置 socks5://127.0.0.1:7890，也支持 http/https 代理', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='proxyUrl');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Bot运行模式', 'string', 'botRuntimeMode', 'auto', 'auto', 40, 'auto/develop 使用 pull，production 使用 webhook；也可显式配置 pull/webhook', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='botRuntimeMode');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Webhook Base URL', 'string', 'webhookBaseUrl', '', '', 50, '线上 webhook 的公网域名，例如 https://api.example.com', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='webhookBaseUrl');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', 'Webhook Secret', 'string', 'webhookSecret', '', '', 60, 'Telegram webhook secret token，可选', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='webhookSecret');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'telegram', '默认推送 Chat ID', 'string', 'defaultTargetChat', '', '', 70, '资料发布后默认推送的 Telegram chat_id，可由后续频道配置覆盖', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='defaultTargetChat');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT seed.`addon_name`, seed.`group`, seed.`name`, seed.`type`, seed.`key`, seed.`value`, seed.`default_value`, seed.`sort`, seed.`tip`, 0, 1, NOW(), NOW()
FROM (
  SELECT 'youban_publish' AS `addon_name`, 'cloudResource' AS `group`, '腾讯云视觉开关' AS `name`, 'int' AS `type`, 'tencentVisionEnabled' AS `key`, '0' AS `value`, '0' AS `default_value`, 80 AS `sort`, '是否启用腾讯云人脸检测和人像分割' AS `tip` UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 SecretId', 'string', 'tencentSecretId', '', '', 90, 'CAM 子用户 SecretId，仅授予 BDA/IAI 必要权限' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 SecretKey', 'string', 'tencentSecretKey', '', '', 100, 'CAM 子用户 SecretKey，页面回显会脱敏' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 Region', 'string', 'tencentRegion', 'ap-guangzhou', 'ap-guangzhou', 110, '腾讯云接口地域，默认 ap-guangzhou' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 BDA Endpoint', 'string', 'tencentBdaEndpoint', 'bda.tencentcloudapi.com', 'bda.tencentcloudapi.com', 120, '人体分析接口域名' UNION ALL
  SELECT 'youban_publish', 'cloudResource', '腾讯云 IAI Endpoint', 'string', 'tencentIaiEndpoint', 'iai.tencentcloudapi.com', 'iai.tencentcloudapi.com', 130, '人脸识别接口域名'
) seed
WHERE NOT EXISTS (
  SELECT 1 FROM `hg_sys_addons_config` c
  WHERE c.`addon_name` = seed.`addon_name`
    AND c.`group` = seed.`group`
    AND c.`key` = seed.`key`
);

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT seed.`addon_name`, seed.`group`, seed.`name`, seed.`type`, seed.`key`, seed.`value`, seed.`default_value`, seed.`sort`, seed.`tip`, 0, 1, NOW(), NOW()
FROM (
  SELECT 'youban_publish' AS `addon_name`, 'publish' AS `group`, '循环上架开关' AS `name`, 'int' AS `type`, 'cyclePublishEnabled' AS `key`, '0' AS `value`, '0' AS `default_value`, 100 AS `sort`, '是否启用全局循环上架' AS `tip` UNION ALL
  SELECT 'youban_publish', 'publish', '循环上架天数', 'int', 'cyclePublishDays', '4', '4', 110, '循环上架间隔天数' UNION ALL
  SELECT 'youban_publish', 'publish', '循环上架时间', 'string', 'cyclePublishTime', '09:00', '09:00', 120, '每天循环上架执行时间' UNION ALL
  SELECT 'youban_publish', 'publish', '下架不推送到下架频道', 'int', 'skipDownChannelEnabled', '1', '1', 130, '资料下架时是否跳过下架频道推送' UNION ALL
  SELECT 'youban_publish', 'publish', '发送间隔秒数', 'int', 'sendIntervalSeconds', '3', '3', 140, 'Telegram 消息发送间隔' UNION ALL
  SELECT 'youban_publish', 'publish', '发送时间窗口开关', 'int', 'sendWindowEnabled', '0', '0', 150, '是否限制自动发送执行时间段' UNION ALL
  SELECT 'youban_publish', 'publish', '发送开始时间', 'string', 'sendWindowStart', '', '', 160, '自动发送允许开始时间' UNION ALL
  SELECT 'youban_publish', 'publish', '发送结束时间', 'string', 'sendWindowEnd', '', '', 170, '自动发送允许结束时间' UNION ALL
  SELECT 'youban_publish', 'publish', '失败处理策略', 'string', 'failureStrategy', 'continue', 'continue', 180, 'continue 继续后续任务，stop 停止后续任务' UNION ALL
  SELECT 'youban_publish', 'publish', '失败重试开关', 'int', 'retryEnabled', '1', '1', 190, '发送失败后是否重试' UNION ALL
  SELECT 'youban_publish', 'publish', '最大重试次数', 'int', 'maxRetryCount', '3', '3', 200, '发送失败最大重试次数' UNION ALL
  SELECT 'youban_publish', 'publish', '重试间隔分钟', 'int', 'retryIntervalMinutes', '5', '5', 210, '发送失败重试间隔' UNION ALL
  SELECT 'youban_publish', 'publish', '默认防扫图开关', 'int', 'defaultAntiScanEnabled', '0', '0', 220, '新发布内容默认是否启用防扫图' UNION ALL
  SELECT 'youban_publish', 'autoDelete', '频道自动删除开关', 'int', 'autoDeleteEnabled', '0', '0', 200, '是否启用频道自动删除' UNION ALL
  SELECT 'youban_publish', 'autoDelete', '自动删除 Bot ID', '[]int64', 'botIds', '[]', '[]', 210, '执行自动删除的 Bot ID 列表' UNION ALL
  SELECT 'youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '[]', '[]', 220, '命中后自动删除的关键词列表' UNION ALL
  SELECT 'youban_publish', 'antiScan', '防扫图总开关', 'int', 'antiScanEnabled', '0', '0', 300, '是否启用防扫图能力' UNION ALL
  SELECT 'youban_publish', 'antiScan', '新笔记默认防扫图', 'int', 'defaultNewNoteEnabled', '0', '0', 310, '新笔记默认是否开启防扫图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '已有资料批量处理', 'int', 'existingBatchEnabled', '0', '0', 320, '是否对已有资料触发批量处理意图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '发送前强制处理', 'int', 'forceBeforeSendEnabled', '0', '0', 330, '发送前是否强制生成防扫图副本' UNION ALL
  SELECT 'youban_publish', 'antiScan', '单条资料允许覆盖', 'int', 'allowSingleOverrideEnabled', '0', '0', 340, '单条资料是否允许覆盖全局开关' UNION ALL
  SELECT 'youban_publish', 'antiScan', '移除图片元信息', 'int', 'metadataStripEnabled', '0', '0', 350, '是否移除 EXIF 等图片元信息' UNION ALL
  SELECT 'youban_publish', 'antiScan', '尺寸微调', 'int', 'resizeEnabled', '0', '0', 360, '是否轻微调整图片尺寸' UNION ALL
  SELECT 'youban_publish', 'antiScan', '尺寸缩放比例', 'int', 'resizeScale', '96', '96', 370, '尺寸缩放比例，80-100' UNION ALL
  SELECT 'youban_publish', 'antiScan', '轻微裁剪', 'int', 'cropEnabled', '0', '0', 380, '是否轻微裁剪图片边缘' UNION ALL
  SELECT 'youban_publish', 'antiScan', '裁剪比例', 'int', 'cropPercent', '2', '2', 390, '边缘裁剪比例，1-8' UNION ALL
  SELECT 'youban_publish', 'antiScan', '人像背景贴图', 'int', 'portraitBackgroundEnabled', '0', '0', 410, '是否启用人像背景贴图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '人像背景替换', 'int', 'backgroundReplaceEnabled', '0', '0', 420, '是否启用替换背景处理' UNION ALL
  SELECT 'youban_publish', 'antiScan', '背景模糊', 'int', 'backgroundBlurEnabled', '0', '0', 430, '是否模糊背景' UNION ALL
  SELECT 'youban_publish', 'antiScan', '背景纹理叠加', 'int', 'backgroundTextureEnabled', '0', '0', 440, '是否叠加背景纹理' UNION ALL
  SELECT 'youban_publish', 'antiScan', '内容遮挡', 'int', 'maskEnabled', '0', '0', 450, '是否启用二维码或贴图遮挡' UNION ALL
  SELECT 'youban_publish', 'antiScan', '打码方式', 'string', 'maskMode', 'qr', 'qr', 460, '打码方式：qr二维码模式，sticker贴图模式' UNION ALL
  SELECT 'youban_publish', 'antiScan', '打码数量', 'int', 'maskCount', '1', '1', 470, '同一张图打码数量，最多3个' UNION ALL
  SELECT 'youban_publish', 'antiScan', '二维码文案', 'string', 'qrText', '', '', 480, '二维码模式的展示文案' UNION ALL
  SELECT 'youban_publish', 'antiScan', '贴图图片', 'string', 'stickerImage', '', '', 490, '贴图模式使用的正方形贴图' UNION ALL
  SELECT 'youban_publish', 'antiScan', '贴图透明度', 'int', 'stickerOpacity', '18', '18', 500, '防扫图贴图透明度，1-100' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印开关', 'int', 'watermarkEnabled', '0', '0', 510, '是否启用背景水印' UNION ALL
  SELECT 'youban_publish', 'antiScan', '资料编号水印', 'int', 'profileNoWatermarkEnabled', '0', '0', 520, '是否叠加资料编号水印' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印字体大小', 'int', 'watermarkFontSize', '22', '22', 530, '水印字体大小，12-56' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印透明度', 'int', 'watermarkOpacity', '28', '28', 540, '水印透明度，5-80' UNION ALL
  SELECT 'youban_publish', 'antiScan', '水印文案', 'string', 'watermarkText', '', '', 550, '防扫图水印文案' UNION ALL
  SELECT 'youban_publish', 'antiScan', '贴纸文案', 'string', 'stickerText', '', '', 560, '防扫图贴纸文案' UNION ALL
  SELECT 'youban_publish', 'antiScan', '噪点扰动', 'int', 'noiseEnabled', '0', '0', 570, '是否启用轻微噪点扰动' UNION ALL
  SELECT 'youban_publish', 'antiScan', '噪点强度', 'int', 'noiseStrength', '18', '18', 580, '噪点扰动强度' UNION ALL
  SELECT 'youban_publish', 'antiScan', '压缩重采样', 'int', 'compressionEnabled', '0', '0', 590, '是否启用压缩重采样' UNION ALL
  SELECT 'youban_publish', 'antiScan', '输出质量', 'int', 'compressionQuality', '82', '82', 600, '压缩重采样输出质量' UNION ALL
  SELECT 'youban_publish', 'antiScan', 'JPEG质量控制', 'int', 'jpegQualityControlEnabled', '0', '0', 610, '是否启用 JPEG 质量控制' UNION ALL
  SELECT 'youban_publish', 'antiScan', '色彩轻扰动', 'int', 'colorJitterEnabled', '0', '0', 620, '是否启用色彩轻扰动' UNION ALL
  SELECT 'youban_publish', 'antiScan', '色彩扰动强度', 'int', 'colorJitterStrength', '12', '12', 630, '色彩扰动强度' UNION ALL
  SELECT 'youban_publish', 'antiScan', '锐化模糊微扰', 'int', 'sharpenBlurEnabled', '0', '0', 640, '是否启用锐化或模糊微扰' UNION ALL
  SELECT 'youban_publish', 'antiScan', '微扰方式', 'string', 'sharpenBlurMode', 'blur', 'blur', 650, 'blur 或 sharpen' UNION ALL
  SELECT 'youban_publish', 'antiScan', '微扰强度', 'int', 'sharpenBlurStrength', '8', '8', 660, '锐化模糊微扰强度'
) seed
WHERE NOT EXISTS (
  SELECT 1 FROM `hg_sys_addons_config` c
  WHERE c.`addon_name` = seed.`addon_name`
    AND c.`group` = seed.`group`
    AND c.`key` = seed.`key`
);

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'account'
  AND `key` IN ('defaultRoleId', 'defaultDeptId');

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'antiScan'
  AND `key` IN ('enabled', 'mode', 'method', 'stickerStyle', 'stickerDensity', 'qrMaskEnabled', 'qrCustomStickerEnabled', 'qrCount', 'customStickerEnabled');

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'autoDelete'
  AND `key` = 'enabled';
