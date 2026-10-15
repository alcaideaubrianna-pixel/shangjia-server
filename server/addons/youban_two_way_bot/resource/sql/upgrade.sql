CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_bot` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '创建账号ID',
  `tg_account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG协议号ID',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '名称',
  `welcome_message` varchar(1000) NOT NULL DEFAULT '' COMMENT '欢迎语',
  `bot_token` varchar(255) NOT NULL DEFAULT '' COMMENT 'Bot Token',
  `bot_user_id` varchar(64) NOT NULL DEFAULT '' COMMENT 'Bot用户ID',
  `bot_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot用户名',
  `supergroup_id` varchar(64) NOT NULL DEFAULT '' COMMENT '管理群ID',
  `supergroup_access_hash` varchar(128) NOT NULL DEFAULT '' COMMENT '管理群AccessHash',
  `supergroup_title` varchar(128) NOT NULL DEFAULT '' COMMENT '管理群名称',
  `invite_link` varchar(512) NOT NULL DEFAULT '' COMMENT '邀请链接',
  `setup_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '初始化状态',
  `webhook_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT 'Webhook状态',
  `status` tinyint(1) NOT NULL DEFAULT '1' COMMENT '状态',
  `error_message` varchar(1024) NOT NULL DEFAULT '' COMMENT '错误信息',
  `last_setup_at` datetime DEFAULT NULL COMMENT '最后初始化时间',
  `last_webhook_at` datetime DEFAULT NULL COMMENT '最后Webhook时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybtwb_bot_tenant` (`tenant_id`,`status`,`id`),
  KEY `idx_ybtwb_bot_tg_account` (`tenant_id`,`tg_account_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='双向机器人';
ALTER TABLE `hg_youban_two_way_bot_bot` ADD COLUMN IF NOT EXISTS `welcome_message` varchar(1000) NOT NULL DEFAULT '' COMMENT '欢迎语';

CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_topic` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '机器人ID',
  `telegram_user_id` varchar(64) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `telegram_first_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG名',
  `telegram_last_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG姓',
  `thread_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '话题ID',
  `title` varchar(128) NOT NULL DEFAULT '' COMMENT '话题标题',
  `closed` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否关闭',
  `last_message_at` datetime DEFAULT NULL COMMENT '最后消息时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybtwb_topic_user` (`tenant_id`,`bot_id`,`telegram_user_id`),
  UNIQUE KEY `uk_ybtwb_topic_thread` (`tenant_id`,`bot_id`,`thread_id`),
  KEY `idx_ybtwb_topic_last` (`tenant_id`,`bot_id`,`last_message_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='双向机器人话题';

CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_message` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '机器人ID',
  `direction` varchar(16) NOT NULL DEFAULT '' COMMENT '方向',
  `telegram_user_id` varchar(64) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `thread_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '话题ID',
  `source_chat_id` varchar(64) NOT NULL DEFAULT '' COMMENT '源聊天ID',
  `source_message_id` int(11) NOT NULL DEFAULT '0' COMMENT '源消息ID',
  `target_chat_id` varchar(64) NOT NULL DEFAULT '' COMMENT '目标聊天ID',
  `target_message_id` int(11) NOT NULL DEFAULT '0' COMMENT '目标消息ID',
  `media_group_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID',
  `status` varchar(32) NOT NULL DEFAULT 'sent' COMMENT '状态',
  `error_message` varchar(1024) NOT NULL DEFAULT '' COMMENT '错误信息',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybtwb_msg_topic` (`tenant_id`,`bot_id`,`thread_id`,`id`),
  KEY `idx_ybtwb_msg_user` (`tenant_id`,`bot_id`,`telegram_user_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='双向机器人消息';

CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_cooperation_config` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `account_id` bigint NOT NULL DEFAULT 0,
  `bot_id` bigint NOT NULL DEFAULT 0,
  `two_way_bot_id` bigint NOT NULL DEFAULT 0,
  `notification_type` varchar(20) NOT NULL DEFAULT 'two_way',
  `review_required` tinyint NOT NULL DEFAULT 1,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_by` bigint NOT NULL DEFAULT 0,
  `updated_by` bigint NOT NULL DEFAULT 0,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybtwb_coop_config_tenant` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_cooperation_channel` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `config_id` bigint NOT NULL DEFAULT 0,
  `channel_id` bigint NOT NULL DEFAULT 0,
  `status` tinyint NOT NULL DEFAULT 1,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybtwb_coop_channel` (`config_id`,`channel_id`),
  KEY `idx_ybtwb_coop_channel_tenant` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_cooperation_blacklist` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `config_id` bigint NOT NULL DEFAULT 0,
  `applicant_tg_user_id` varchar(64) NOT NULL DEFAULT '',
  `applicant_username` varchar(128) NOT NULL DEFAULT '',
  `applicant_first_name` varchar(128) NOT NULL DEFAULT '',
  `applicant_last_name` varchar(128) NOT NULL DEFAULT '',
  `reason` varchar(500) NOT NULL DEFAULT '',
  `status` tinyint NOT NULL DEFAULT 1,
  `created_by` bigint NOT NULL DEFAULT 0,
  `updated_by` bigint NOT NULL DEFAULT 0,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybtwb_coop_blacklist_user` (`config_id`,`applicant_tg_user_id`),
  KEY `idx_ybtwb_coop_blacklist_tenant` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_cooperation_application` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `config_id` bigint NOT NULL DEFAULT 0,
  `applicant_tg_user_id` varchar(64) NOT NULL DEFAULT '',
  `applicant_username` varchar(128) NOT NULL DEFAULT '',
  `applicant_first_name` varchar(128) NOT NULL DEFAULT '',
  `applicant_last_name` varchar(128) NOT NULL DEFAULT '',
  `submitted_bot_user_id` varchar(64) NOT NULL DEFAULT '',
  `submitted_bot_username` varchar(128) NOT NULL DEFAULT '',
  `submitted_bot_name` varchar(255) NOT NULL DEFAULT '',
  `review_status` varchar(24) NOT NULL DEFAULT 'pending',
  `join_status` varchar(24) NOT NULL DEFAULT 'not_started',
  `topic_thread_id` bigint NOT NULL DEFAULT 0,
  `reviewed_by` bigint NOT NULL DEFAULT 0,
  `review_remark` varchar(500) NOT NULL DEFAULT '',
  `error_message` text,
  `submitted_at` datetime DEFAULT NULL,
  `reviewed_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybtwb_coop_app_tenant` (`tenant_id`,`review_status`,`join_status`,`id`),
  KEY `idx_ybtwb_coop_app_bot` (`config_id`,`submitted_bot_user_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS `hg_youban_two_way_bot_cooperation_application_channel` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `application_id` bigint NOT NULL DEFAULT 0,
  `channel_id` bigint NOT NULL DEFAULT 0,
  `status` varchar(24) NOT NULL DEFAULT 'not_started',
  `error_message` text,
  `retry_count` int NOT NULL DEFAULT 0,
  `joined_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybtwb_coop_app_channel` (`application_id`,`channel_id`),
  KEY `idx_ybtwb_coop_app_channel_status` (`tenant_id`,`status`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO `hg_sys_addons_install` (`name`,`version`,`status`,`created_at`,`updated_at`) VALUES ('youban_tg_bot_gateway','v1.0.0',1,NOW(),NOW()) ON DUPLICATE KEY UPDATE `version`=VALUES(`version`),`status`=1,`updated_at`=NOW();

-- 将旧版双向机器人 Token 幂等迁移到通用 Bot Token 表。
UPDATE `hg_youban_two_way_bot_bot`
SET `status` = 1, `updated_at` = NOW()
WHERE `deleted_at` IS NULL AND `status` = 0 AND `setup_status` = 'ready' AND TRIM(`bot_token`) <> '';
UPDATE `hg_youban_publish_bot` publish_bot
INNER JOIN `hg_youban_two_way_bot_bot` two_way_bot
  ON publish_bot.`tenant_id` = two_way_bot.`tenant_id`
  AND TRIM(publish_bot.`bot_token`) = TRIM(two_way_bot.`bot_token`)
SET publish_bot.`status` = 1, publish_bot.`updated_at` = NOW()
WHERE publish_bot.`deleted_at` IS NULL AND publish_bot.`remark` = '由旧版双向机器人迁移'
  AND two_way_bot.`deleted_at` IS NULL AND two_way_bot.`status` = 1 AND two_way_bot.`setup_status` = 'ready';
INSERT INTO `hg_youban_publish_bot` (`tenant_id`,`bot_name`,`bot_username`,`bot_token`,`remark`,`status`,`created_by`,`updated_by`,`created_at`,`updated_at`)
SELECT legacy.`tenant_id`,
  COALESCE(NULLIF(TRIM(legacy.`name`), ''), NULLIF(TRIM(legacy.`bot_username`), ''), '迁移机器人'),
  TRIM(LEADING '@' FROM TRIM(legacy.`bot_username`)), TRIM(legacy.`bot_token`), '由旧版双向机器人迁移',
  CASE WHEN legacy.`status` = 1 THEN 1 ELSE 2 END, legacy.`account_id`, legacy.`account_id`,
  COALESCE(legacy.`created_at`, NOW()), COALESCE(legacy.`updated_at`, NOW())
FROM `hg_youban_two_way_bot_bot` legacy
LEFT JOIN `hg_youban_two_way_bot_bot` newer
  ON newer.`tenant_id` = legacy.`tenant_id`
  AND TRIM(newer.`bot_token`) = TRIM(legacy.`bot_token`)
  AND newer.`deleted_at` IS NULL
  AND (newer.`status` > legacy.`status` OR (newer.`status` = legacy.`status` AND newer.`id` > legacy.`id`))
WHERE legacy.`deleted_at` IS NULL
  AND TRIM(legacy.`bot_token`) <> ''
  AND newer.`id` IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM `hg_youban_publish_bot` current_bot
    WHERE current_bot.`tenant_id` = legacy.`tenant_id`
      AND TRIM(current_bot.`bot_token`) = TRIM(legacy.`bot_token`)
      AND current_bot.`deleted_at` IS NULL
  );
