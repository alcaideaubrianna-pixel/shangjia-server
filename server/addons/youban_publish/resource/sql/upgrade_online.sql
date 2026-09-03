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
