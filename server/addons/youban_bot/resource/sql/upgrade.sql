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
  KEY `idx_ybbm_message` (`bot_id`,`message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴全局Bot消息日志';

ALTER TABLE `hg_youban_bot_bot` ADD COLUMN `run_mode` varchar(32) NOT NULL DEFAULT 'auto' COMMENT '运行模式' AFTER `is_default`;
ALTER TABLE `hg_youban_bot_bot` ADD COLUMN `webhook_url` varchar(500) NOT NULL DEFAULT '' COMMENT 'Webhook地址' AFTER `run_mode`;
ALTER TABLE `hg_youban_bot_user` ADD COLUMN `is_super_admin` tinyint(1) NOT NULL DEFAULT '0' COMMENT '超级管理员' AFTER `status`;
