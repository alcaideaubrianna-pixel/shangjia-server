CREATE TABLE IF NOT EXISTS `hg_tg_collector_source` (
    `id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `tenant_id` BIGINT NOT NULL DEFAULT 0,
    `account_id` BIGINT NOT NULL DEFAULT 0,
    `bot_id` BIGINT NOT NULL DEFAULT 0,
    `source_type` VARCHAR(32) NOT NULL DEFAULT 'account',
    `chat_id` VARCHAR(64) NOT NULL DEFAULT '',
    `chat_title` VARCHAR(255) NOT NULL DEFAULT '',
    `chat_username` VARCHAR(255) NOT NULL DEFAULT '',
    `status` VARCHAR(32) NOT NULL DEFAULT 'enabled',
    `history_enabled` TINYINT NOT NULL DEFAULT 0,
    `history_cursor` TEXT NOT NULL,
    `created_at` DATETIME NULL,
    `updated_at` DATETIME NULL,
    UNIQUE KEY `uq_tg_collector_source_identity` (`tenant_id`, `source_type`, `chat_id`, `account_id`, `bot_id`),
    KEY `idx_tg_collector_source_status` (`tenant_id`, `status`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_tg_collector_event` (
    `id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `tenant_id` BIGINT NOT NULL DEFAULT 0,
    `source_id` BIGINT NOT NULL DEFAULT 0,
    `source_type` VARCHAR(32) NOT NULL DEFAULT '',
    `bot_key` VARCHAR(255) NOT NULL DEFAULT '',
    `account_id` BIGINT NOT NULL DEFAULT 0,
    `chat_id` VARCHAR(64) NOT NULL DEFAULT '',
    `message_id` BIGINT NOT NULL DEFAULT 0,
    `update_id` BIGINT NOT NULL DEFAULT 0,
    `event_key` VARCHAR(255) NOT NULL,
    `raw_update` LONGTEXT NOT NULL,
    `priority` INT NOT NULL DEFAULT 0,
    `status` VARCHAR(32) NOT NULL DEFAULT 'received',
    `attempt_count` INT NOT NULL DEFAULT 0,
    `next_run_at` DATETIME NULL,
    `lease_owner` VARCHAR(128) NOT NULL DEFAULT '',
    `lease_until` DATETIME NULL,
    `received_at` DATETIME NULL,
    `processed_at` DATETIME NULL,
    `error_message` TEXT NOT NULL,
    `created_at` DATETIME NULL,
    `updated_at` DATETIME NULL,
    UNIQUE KEY `uq_tg_collector_event_key` (`tenant_id`, `event_key`),
    KEY `idx_tg_collector_event_task` (`status`, `priority`, `next_run_at`, `lease_until`, `id`),
    KEY `idx_tg_collector_event_source_chat` (`source_id`, `chat_id`, `status`, `next_run_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_tg_collector_media` (
    `id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `tenant_id` BIGINT NOT NULL DEFAULT 0,
    `fingerprint` VARCHAR(128) NOT NULL,
    `kind` VARCHAR(32) NOT NULL DEFAULT '',
    `mime_type` VARCHAR(128) NOT NULL DEFAULT '',
    `size` BIGINT NOT NULL DEFAULT 0,
    `pipeline_version` VARCHAR(64) NOT NULL DEFAULT 'v1',
    `status` VARCHAR(32) NOT NULL DEFAULT 'processing',
    `storage_path` TEXT NOT NULL,
    `poster_storage_path` TEXT NOT NULL,
    `phash` VARCHAR(128) NOT NULL,
    `dhash` VARCHAR(128) NOT NULL,
    `attempt_count` INT NOT NULL DEFAULT 0,
    `next_run_at` DATETIME NULL,
    `lease_owner` VARCHAR(128) NOT NULL DEFAULT '',
    `lease_until` DATETIME NULL,
    `error_message` TEXT NOT NULL,
    `created_at` DATETIME NULL,
    `updated_at` DATETIME NULL,
    UNIQUE KEY `uq_tg_collector_media_fingerprint` (`tenant_id`, `fingerprint`, `pipeline_version`),
    KEY `idx_tg_collector_media_task` (`status`, `next_run_at`, `lease_until`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `hg_tg_collector_delivery` (
    `id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `tenant_id` BIGINT NOT NULL DEFAULT 0,
    `event_id` BIGINT NOT NULL DEFAULT 0,
    `delivery_key` VARCHAR(255) NOT NULL,
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending',
    `priority` INT NOT NULL DEFAULT 0,
    `attempt_count` INT NOT NULL DEFAULT 0,
    `next_run_at` DATETIME NULL,
    `lease_owner` VARCHAR(128) NOT NULL DEFAULT '',
    `lease_until` DATETIME NULL,
    `error_message` TEXT NOT NULL,
    `created_at` DATETIME NULL,
    `updated_at` DATETIME NULL,
    UNIQUE KEY `uq_tg_collector_delivery_key` (`tenant_id`, `delivery_key`),
    KEY `idx_tg_collector_delivery_task` (`status`, `priority`, `next_run_at`, `lease_until`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE `hg_tg_collector_delivery` DROP COLUMN IF EXISTS `payload`;

CREATE TABLE IF NOT EXISTS `hg_tg_collector_account_task` (
    `id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `tenant_id` BIGINT NOT NULL DEFAULT 0,
    `account_id` BIGINT NOT NULL DEFAULT 0,
    `task_type` VARCHAR(64) NOT NULL DEFAULT '',
    `task_key` VARCHAR(255) NOT NULL,
    `priority` INT NOT NULL DEFAULT 0,
    `status` VARCHAR(32) NOT NULL DEFAULT 'pending',
    `history_task_id` BIGINT NOT NULL DEFAULT 0,
    `media_owner_account_id` BIGINT NOT NULL DEFAULT 0,
    `media_type` VARCHAR(32) NOT NULL DEFAULT '',
    `media_purpose` VARCHAR(32) NOT NULL DEFAULT '',
    `source_file_id` TEXT NOT NULL,
    `file_url` TEXT NOT NULL,
    `storage_path` TEXT NOT NULL,
    `poster_url` TEXT NOT NULL,
    `file_md5` VARCHAR(128) NOT NULL DEFAULT '',
    `file_phash` VARCHAR(128) NOT NULL DEFAULT '',
    `source_kind` VARCHAR(32) NOT NULL DEFAULT '',
    `source_media_id` BIGINT NOT NULL DEFAULT 0,
    `source_access_hash` BIGINT NOT NULL DEFAULT 0,
    `source_file_reference` BLOB NULL,
    `source_thumb_size` VARCHAR(32) NOT NULL DEFAULT '',
    `source_mime_type` VARCHAR(128) NOT NULL DEFAULT '',
    `source_dc_id` INT NOT NULL DEFAULT 0,
    `source_size` BIGINT NOT NULL DEFAULT 0,
    `debug_meta_text` TEXT NOT NULL,
    `attachment_id` BIGINT NOT NULL DEFAULT 0,
    `result_error_code` VARCHAR(64) NOT NULL DEFAULT '',
    `attempt_count` INT NOT NULL DEFAULT 0,
    `max_attempts` INT NOT NULL DEFAULT 5,
    `next_run_at` DATETIME NULL,
    `lease_owner` VARCHAR(128) NOT NULL DEFAULT '',
    `lease_epoch` BIGINT NOT NULL DEFAULT 0,
    `lease_until` DATETIME NULL,
    `error_message` TEXT NOT NULL,
    `completed_at` DATETIME NULL,
    `created_at` DATETIME NULL,
    `updated_at` DATETIME NULL,
    UNIQUE KEY `uq_tg_collector_account_task_key` (`tenant_id`, `task_key`),
    KEY `idx_tg_collector_account_task_claim` (`account_id`, `status`, `priority`, `next_run_at`, `lease_until`, `id`),
    KEY `idx_tg_collector_account_task_recovery` (`status`, `lease_until`, `next_run_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `history_task_id` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `media_owner_account_id` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `media_type` VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `media_purpose` VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_file_id` TEXT NOT NULL;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `file_url` TEXT NOT NULL;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `storage_path` TEXT NOT NULL;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `poster_url` TEXT NOT NULL;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `file_md5` VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `file_phash` VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_kind` VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_media_id` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_access_hash` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_file_reference` BLOB NULL;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_thumb_size` VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_mime_type` VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_dc_id` INT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `source_size` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `debug_meta_text` TEXT NOT NULL;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `attachment_id` BIGINT NOT NULL DEFAULT 0;
ALTER TABLE `hg_tg_collector_account_task` ADD COLUMN IF NOT EXISTS `result_error_code` VARCHAR(64) NOT NULL DEFAULT '';
DELETE FROM `hg_tg_collector_account_task` WHERE (`task_type`='history_page' AND `history_task_id`<=0) OR (`task_type`='media_download' AND `source_file_id`='');
ALTER TABLE `hg_tg_collector_account_task` DROP COLUMN IF EXISTS `payload`;
ALTER TABLE `hg_tg_collector_account_task` DROP COLUMN IF EXISTS `result`;
