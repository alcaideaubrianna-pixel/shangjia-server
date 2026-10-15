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


CREATE TABLE IF NOT EXISTS `hg_youban_publish_image_quota` (
  `id` bigint NOT NULL AUTO_INCREMENT, `tenant_id` bigint NOT NULL DEFAULT '0',
  `monthly_limit` bigint NOT NULL DEFAULT '5000', `monthly_used` bigint NOT NULL DEFAULT '0',
  `purchased_remaining` bigint NOT NULL DEFAULT '0', `period` varchar(7) NOT NULL DEFAULT '',
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_image_quota_tenant` (`tenant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS `hg_youban_publish_image_quota_ledger` (
  `id` bigint NOT NULL AUTO_INCREMENT, `tenant_id` bigint NOT NULL DEFAULT '0', `account_id` bigint NOT NULL DEFAULT '0',
  `event_type` varchar(24) NOT NULL DEFAULT '', `reference_key` varchar(160) NOT NULL DEFAULT '',
  `change_count` bigint NOT NULL DEFAULT '0', `remark` varchar(500) NOT NULL DEFAULT '', `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_image_quota_ledger_ref` (`reference_key`),
  KEY `idx_ybp_image_quota_ledger_tenant` (`tenant_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN IF NOT EXISTS `anti_scan_mode` varchar(32) NOT NULL DEFAULT 'lightweight';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN IF NOT EXISTS `anti_scan_background_url` varchar(1024) NOT NULL DEFAULT '';
ALTER TABLE `hg_youban_publish_channel` ADD COLUMN IF NOT EXISTS `anti_scan_background_name` varchar(255) NOT NULL DEFAULT '';
ALTER TABLE `hg_youban_publish_channel_profile` ADD INDEX IF NOT EXISTS `idx_ybp_channel_profile_profile` (`profile_id`,`channel_id`);
ALTER TABLE `hg_youban_publish_anti_scan_cache` ADD COLUMN IF NOT EXISTS `media_id` bigint(20) NOT NULL DEFAULT '0';
ALTER TABLE `hg_youban_publish_anti_scan_cache` ADD COLUMN IF NOT EXISTS `image_width` int(11) NOT NULL DEFAULT '0';
ALTER TABLE `hg_youban_publish_anti_scan_cache` ADD COLUMN IF NOT EXISTS `image_height` int(11) NOT NULL DEFAULT '0';
ALTER TABLE `hg_youban_publish_anti_scan_cache` ADD INDEX IF NOT EXISTS `idx_ybp_anti_scan_media` (`media_id`,`cloud_raw_saved`);
ALTER TABLE `hg_youban_publish_anti_scan_cache` ADD INDEX IF NOT EXISTS `idx_ybp_anti_scan_media_provider` (`media_id`,`provider`,`cloud_raw_saved`);
ALTER TABLE `hg_youban_publish_anti_scan_cache` ADD INDEX IF NOT EXISTS `idx_ybp_anti_scan_image_provider` (`image_hash`,`provider`,`cloud_raw_saved`);
ALTER TABLE `hg_youban_publish_message_media` ADD COLUMN IF NOT EXISTS `original_file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始访问地址';
ALTER TABLE `hg_youban_publish_message_media` ADD COLUMN IF NOT EXISTS `original_storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '原始存储路径';
ALTER TABLE `hg_youban_publish_message_media` ADD COLUMN IF NOT EXISTS `edit_status` varchar(16) NOT NULL DEFAULT 'raw' COMMENT '编辑状态：raw/edited';
ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN IF NOT EXISTS `channel_suffix` text NOT NULL;
ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN IF NOT EXISTS `channel_blacklisted` tinyint(1) NOT NULL DEFAULT 0;
ALTER TABLE `hg_youban_publish_collect_source` ADD KEY `idx_ybp_collect_source_channel_rule` (`tenant_id`,`account_id`,`source_chat_id`,`channel_blacklisted`);
CREATE TABLE IF NOT EXISTS `hg_youban_publish_intro_fee_channel_rule` (
 `id` bigint NOT NULL AUTO_INCREMENT, `tenant_id` bigint NOT NULL DEFAULT 0, `rule_id` bigint NOT NULL DEFAULT 0, `bot_id` bigint NOT NULL DEFAULT 0,
 `channel_id` varchar(128) NOT NULL DEFAULT '', `channel_title` varchar(255) NOT NULL DEFAULT '', `channel_username` varchar(128) NOT NULL DEFAULT '',
 `suffix` varchar(255) NOT NULL DEFAULT '', `status` tinyint NOT NULL DEFAULT 1, `last_seen_at` datetime NULL, `created_at` datetime NULL, `updated_at` datetime NULL, `deleted_at` datetime NULL,
 PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_intro_fee_channel_rule` (`tenant_id`,`rule_id`,`bot_id`,`channel_id`), KEY `idx_ybp_intro_fee_channel_rule_lookup` (`tenant_id`,`rule_id`,`bot_id`,`channel_id`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_channel_profile` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT 0,
  `account_id` bigint(20) NOT NULL DEFAULT 0,
  `source_id` bigint(20) NOT NULL DEFAULT 0,
  `rule_id` bigint(20) NOT NULL DEFAULT 0,
  `channel_id` varchar(128) NOT NULL DEFAULT '',
  `profile_id` bigint(20) NOT NULL DEFAULT 0,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_collect_channel_profile` (`tenant_id`,`account_id`,`rule_id`,`channel_id`,`profile_id`),
  KEY `idx_ybp_collect_channel_profile_list` (`tenant_id`,`account_id`,`rule_id`,`channel_id`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='采集频道资料计数索引';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tg_delivery_step` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT, `job_id` bigint(20) NOT NULL DEFAULT 0,
  `step_key` varchar(32) NOT NULL DEFAULT '', `status` varchar(32) NOT NULL DEFAULT 'pending',
  `attempt_id` bigint(20) NOT NULL DEFAULT 0, `retry_count` int(11) NOT NULL DEFAULT 0,
  `next_retry_at` datetime DEFAULT NULL, `error_message` text,
  `started_at` datetime DEFAULT NULL, `confirmed_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL, `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `uk_ybp_tg_delivery_step_job` (`job_id`,`step_key`),
  KEY `idx_ybp_tg_delivery_step_recovery` (`status`,`next_retry_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='TG发布步骤状态';

ALTER TABLE hg_youban_bot_registry ADD COLUMN account_id bigint NOT NULL DEFAULT 0;
CREATE INDEX idx_ybp_bot_account ON hg_youban_bot_registry(tenant_id,account_id,created_by,status);

ALTER TABLE hg_youban_bot_registry ADD COLUMN bot_type varchar(32) NOT NULL DEFAULT 'normal';
