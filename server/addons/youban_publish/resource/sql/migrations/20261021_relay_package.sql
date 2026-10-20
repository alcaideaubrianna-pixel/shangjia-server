-- MySQL additive migration; apply relay.sql first. Rerunnable.
SET @relay_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_relay_channel' AND column_name='access_hash'), 'SELECT 1', 'ALTER TABLE hg_youban_publish_relay_channel ADD COLUMN access_hash varchar(64) NOT NULL DEFAULT ''''');
PREPARE relay_stmt FROM @relay_ddl;
EXECUTE relay_stmt;
DEALLOCATE PREPARE relay_stmt;
SET @relay_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_relay_channel' AND column_name='bot_id'), 'SELECT 1', 'ALTER TABLE hg_youban_publish_relay_channel ADD COLUMN bot_id bigint NOT NULL DEFAULT 0');
PREPARE relay_stmt FROM @relay_ddl;
EXECUTE relay_stmt;
DEALLOCATE PREPARE relay_stmt;
SET @relay_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_relay_channel' AND column_name='capacity'), 'SELECT 1', 'ALTER TABLE hg_youban_publish_relay_channel ADD COLUMN capacity int NOT NULL DEFAULT 2');
PREPARE relay_stmt FROM @relay_ddl;
EXECUTE relay_stmt;
DEALLOCATE PREPARE relay_stmt;
SET @relay_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_relay_channel' AND column_name='operation_kind'), 'SELECT 1', 'ALTER TABLE hg_youban_publish_relay_channel ADD COLUMN operation_kind varchar(16) NOT NULL DEFAULT ''''');
PREPARE relay_stmt FROM @relay_ddl;
EXECUTE relay_stmt;
DEALLOCATE PREPARE relay_stmt;
SET @relay_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_relay_channel' AND column_name='operation_status'), 'SELECT 1', 'ALTER TABLE hg_youban_publish_relay_channel ADD COLUMN operation_status varchar(24) NOT NULL DEFAULT ''''');
PREPARE relay_stmt FROM @relay_ddl;
EXECUTE relay_stmt;
DEALLOCATE PREPARE relay_stmt;
SET @relay_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='hg_youban_publish_relay_channel' AND column_name='task_id'), 'SELECT 1', 'ALTER TABLE hg_youban_publish_relay_channel ADD COLUMN task_id bigint NOT NULL DEFAULT 0');
PREPARE relay_stmt FROM @relay_ddl;
EXECUTE relay_stmt;
DEALLOCATE PREPARE relay_stmt;
CREATE TABLE IF NOT EXISTS hg_youban_publish_relay_bot (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY, tenant_id bigint NOT NULL, relay_channel_id bigint NOT NULL,
 bot_id bigint NOT NULL, telegram_user_id bigint NOT NULL, status varchar(24) NOT NULL DEFAULT 'unknown',
 checked_at datetime NULL, last_error text NOT NULL, created_at datetime NULL, updated_at datetime NULL,
 UNIQUE KEY uk_relay_bot(tenant_id,relay_channel_id,telegram_user_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_publish_relay_package (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY, tenant_id bigint NOT NULL, job_id bigint NOT NULL,
 bot_id bigint NOT NULL, telegram_user_id bigint NOT NULL, relay_channel_id bigint NOT NULL DEFAULT 0,
 generation int NOT NULL DEFAULT 1, status varchar(24) NOT NULL DEFAULT 'pending',
 payload longtext NOT NULL, messages longtext NULL, attempt_id bigint NOT NULL DEFAULT 0,
 task_id bigint NOT NULL DEFAULT 0, lease_until datetime NULL, next_retry_at datetime NULL,
 last_error text NULL, created_at datetime NULL, updated_at datetime NULL,
 UNIQUE KEY uk_relay_job(job_id), KEY idx_relay_package_load(relay_channel_id,status,lease_until), KEY idx_relay_package_tenant_load(tenant_id,status,lease_until,relay_channel_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_publish_relay_file (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY, tenant_id bigint NOT NULL, telegram_user_id bigint NOT NULL,
 asset_key varchar(64) NOT NULL, file_id text NOT NULL, file_unique_id text NOT NULL,
 created_at datetime NULL, updated_at datetime NULL, UNIQUE KEY uk_relay_file(tenant_id,telegram_user_id,asset_key)
);
