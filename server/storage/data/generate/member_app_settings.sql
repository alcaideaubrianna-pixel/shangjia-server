CREATE TABLE IF NOT EXISTS `hg_member_favorite` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '用户ID',
  `profile_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '资料ID',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_member_profile_deleted` (`member_id`,`profile_id`,`deleted_at`),
  KEY `idx_member_created` (`member_id`,`created_at`),
  KEY `idx_profile` (`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='移动端用户收藏';

CREATE TABLE IF NOT EXISTS `hg_member_setting` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '用户ID',
  `message_enabled` tinyint(1) NOT NULL DEFAULT 1 COMMENT '新消息提醒',
  `hide_online` tinyint(1) NOT NULL DEFAULT 0 COMMENT '隐藏在线状态',
  `hide_view_history` tinyint(1) NOT NULL DEFAULT 1 COMMENT '隐藏浏览记录',
  `match_chat_only` tinyint(1) NOT NULL DEFAULT 1 COMMENT '仅匹配后聊天',
  `profile_scope` varchar(32) NOT NULL DEFAULT 'all' COMMENT '资料可见范围',
  `photo_scope` varchar(32) NOT NULL DEFAULT 'verified' COMMENT '照片可见范围',
  `theme_mode` varchar(16) NOT NULL DEFAULT 'system' COMMENT '主题模式',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_member_deleted` (`member_id`,`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='移动端用户设置';
