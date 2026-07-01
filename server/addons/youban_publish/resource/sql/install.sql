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

ALTER TABLE `hg_youban_publish_account` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`;
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `password_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '密码hash' AFTER `username`;
ALTER TABLE `hg_youban_publish_account` ADD COLUMN `salt` varchar(16) NOT NULL DEFAULT '' COMMENT '密码盐' AFTER `password_hash`;
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

ALTER TABLE `hg_youban_publish_task` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`;
ALTER TABLE `hg_youban_publish_task` ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_task` ADD COLUMN `channel_id_json` text COMMENT '推送频道ID JSON' AFTER `media_count`;
UPDATE `hg_youban_publish_task` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;
ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_tenant_client_request` (`tenant_id`,`client_request_id`);
ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_tenant_status` (`tenant_id`,`status`,`id`);

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

ALTER TABLE `hg_youban_publish_media` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `perceptual_hash` varchar(64) NOT NULL DEFAULT '' COMMENT '图片感知哈希' AFTER `md5`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `purpose` varchar(16) NOT NULL DEFAULT 'display' COMMENT '用途：display展示 verify验证' AFTER `media_type`;
ALTER TABLE `hg_youban_publish_media` ADD COLUMN `poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '视频封面' AFTER `file_url`;
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

ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `name` varchar(64) NOT NULL DEFAULT '' COMMENT '标签名称' AFTER `id`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `review_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '审核状态' AFTER `name`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态' AFTER `review_status`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `use_count` int(11) NOT NULL DEFAULT '0' COMMENT '使用数量' AFTER `status`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `created_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建人' AFTER `use_count`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `updated_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '更新人' AFTER `created_by`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `deleted_by` bigint(20) NOT NULL DEFAULT '0' COMMENT '删除人' AFTER `updated_by`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `created_at` datetime DEFAULT NULL COMMENT '创建时间' AFTER `deleted_by`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `updated_at` datetime DEFAULT NULL COMMENT '更新时间' AFTER `created_at`;
ALTER TABLE `hg_youban_publish_tag` ADD COLUMN `deleted_at` datetime DEFAULT NULL COMMENT '删除时间' AFTER `updated_at`;
ALTER TABLE `hg_youban_publish_tag` ADD UNIQUE KEY `uk_ybp_tag_name_deleted` (`name`,`deleted_at`);
ALTER TABLE `hg_youban_publish_tag` ADD KEY `idx_ybp_tag_review_status` (`review_status`,`status`,`id`);

INSERT INTO `hg_youban_publish_tag` (`name`, `review_status`, `status`, `use_count`, `created_by`, `updated_by`, `deleted_by`, `created_at`, `updated_at`, `deleted_at`)
SELECT seed.`name`, 'approved', 1, 0, 0, 0, 0, NOW(), NOW(), NULL
FROM (
  SELECT '颜值' AS `name` UNION ALL
  SELECT '穿搭' UNION ALL
  SELECT '美食' UNION ALL
  SELECT '探店' UNION ALL
  SELECT '旅行' UNION ALL
  SELECT '运动' UNION ALL
  SELECT '健身' UNION ALL
  SELECT '摄影' UNION ALL
  SELECT '音乐' UNION ALL
  SELECT '舞蹈' UNION ALL
  SELECT '日常' UNION ALL
  SELECT '生活' UNION ALL
  SELECT '情感' UNION ALL
  SELECT '职场' UNION ALL
  SELECT '学习' UNION ALL
  SELECT '数码' UNION ALL
  SELECT '游戏' UNION ALL
  SELECT '电影' UNION ALL
  SELECT '宠物' UNION ALL
  SELECT '家居'
) seed
WHERE NOT EXISTS (
  SELECT 1 FROM `hg_youban_publish_tag`
);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_job` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `target_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '目标Chat ID',
  `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID',
  `status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试次数',
  `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间',
  `error_message` text COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tg_job_status_retry` (`status`,`next_retry_at`,`id`),
  KEY `idx_ybp_tg_job_task` (`task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴TG发布任务';

ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID' AFTER `task_id`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '兼容旧版本商家ID' AFTER `tenant_id`;
UPDATE `hg_youban_publish_tg_job` SET `tenant_id` = `merchant_id` WHERE `tenant_id` = 0 AND `merchant_id` > 0;

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

UPDATE `hg_sys_addons_config`
SET `status` = 2,
    `updated_at` = NOW()
WHERE `addon_name` = 'youban_publish'
  AND `group` = 'account'
  AND `key` IN ('defaultRoleId', 'defaultDeptId');
