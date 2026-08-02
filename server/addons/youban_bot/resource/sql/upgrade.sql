ALTER TABLE `hg_youban_bot_bot` ADD COLUMN `is_official` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否官方Bot' AFTER `bot_token`;
ALTER TABLE `hg_youban_bot_bot` ADD COLUMN `is_default` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否默认官方Bot' AFTER `is_official`;
CREATE TABLE IF NOT EXISTS `hg_youban_bot_user` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名',
  `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓',
  `chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Chat ID',
  `chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT 'Chat类型',
  `chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT 'Chat标题',
  `message_count` int(11) NOT NULL DEFAULT '0' COMMENT '消息数',
  `last_message_text` text COMMENT '最后消息',
  `last_message_at` datetime DEFAULT NULL COMMENT '最后消息时间',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybbu_bot_user` (`bot_id`,`telegram_user_id`),
  KEY `idx_ybbu_user` (`telegram_user_id`),
  KEY `idx_ybbu_last` (`bot_id`,`last_message_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴全局Bot用户';
CREATE TABLE IF NOT EXISTS `hg_youban_bot_message` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Chat ID',
  `chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT 'Chat类型',
  `message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID',
  `message_type` varchar(32) NOT NULL DEFAULT '' COMMENT '消息类型',
  `text` text COMMENT '消息内容',
  `raw_json` longtext COMMENT '原始消息JSON',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybbm_bot` (`bot_id`,`id`),
  KEY `idx_ybbm_user` (`telegram_user_id`,`id`),
  KEY `idx_ybbm_message` (`bot_id`,`message_id`),
  UNIQUE KEY `uk_ybbm_chat_message` (`chat_id`,`message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴全局Bot消息日志';
ALTER TABLE `hg_youban_bot_message` ADD COLUMN `retained_at` datetime DEFAULT NULL COMMENT '业务保留时间';

CREATE TABLE IF NOT EXISTS `hg_youban_bot_channel_cache` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Chat ID',
  `chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT 'Chat类型',
  `chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT 'Chat标题',
  `chat_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'Chat用户名',
  `is_broadcast` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否频道',
  `is_megagroup` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否群聊',
  `message_count` int(11) NOT NULL DEFAULT '0' COMMENT '消息数',
  `last_message_text` text COMMENT '最后消息',
  `last_message_at` datetime DEFAULT NULL COMMENT '最后消息时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybbcc_bot_chat` (`bot_id`,`chat_id`),
  KEY `idx_ybbcc_last` (`bot_id`,`last_message_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴全局Bot频道缓存';

ALTER TABLE `hg_youban_bot_bot` ADD COLUMN `run_mode` varchar(32) NOT NULL DEFAULT 'auto' COMMENT '运行模式' AFTER `is_default`;
ALTER TABLE `hg_youban_bot_bot` ADD COLUMN `webhook_url` varchar(500) NOT NULL DEFAULT '' COMMENT 'Webhook地址' AFTER `run_mode`;
ALTER TABLE `hg_youban_bot_user` ADD COLUMN `is_super_admin` tinyint(1) NOT NULL DEFAULT '0' COMMENT '超级管理员' AFTER `status`;

ALTER TABLE `hg_youban_bot_account_bind` DROP INDEX `uk_ybbab_account`;
ALTER TABLE `hg_youban_bot_account_bind` DROP INDEX `uk_ybbab_telegram`;
ALTER TABLE `hg_youban_bot_account_bind`
  ADD COLUMN IF NOT EXISTS `active_account_id` bigint(20) GENERATED ALWAYS AS (CASE WHEN `status`=1 AND `deleted_at` IS NULL THEN `account_id` ELSE NULL END) STORED,
  ADD COLUMN IF NOT EXISTS `active_telegram_user_id` varchar(128) GENERATED ALWAYS AS (CASE WHEN `status`=1 AND `deleted_at` IS NULL THEN `telegram_user_id` ELSE NULL END) STORED;
ALTER TABLE `hg_youban_bot_account_bind` ADD UNIQUE KEY `uk_ybbab_account` (`app`,`active_account_id`);
ALTER TABLE `hg_youban_bot_account_bind` ADD UNIQUE KEY `uk_ybbab_telegram` (`app`,`active_telegram_user_id`);

CREATE TABLE IF NOT EXISTS `hg_youban_bot_invite_code` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `code` varchar(16) NOT NULL DEFAULT '' COMMENT '邀请码',
  `source` varchar(16) NOT NULL DEFAULT 'web' COMMENT '来源',
  `inviter_app` varchar(32) NOT NULL DEFAULT 'api' COMMENT '邀请人应用',
  `inviter_tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '邀请人租户ID',
  `inviter_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '邀请人账号ID',
  `inviter_username` varchar(128) NOT NULL DEFAULT '' COMMENT '邀请人账号',
  `inviter_nickname` varchar(128) NOT NULL DEFAULT '' COMMENT '邀请人昵称',
  `used_tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '使用邀请码注册租户ID',
  `used_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '使用邀请码注册账号ID',
  `used_username` varchar(128) NOT NULL DEFAULT '' COMMENT '使用邀请码注册账号',
  `status` varchar(16) NOT NULL DEFAULT 'active' COMMENT '状态',
  `expires_at` datetime DEFAULT NULL COMMENT '过期时间',
  `used_at` datetime DEFAULT NULL COMMENT '使用时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybbic_code` (`code`),
  KEY `idx_ybbic_inviter` (`inviter_app`,`inviter_account_id`,`source`,`status`,`id`),
  KEY `idx_ybbic_status` (`status`,`expires_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴全局Bot邀请码';

CREATE TABLE IF NOT EXISTS `hg_youban_bot_profile_session` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Chat ID',
  `app` varchar(32) NOT NULL DEFAULT 'api' COMMENT '应用',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `scene` varchar(32) NOT NULL DEFAULT '' COMMENT '场景',
  `step` varchar(64) NOT NULL DEFAULT '' COMMENT '步骤',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `profile_no` varchar(32) NOT NULL DEFAULT '' COMMENT '资料编号',
  `payload_json` text COMMENT '会话数据',
  `expires_at` datetime DEFAULT NULL COMMENT '过期时间',
  `status` varchar(32) NOT NULL DEFAULT 'active' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybbps_user` (`bot_id`,`telegram_user_id`,`chat_id`,`status`),
  KEY `idx_ybbps_expire` (`status`,`expires_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴Bot资料管理会话';

CREATE TABLE IF NOT EXISTS `hg_youban_bot_inline_share` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `token` varchar(64) NOT NULL DEFAULT '' COMMENT '内联分享Token',
  `profile_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '资料ID',
  `profile_no` varchar(32) NOT NULL DEFAULT '' COMMENT '资料编号',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT '创建TG用户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `usage_count` int(11) NOT NULL DEFAULT '0' COMMENT '使用次数',
  `expires_at` datetime DEFAULT NULL COMMENT '过期时间',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybbis_token` (`token`),
  KEY `idx_ybbis_profile` (`profile_no`,`status`,`id`),
  KEY `idx_ybbis_owner` (`tenant_id`,`account_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴Bot资料内联分享';

CREATE TABLE IF NOT EXISTS `hg_youban_bot_custom_emoji` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `custom_emoji_id` varchar(64) NOT NULL DEFAULT '' COMMENT 'Telegram自定义Emoji ID',
  `file_unique_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'Telegram文件唯一ID',
  `attachment_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'HotGo附件ID',
  `storage_path` varchar(500) NOT NULL DEFAULT '' COMMENT '存储路径',
  `file_url` varchar(1000) NOT NULL DEFAULT '' COMMENT '访问地址',
  `file_format` varchar(16) NOT NULL DEFAULT '' COMMENT 'Telegram原始格式',
  `render_type` varchar(16) NOT NULL DEFAULT '' COMMENT '渲染类型',
  `fallback_emoji` varchar(64) NOT NULL DEFAULT '' COMMENT '回退Emoji',
  `width` int(11) NOT NULL DEFAULT '0' COMMENT '宽度',
  `height` int(11) NOT NULL DEFAULT '0' COMMENT '高度',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybbce_emoji` (`custom_emoji_id`),
  KEY `idx_ybbce_file` (`file_unique_id`),
  KEY `idx_ybbce_status` (`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴Telegram自定义Emoji缓存';
