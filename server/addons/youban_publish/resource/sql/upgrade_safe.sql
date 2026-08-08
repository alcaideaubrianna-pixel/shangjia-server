ALTER TABLE `hg_youban_publish_collect_event_media`
  ADD COLUMN IF NOT EXISTS `next_retry_at` datetime DEFAULT NULL COMMENT '下次重试时间';

ALTER TABLE `hg_youban_publish_tg_job`
  ADD KEY `idx_ybp_tg_job_cycle_due` (`cycle_enabled`,`status`,`next_cycle_at`,`id`);

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `level` int NOT NULL DEFAULT 0,
  `status` int NOT NULL DEFAULT 2,
  `opened_at` datetime DEFAULT NULL,
  `expired_at` datetime DEFAULT NULL,
  `remark` text NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tenant_vip_tenant` (`tenant_id`),
  KEY `idx_ybp_vip_expired` (`status`,`expired_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `operator_id` bigint NOT NULL DEFAULT 0,
  `source` varchar(32) NOT NULL DEFAULT '',
  `action` varchar(32) NOT NULL DEFAULT '',
  `before_status` int NOT NULL DEFAULT 0,
  `before_level` int NOT NULL DEFAULT 0,
  `before_expired_at` datetime DEFAULT NULL,
  `after_status` int NOT NULL DEFAULT 0,
  `after_level` int NOT NULL DEFAULT 0,
  `after_expired_at` datetime DEFAULT NULL,
  `remark` text NOT NULL,
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_ybp_tenant_vip_log_tenant` (`tenant_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_coupon` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(64) NOT NULL DEFAULT '',
  `use_type` varchar(16) NOT NULL DEFAULT 'single',
  `amount` decimal(10,2) NOT NULL DEFAULT 0,
  `total_count` int NOT NULL DEFAULT 1,
  `used_count` int NOT NULL DEFAULT 0,
  `status` int NOT NULL DEFAULT 1,
  `remark` text NOT NULL,
  `expired_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `deleted_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_tenant_vip_coupon_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_vip_event` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `event_key` varchar(160) NOT NULL DEFAULT '',
  `event_type` varchar(48) NOT NULL DEFAULT '',
  `activity_code` varchar(64) NOT NULL DEFAULT '',
  `activity_generation` int NOT NULL DEFAULT 1,
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `account_id` bigint NOT NULL DEFAULT 0,
  `trigger_tenant_id` bigint NOT NULL DEFAULT 0,
  `trigger_account_id` bigint NOT NULL DEFAULT 0,
  `reference_type` varchar(32) NOT NULL DEFAULT '',
  `reference_id` varchar(64) NOT NULL DEFAULT '',
  `change_days` int NOT NULL DEFAULT 0,
  `before_expired_at` datetime DEFAULT NULL,
  `after_expired_at` datetime DEFAULT NULL,
  `notify_status` varchar(16) NOT NULL DEFAULT 'pending',
  `notify_retry_count` int NOT NULL DEFAULT 0,
  `notify_next_retry_at` datetime DEFAULT NULL,
  `notified_at` datetime DEFAULT NULL,
  `error_message` text NOT NULL,
  `remark` text NOT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_vip_event_key` (`event_key`),
  KEY `idx_ybp_vip_event_activity` (`activity_code`,`activity_generation`,`id`),
  KEY `idx_ybp_vip_event_tenant` (`tenant_id`,`event_type`,`id`),
  KEY `idx_ybp_vip_event_notify` (`notify_status`,`notify_next_retry_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE `hg_youban_publish_tenant_vip_event`
  ADD COLUMN IF NOT EXISTS `activity_code` varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS `activity_generation` int NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_activity_generation` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `activity_code` varchar(64) NOT NULL DEFAULT '',
  `tenant_id` bigint NOT NULL DEFAULT 0,
  `generation` int NOT NULL DEFAULT 1,
  `reset_reason` text NOT NULL,
  `updated_by` bigint NOT NULL DEFAULT 0,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_activity_generation` (`activity_code`,`tenant_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_youban_publish_cloud_resource_usage` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID，0为系统调用',
  `resource_type` varchar(32) NOT NULL DEFAULT '' COMMENT '资源类型',
  `scene` varchar(32) NOT NULL DEFAULT '' COMMENT '调用场景',
  `usage_date` date NOT NULL COMMENT '统计日期',
  `request_count` bigint(20) NOT NULL DEFAULT '0' COMMENT '请求次数',
  `success_count` bigint(20) NOT NULL DEFAULT '0' COMMENT '成功次数',
  `failure_count` bigint(20) NOT NULL DEFAULT '0' COMMENT '失败次数',
  `total_duration_ms` bigint(20) NOT NULL DEFAULT '0' COMMENT '累计耗时毫秒',
  `last_called_at` datetime DEFAULT NULL COMMENT '最后调用时间',
  `created_at` datetime DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime DEFAULT NULL COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ybp_cloud_usage_daily` (`tenant_id`,`account_id`,`resource_type`,`scene`,`usage_date`),
  KEY `idx_ybp_cloud_usage_date` (`usage_date`,`resource_type`,`account_id`),
  KEY `idx_ybp_cloud_usage_account` (`account_id`,`usage_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴云资源每日调用统计';
ALTER TABLE `hg_youban_publish_media` ADD COLUMN IF NOT EXISTS `must_send` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否每次推送必发' AFTER `media_type`;
UPDATE `hg_youban_publish_media` SET `must_send` = 0 WHERE EXISTS (SELECT 1 FROM `information_schema`.`columns` WHERE `table_schema` = DATABASE() AND `table_name` = 'hg_youban_publish_media' AND `column_name` = 'must_send' AND `column_default` = '1');
ALTER TABLE `hg_youban_publish_media` ALTER COLUMN `must_send` SET DEFAULT '0';
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN IF NOT EXISTS `send_phase` varchar(32) NOT NULL DEFAULT '' COMMENT '发送阶段' AFTER `dispatch_count`;
ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN IF NOT EXISTS `reconcile_count` int(11) NOT NULL DEFAULT '0' COMMENT '对账次数' AFTER `send_phase`;
ALTER TABLE `hg_youban_publish_tg_job` ADD INDEX `idx_ybp_tg_job_tenant_channel_status` (`tenant_id`,`channel_id`,`status`,`id`);
DELETE older FROM `hg_youban_publish_tg_message` older INNER JOIN `hg_youban_publish_tg_message` newer ON older.`job_id`=newer.`job_id` AND older.`tg_message_id`=newer.`tg_message_id` AND older.`id`<newer.`id`;
ALTER TABLE `hg_youban_publish_tg_message` ADD UNIQUE INDEX `uk_ybp_tg_message_job_message` (`job_id`,`tg_message_id`);
ALTER TABLE `hg_youban_publish_tg_channel_stat` MODIFY COLUMN `last_error_message` text NOT NULL COMMENT '最后错误';
ALTER TABLE `hg_youban_publish_tg_bot_stat` MODIFY COLUMN `last_error_message` text NOT NULL COMMENT '最后错误';

CREATE TABLE IF NOT EXISTS `hg_youban_publish_profile_channel` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `tenant_id` bigint(20) NOT NULL DEFAULT '0',
  `account_id` bigint(20) NOT NULL DEFAULT '0',
  `profile_id` bigint(20) NOT NULL DEFAULT '0',
  `channel_id` bigint(20) NOT NULL DEFAULT '0',
  `is_manual` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否资料手动选择',
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

ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN `bot_collect_scope` varchar(16) NOT NULL DEFAULT 'chat' COMMENT 'Bot采集范围：chat/private' AFTER `bot_id`;
ALTER TABLE `hg_youban_publish_collect_source` ADD KEY `idx_ybp_collect_source_bot_scope` (`bot_id`,`bot_collect_scope`,`source_chat_id`,`status`);
