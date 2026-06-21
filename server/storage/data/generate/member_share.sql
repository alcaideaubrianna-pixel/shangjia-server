CREATE TABLE IF NOT EXISTS `hg_member_share` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `member_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '分享用户ID',
  `profile_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '资料ID',
  `share_token` varchar(64) NOT NULL DEFAULT '' COMMENT '分享TOKEN',
  `visit_count` int(11) NOT NULL DEFAULT 0 COMMENT '访问次数',
  `register_count` int(11) NOT NULL DEFAULT 0 COMMENT '注册次数',
  `last_visit_at` datetime DEFAULT NULL COMMENT '最后访问时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_share_token` (`share_token`),
  KEY `idx_member_profile` (`member_id`,`profile_id`),
  KEY `idx_profile` (`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='移动端资料分享';
