CREATE TABLE IF NOT EXISTS `hg_member_profile_view` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '用户ID',
  `profile_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '资料ID',
  `view_count` int(11) NOT NULL DEFAULT 0 COMMENT '查看次数',
  `last_view_at` datetime DEFAULT NULL COMMENT '最后查看时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_member_profile_deleted` (`member_id`,`profile_id`,`deleted_at`),
  KEY `idx_member_last_view` (`member_id`,`last_view_at`),
  KEY `idx_profile` (`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='移动端资料浏览痕迹';
