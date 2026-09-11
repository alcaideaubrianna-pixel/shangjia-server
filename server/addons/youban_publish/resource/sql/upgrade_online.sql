-- Interactive upgrades must remain short. Historical cleanup and backfills belong in upgrade.sql.
CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_dedupe_entry` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0', `account_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0', `layer` varchar(32) NOT NULL DEFAULT '',
  `signature` varchar(64) NOT NULL DEFAULT '', `item_total` int(11) NOT NULL DEFAULT '0',
  `signature_count` int(11) NOT NULL DEFAULT '0', `first_event_id` bigint(20) NOT NULL DEFAULT '0',
  `last_event_id` bigint(20) NOT NULL DEFAULT '0', `first_seen_at` datetime DEFAULT NULL,
  `last_seen_at` datetime DEFAULT NULL, `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_collect_dedupe_entry` (`tenant_id`,`account_id`,`channel_id`,`layer`,`signature`,`item_total`,`signature_count`),
  KEY `idx_ybp_collect_dedupe_lookup` (`tenant_id`,`account_id`,`layer`,`signature`,`channel_id`,`last_seen_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='采集永久去重账本';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_dedupe_source` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT, `entry_id` bigint(20) NOT NULL DEFAULT '0',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0', `account_id` bigint(20) NOT NULL DEFAULT '0',
  `source_id` bigint(20) NOT NULL DEFAULT '0', `rule_id` bigint(20) NOT NULL DEFAULT '0',
  `dispatch_id` bigint(20) NOT NULL DEFAULT '0', `event_id` bigint(20) NOT NULL DEFAULT '0',
  `accepted_at` datetime DEFAULT NULL, `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_collect_dedupe_source` (`entry_id`,`dispatch_id`),
  KEY `idx_ybp_collect_dedupe_source_owner` (`tenant_id`,`account_id`,`source_id`,`entry_id`),
  KEY `idx_ybp_collect_dedupe_source_dispatch` (`dispatch_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='采集去重来源贡献';

-- A bound rule is source-local. Global rules are account-level delete/replace policies only.
UPDATE `hg_youban_publish_collect_rule` r
INNER JOIN `hg_youban_publish_collect_source_rule` sr ON sr.`rule_id` = r.`id`
SET r.`global_enabled` = 0, r.`updated_at` = CURRENT_TIMESTAMP
WHERE r.`global_enabled` = 1;

DELETE sr
FROM `hg_youban_publish_collect_source_rule` sr
INNER JOIN `hg_youban_publish_collect_source_rule` keep
  ON keep.`source_id` = sr.`source_id`
 AND (keep.`sort` < sr.`sort` OR (keep.`sort` = sr.`sort` AND keep.`id` < sr.`id`));

ALTER TABLE `hg_youban_publish_collect_source_rule`
  ADD UNIQUE KEY `uk_ybp_collect_source_single_rule` (`source_id`);
CREATE TABLE IF NOT EXISTS `hg_youban_publish_profile_fingerprint` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0', `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0', `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `layer` varchar(32) NOT NULL DEFAULT '', `signature` varchar(64) NOT NULL DEFAULT '',
  `item_total` int(11) NOT NULL DEFAULT '0', `signature_count` int(11) NOT NULL DEFAULT '0',
  `owner_marker` varchar(16) DEFAULT NULL,
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_profile_fingerprint_scope` (`tenant_id`,`account_id`,`channel_id`,`layer`,`signature`,`item_total`,`signature_count`,`owner_marker`),
  UNIQUE KEY `uk_ybp_profile_fingerprint_profile` (`profile_id`,`channel_id`,`layer`,`signature`,`item_total`,`signature_count`),
  KEY `idx_ybp_profile_fingerprint_profile` (`profile_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE `hg_youban_publish_channel_profile` ADD INDEX IF NOT EXISTS `idx_ybp_channel_profile_profile` (`profile_id`,`channel_id`);
