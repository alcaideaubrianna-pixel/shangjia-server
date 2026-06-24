CREATE TABLE IF NOT EXISTS `hg_youban_chat_conversation` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL COMMENT '会员ID',
  `profile_id` bigint(20) NOT NULL COMMENT '资料ID',
  `pocketping_session_id` varchar(128) NOT NULL DEFAULT '' COMMENT '本地会话标识',
  `chatwoot_contact_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '旧Chatwoot联系人ID',
  `chatwoot_conversation_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '旧Chatwoot会话ID',
  `tg_chat_id` varchar(128) DEFAULT NULL COMMENT 'TG群ID',
  `tg_message_thread_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG话题ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `last_message` varchar(500) DEFAULT NULL COMMENT '最后消息',
  `last_message_at` datetime DEFAULT NULL COMMENT '最后消息时间',
  `unread_count` int(11) NOT NULL DEFAULT '0' COMMENT '未读数',
  `pinned_at` datetime DEFAULT NULL COMMENT '置顶时间',
  `hidden_before_at` datetime DEFAULT NULL COMMENT '用户清空记录时间',
  `status` varchar(32) NOT NULL DEFAULT 'opened' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybc_member_profile` (`member_id`, `profile_id`),
  KEY `idx_ybc_pocketping_session` (`pocketping_session_id`),
  KEY `idx_ybc_member_updated` (`member_id`, `updated_at`),
  KEY `idx_ybc_profile` (`profile_id`),
  KEY `idx_ybc_tg_thread` (`tg_chat_id`, `tg_message_thread_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_会话映射';

ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `pocketping_session_id` varchar(128) NOT NULL DEFAULT '' COMMENT '本地会话标识' AFTER `profile_id`;
ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `routing_rule_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '路由规则ID' AFTER `tg_message_thread_id`;
ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `assigned_operator_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '分配客服ID' AFTER `routing_rule_id`;
ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID' AFTER `tg_message_thread_id`;
ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `pinned_at` datetime DEFAULT NULL COMMENT '置顶时间';
ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `hidden_before_at` datetime DEFAULT NULL COMMENT '用户清空记录时间';

CREATE TABLE IF NOT EXISTS `hg_youban_chat_message` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `conversation_id` bigint(20) NOT NULL COMMENT '本地会话ID',
  `pocketping_message_id` varchar(128) NOT NULL DEFAULT '' COMMENT '外部消息标识',
  `direction` varchar(16) NOT NULL DEFAULT 'mine' COMMENT '消息方向',
  `content` text COMMENT '消息内容',
  `content_type` varchar(32) NOT NULL DEFAULT 'text' COMMENT '内容类型',
  `status` varchar(32) NOT NULL DEFAULT 'sent' COMMENT '状态',
  `sender_name` varchar(128) DEFAULT NULL COMMENT '发送人',
  `attachments_json` text COMMENT '附件JSON',
  `tg_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG群ID',
  `tg_message_thread_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG话题ID',
  `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID',
  `read_at` datetime DEFAULT NULL COMMENT '已读时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybcm_pocketping_message` (`pocketping_message_id`),
  KEY `idx_ybcm_conversation_id` (`conversation_id`, `id`),
  KEY `idx_ybcm_created_at` (`created_at`),
  KEY `idx_ybcm_tg_message` (`tg_chat_id`, `tg_message_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_消息';
ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `tg_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG群ID';
ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `tg_message_thread_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG话题ID';
ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID';
ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `read_at` datetime DEFAULT NULL COMMENT '已读时间';

CREATE TABLE IF NOT EXISTS `hg_youban_chat_bot` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bot_name` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot名称',
  `bot_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'Bot用户名',
  `bot_token` varchar(255) NOT NULL DEFAULT '' COMMENT 'Bot Token',
  `remark` varchar(255) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybcb_username` (`bot_username`),
  KEY `idx_ybcb_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_Bot';

CREATE TABLE IF NOT EXISTS `hg_youban_chat_binding` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `bind_code` varchar(64) NOT NULL DEFAULT '' COMMENT '绑定码',
  `bind_type` varchar(32) NOT NULL DEFAULT 'channel' COMMENT '绑定类型',
  `source_channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源频道ID',
  `content_channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '本地频道ID',
  `bot_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'Bot ID',
  `tg_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG群ID',
  `tg_chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT 'TG群标题',
  `remark` varchar(255) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybcb_bind_code` (`bind_code`),
  KEY `idx_ybcb_source_channel` (`source_channel_id`, `status`),
  KEY `idx_ybcb_content_channel` (`content_channel_id`, `status`),
  KEY `idx_ybcb_global` (`bind_type`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_频道群绑定';

CREATE TABLE IF NOT EXISTS `hg_youban_chat_binding_channel` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `binding_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '绑定ID',
  `channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '频道ID',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybcbc_binding_channel` (`binding_id`, `channel_id`),
  KEY `idx_ybcbc_channel` (`channel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_绑定频道';

CREATE TABLE IF NOT EXISTS `hg_youban_chat_operator` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `admin_member_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '后台会员ID',
  `telegram_user_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户ID',
  `telegram_username` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG用户名',
  `display_name` varchar(128) NOT NULL DEFAULT '' COMMENT '显示名称',
  `bind_code` varchar(64) NOT NULL DEFAULT '' COMMENT '绑定码',
  `remark` varchar(255) NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_ybco_admin_member` (`admin_member_id`),
  KEY `idx_ybco_telegram_user` (`telegram_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_客服绑定';

CREATE TABLE IF NOT EXISTS `hg_youban_chat_feature` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `feature_key` varchar(64) NOT NULL DEFAULT '' COMMENT '功能Key',
  `name` varchar(128) NOT NULL DEFAULT '' COMMENT '功能名称',
  `command` varchar(64) NOT NULL DEFAULT '' COMMENT 'Telegram命令',
  `description` varchar(255) NOT NULL DEFAULT '' COMMENT '功能描述',
  `config_json` text COMMENT '配置JSON',
  `sort` int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybcf_feature_key` (`feature_key`),
  KEY `idx_ybcf_status_sort` (`status`, `sort`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_Telegram功能配置';
