-- Additive migration. Apply before enabling YOUBAN_RELAY_PACKAGE_ENABLED.
ALTER TABLE hg_youban_publish_relay_channel
 ADD COLUMN IF NOT EXISTS access_hash varchar(64) NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS bot_id bigint NOT NULL DEFAULT 0,
 ADD COLUMN IF NOT EXISTS capacity integer NOT NULL DEFAULT 2,
 ADD COLUMN IF NOT EXISTS operation_kind varchar(16) NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS operation_status varchar(24) NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS task_id bigint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS hg_youban_publish_relay_bot (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, relay_channel_id bigint NOT NULL,
 bot_id bigint NOT NULL, telegram_user_id bigint NOT NULL, status varchar(24) NOT NULL DEFAULT 'unknown',
 checked_at timestamp, last_error text NOT NULL DEFAULT '', created_at timestamp, updated_at timestamp,
 UNIQUE(tenant_id, relay_channel_id, telegram_user_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_publish_relay_package (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, job_id bigint NOT NULL UNIQUE,
 bot_id bigint NOT NULL, telegram_user_id bigint NOT NULL, relay_channel_id bigint NOT NULL DEFAULT 0,
 generation integer NOT NULL DEFAULT 1, status varchar(24) NOT NULL DEFAULT 'pending',
 payload text NOT NULL DEFAULT '', messages text NOT NULL DEFAULT '', attempt_id bigint NOT NULL DEFAULT 0,
 task_id bigint NOT NULL DEFAULT 0, lease_until timestamp, next_retry_at timestamp,
 last_error text NOT NULL DEFAULT '', created_at timestamp, updated_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_ybp_relay_package_load ON hg_youban_publish_relay_package(relay_channel_id,status,lease_until);
CREATE INDEX IF NOT EXISTS idx_ybp_relay_package_tenant_load ON hg_youban_publish_relay_package(tenant_id,status,lease_until,relay_channel_id);
CREATE TABLE IF NOT EXISTS hg_youban_publish_relay_file (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, telegram_user_id bigint NOT NULL,
 asset_key varchar(64) NOT NULL, file_id text NOT NULL, file_unique_id text NOT NULL DEFAULT '',
 created_at timestamp, updated_at timestamp, UNIQUE(tenant_id,telegram_user_id,asset_key)
);
