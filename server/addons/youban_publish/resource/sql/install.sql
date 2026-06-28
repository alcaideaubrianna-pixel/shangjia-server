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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架商家';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_account` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '商家ID',
  `admin_member_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '绑定系统账号ID',
  `parent_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '父账号ID',
  `account_type` varchar(32) NOT NULL DEFAULT 'uploader' COMMENT '账号类型',
  `nickname` varchar(128) NOT NULL DEFAULT '' COMMENT '昵称',
  `username` varchar(128) NOT NULL DEFAULT '' COMMENT '用户名',
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
  KEY `idx_ybp_account_merchant` (`merchant_id`,`account_type`,`status`),
  KEY `idx_ybp_account_admin_member` (`admin_member_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架账号';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_task` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `client_request_id` varchar(128) NOT NULL DEFAULT '' COMMENT '客户端幂等ID',
  `title` varchar(255) NOT NULL DEFAULT '' COMMENT '标题',
  `province` varchar(64) NOT NULL DEFAULT '' COMMENT '省份',
  `city` varchar(64) NOT NULL DEFAULT '' COMMENT '城市',
  `plain_text` text COMMENT '正文',
  `media_count` int(11) NOT NULL DEFAULT '0' COMMENT '媒体数量',
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
  KEY `idx_ybp_task_client_request` (`merchant_id`,`client_request_id`),
  KEY `idx_ybp_task_merchant_status` (`merchant_id`,`status`,`id`),
  KEY `idx_ybp_task_account_status` (`account_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架任务';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_media` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '商家ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'HotGo附件ID',
  `media_type` varchar(16) NOT NULL DEFAULT 'image' COMMENT '媒体类型',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT '文件名',
  `file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '访问地址',
  `storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径',
  `mime_type` varchar(128) NOT NULL DEFAULT '' COMMENT 'MIME',
  `md5` varchar(64) NOT NULL DEFAULT '' COMMENT 'MD5',
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
  KEY `idx_ybp_media_profile` (`profile_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架媒体';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_job` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `task_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '任务ID',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '商家ID',
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

CREATE TABLE IF NOT EXISTS `hg_youban_publish_bot` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
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
  KEY `idx_ybp_bot_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架Bot配置';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_login` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `merchant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '商家ID',
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
SELECT 'youban_publish', 'account', '默认角色ID', 'int', 'defaultRoleId', '10', '10', 10, '创建管理员账号和上架账号时绑定的 HotGo 后台角色ID', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='defaultRoleId');

INSERT INTO `hg_sys_addons_config` (`addon_name`, `group`, `name`, `type`, `key`, `value`, `default_value`, `sort`, `tip`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'youban_publish', 'account', '默认部门ID', 'int', 'defaultDeptId', '1', '1', 20, '创建管理员账号和上架账号时绑定的 HotGo 后台部门ID', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM `hg_sys_addons_config` WHERE `addon_name`='youban_publish' AND `key`='defaultDeptId');
