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

CREATE TABLE IF NOT EXISTS `hg_content_profile_stats` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `profile_id` bigint(20) NOT NULL DEFAULT 0 COMMENT '资料ID',
  `view_total` int(11) NOT NULL DEFAULT 0 COMMENT '总浏览量',
  `view_24h` int(11) NOT NULL DEFAULT 0 COMMENT '近24小时浏览量',
  `click_total` int(11) NOT NULL DEFAULT 0 COMMENT '总点击量',
  `hot_score` int(11) NOT NULL DEFAULT 0 COMMENT '热度分',
  `last_view_at` datetime DEFAULT NULL COMMENT '最后浏览时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_content_profile_stats_profile` (`profile_id`),
  KEY `idx_content_profile_stats_hot` (`hot_score`,`view_24h`,`profile_id`),
  KEY `idx_content_profile_stats_view` (`last_view_at`,`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='内容资料热度统计';

INSERT INTO `hg_content_profile_stats` (`profile_id`,`view_total`,`view_24h`,`click_total`,`hot_score`,`last_view_at`,`created_at`,`updated_at`)
SELECT `profile_id`,
       COALESCE(SUM(`view_count`), 0),
       COALESCE(SUM(`view_count`), 0),
       0,
       COALESCE(SUM(CASE WHEN `last_view_at` >= DATE_SUB(NOW(), INTERVAL 24 HOUR) THEN `view_count` ELSE 0 END), 0),
       MAX(`last_view_at`),
       NOW(),
       NOW()
FROM `hg_member_profile_view`
WHERE `deleted_at` IS NULL
GROUP BY `profile_id`
ON DUPLICATE KEY UPDATE
  `view_total` = VALUES(`view_total`),
  `hot_score` = VALUES(`hot_score`),
  `view_24h` = VALUES(`view_24h`),
  `last_view_at` = VALUES(`last_view_at`),
  `updated_at` = VALUES(`updated_at`);
