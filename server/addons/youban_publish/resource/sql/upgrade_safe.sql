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
