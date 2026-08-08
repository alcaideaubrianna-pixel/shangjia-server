CREATE TABLE IF NOT EXISTS `hg_youban_publish_profile_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `created_by` bigint(20) NOT NULL DEFAULT '0',
  `updated_by` bigint(20) NOT NULL DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_profile_channel` (`tenant_id`,`profile_id`,`channel_id`),
  KEY `idx_ybp_profile_channel_owner` (`tenant_id`,`account_id`,`profile_id`),
  KEY `idx_ybp_profile_channel_channel` (`tenant_id`,`channel_id`,`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='资料推送频道映射';
ALTER TABLE `hg_youban_publish_profile_channel` ADD COLUMN IF NOT EXISTS `is_manual` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否资料手动选择';
UPDATE `hg_youban_publish_profile_channel` SET `is_manual` = 0;

INSERT IGNORE INTO `hg_youban_publish_profile_channel` (`tenant_id`,`account_id`,`profile_id`,`channel_id`,`is_manual`,`created_at`,`updated_at`)
SELECT ps.`tenant_id`,ps.`account_id`,ps.`profile_id`,jt.`channel_id`,0,NOW(),NOW()
FROM `hg_youban_publish_profile_state` ps
JOIN JSON_TABLE(CASE WHEN JSON_VALID(ps.`channel_id_json`) THEN ps.`channel_id_json` ELSE '[]' END, '$[*]' COLUMNS (`channel_id` bigint PATH '$')) jt
WHERE ps.`channel_id_json` IS NOT NULL AND ps.`channel_id_json` <> '' AND ps.`deleted_at` IS NULL;

ALTER TABLE `hg_youban_publish_profile_state` DROP COLUMN `channel_id_json`;
