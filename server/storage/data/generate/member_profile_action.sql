CREATE TABLE IF NOT EXISTS `hg_member_profile_action` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '用户ID',
  `profile_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '资料ID',
  `action_type` varchar(32) NOT NULL DEFAULT '' COMMENT '动作类型：reject/block',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_member_profile_action_deleted` (`member_id`,`profile_id`,`action_type`,`deleted_at`),
  KEY `idx_member_action_created` (`member_id`,`action_type`,`created_at`),
  KEY `idx_profile` (`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='移动端资料动作';
