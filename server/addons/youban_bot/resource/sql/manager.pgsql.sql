CREATE TABLE IF NOT EXISTS hg_youban_bot_managed (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, account_id bigint NOT NULL,
 telegram_bot_id bigint NOT NULL, bot_name varchar(128) NOT NULL DEFAULT '',
 bot_username varchar(128) NOT NULL DEFAULT '', bot_token text NOT NULL DEFAULT '',
 publish_bot_id bigint NOT NULL DEFAULT 0, status smallint NOT NULL DEFAULT 1,
 created_at timestamptz, updated_at timestamptz, deleted_at timestamptz,
 token_owner_id bigint GENERATED ALWAYS AS (CASE WHEN bot_token<>'' AND deleted_at IS NULL THEN telegram_bot_id ELSE NULL END) STORED UNIQUE,
 UNIQUE(tenant_id,account_id,telegram_bot_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_bot_manager_state (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, account_id bigint NOT NULL,
 entry_bot_id bigint NOT NULL, telegram_user_id bigint NOT NULL,
 selected_channel_ids text NOT NULL DEFAULT '[]', selection_saved smallint NOT NULL DEFAULT 0,
 input_mode varchar(32) NOT NULL DEFAULT '', keyword varchar(128) NOT NULL DEFAULT '',
 draft_bot_id bigint NOT NULL DEFAULT 0, draft_rights_json varchar(500) NOT NULL DEFAULT '{}', revision bigint NOT NULL DEFAULT 0,
 synced_at timestamptz, created_at timestamptz, updated_at timestamptz,
 UNIQUE(entry_bot_id,telegram_user_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_bot_managed_channel (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, account_id bigint NOT NULL,
 publish_channel_id bigint NOT NULL, tg_account_id bigint NOT NULL,
 chat_id varchar(128) NOT NULL, title varchar(128) NOT NULL DEFAULT '',
 access_hash varchar(128) NOT NULL DEFAULT '', visit_url text NOT NULL DEFAULT '',
 admin_count integer NOT NULL DEFAULT -1, admin_limit integer NOT NULL DEFAULT 0,
 bot_count integer NOT NULL DEFAULT 0, can_manage smallint NOT NULL DEFAULT 0,
 sync_status varchar(32) NOT NULL DEFAULT 'unknown', error_message varchar(500) NOT NULL DEFAULT '',
 last_synced_at timestamptz, last_assigned_at timestamptz,
 created_at timestamptz, updated_at timestamptz,
 UNIQUE(tenant_id,account_id,publish_channel_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_bot_managed_member (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, account_id bigint NOT NULL,
 managed_bot_id bigint NOT NULL, channel_id bigint NOT NULL,
 member_status varchar(32) NOT NULL DEFAULT 'unknown', rights_json text NOT NULL DEFAULT '{}',
 last_synced_at timestamptz, created_at timestamptz, updated_at timestamptz,
 UNIQUE(managed_bot_id,channel_id)
);
CREATE TABLE IF NOT EXISTS hg_youban_bot_manager_task (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, account_id bigint NOT NULL,
 entry_bot_id bigint NOT NULL, telegram_user_id bigint NOT NULL,
 batch_key varchar(64) NOT NULL, action varchar(32) NOT NULL,
 managed_bot_id bigint NOT NULL DEFAULT 0, channel_id bigint NOT NULL DEFAULT 0,
 rights_json text NOT NULL DEFAULT '{}', status varchar(32) NOT NULL DEFAULT 'pending',
 account_task_id bigint NOT NULL DEFAULT 0, error_message varchar(500) NOT NULL DEFAULT '',
 notified smallint NOT NULL DEFAULT 0, created_at timestamptz, updated_at timestamptz,
 UNIQUE(batch_key,action,managed_bot_id,channel_id)
);
CREATE INDEX IF NOT EXISTS idx_ybb_manager_task_pending ON hg_youban_bot_manager_task(status,id);
CREATE INDEX IF NOT EXISTS idx_ybb_manager_task_batch ON hg_youban_bot_manager_task(batch_key,id);
CREATE INDEX IF NOT EXISTS idx_ybb_managed_channel_owner ON hg_youban_bot_managed_channel(tenant_id,account_id,id);
