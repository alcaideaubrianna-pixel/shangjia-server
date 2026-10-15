CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_config (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL UNIQUE, tenant_id bigint NOT NULL DEFAULT 0, account_id bigint NOT NULL DEFAULT 0, source_bot_id bigint NOT NULL DEFAULT 0, owner_id bigint NOT NULL, enabled integer NOT NULL DEFAULT 0, block_links integer NOT NULL DEFAULT 0, audit_database integer NOT NULL DEFAULT 1, audit_group_id varchar(64) NOT NULL DEFAULT '',
 created_at timestamp, updated_at timestamp
);

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_admin (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL, user_id varchar(64) NOT NULL DEFAULT '', username varchar(128) NOT NULL DEFAULT '',
 created_at timestamp, updated_at timestamp, UNIQUE(bot_id,username)
);

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_binding (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL, pair_key varchar(140) NOT NULL, left_chat_id varchar(64) NOT NULL, right_chat_id varchar(64) NOT NULL, left_name varchar(255) NOT NULL, right_name varchar(255) NOT NULL,
 created_at timestamp, updated_at timestamp, UNIQUE(bot_id,pair_key)
);

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_endpoint (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL, binding_id bigint NOT NULL, chat_id varchar(64) NOT NULL,
 created_at timestamp, updated_at timestamp, UNIQUE(bot_id,chat_id)
);

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_rule (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL, kind varchar(16) NOT NULL, value varchar(255) NOT NULL, label varchar(255) NOT NULL,
 created_at timestamp, updated_at timestamp, UNIQUE(bot_id,kind,value)
);

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_audit_topic (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL, group_id varchar(64) NOT NULL, pair_key varchar(140) NOT NULL, thread_id bigint NOT NULL DEFAULT 0,
 created_at timestamp, updated_at timestamp, UNIQUE(bot_id,group_id,pair_key)
);

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_delivery (
 id bigserial PRIMARY KEY,
 bot_id bigint NOT NULL, binding_id bigint NOT NULL DEFAULT 0, source_chat_id varchar(64) NOT NULL, source_message_id bigint NOT NULL, media_group_id varchar(128) NOT NULL DEFAULT '', target_chat_id varchar(64) NOT NULL DEFAULT '', kind varchar(16) NOT NULL, pair_key varchar(140) NOT NULL DEFAULT '', topic_title varchar(128) NOT NULL DEFAULT '', raw_json text NOT NULL, status varchar(16) NOT NULL DEFAULT 'pending', attempts integer NOT NULL DEFAULT 0, available_at timestamp, lease_token varchar(64) NOT NULL DEFAULT '', target_message_id bigint NOT NULL DEFAULT 0, error_message varchar(1000) NOT NULL DEFAULT '',
 created_at timestamp, updated_at timestamp, UNIQUE(bot_id,source_chat_id,source_message_id,kind,target_chat_id)
);

CREATE INDEX IF NOT EXISTS idx_ybb_relay_delivery_ready ON hg_youban_bot_relay_delivery (status,available_at,id);

ALTER TABLE hg_youban_bot_relay_config ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_bot_relay_config ADD COLUMN IF NOT EXISTS account_id bigint NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybb_relay_account ON hg_youban_bot_relay_config(tenant_id,account_id) WHERE tenant_id > 0 AND account_id > 0;
UPDATE hg_youban_bot_relay_config SET enabled=0 WHERE (tenant_id=0 OR account_id=0) AND enabled<>0;

ALTER TABLE hg_youban_bot_relay_config ADD COLUMN IF NOT EXISTS source_bot_id bigint NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybb_relay_source_bot ON hg_youban_bot_relay_config(source_bot_id) WHERE source_bot_id > 0;

UPDATE hg_youban_bot_relay_config SET enabled=0 WHERE source_bot_id=0 AND enabled<>0;
