ALTER TABLE hg_youban_bot_bot ADD COLUMN IF NOT EXISTS is_official smallint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_bot_bot ADD COLUMN IF NOT EXISTS is_default smallint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS hg_youban_bot_user (
  id bigserial PRIMARY KEY,
  bot_id bigint NOT NULL DEFAULT 0,
  telegram_user_id varchar(128) NOT NULL DEFAULT '',
  telegram_username varchar(128) NOT NULL DEFAULT '',
  telegram_first_name varchar(128) NOT NULL DEFAULT '',
  telegram_last_name varchar(128) NOT NULL DEFAULT '',
  chat_id varchar(128) NOT NULL DEFAULT '',
  chat_type varchar(32) NOT NULL DEFAULT '',
  chat_title varchar(255) NOT NULL DEFAULT '',
  message_count integer NOT NULL DEFAULT 0,
  last_message_text text,
  last_message_at timestamp,
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbu_bot_user ON hg_youban_bot_user (bot_id,telegram_user_id);
CREATE INDEX IF NOT EXISTS idx_ybbu_user ON hg_youban_bot_user (telegram_user_id);
CREATE INDEX IF NOT EXISTS idx_ybbu_last ON hg_youban_bot_user (bot_id,last_message_at,id);
CREATE TABLE IF NOT EXISTS hg_youban_bot_message (
  id bigserial PRIMARY KEY,
  bot_id bigint NOT NULL DEFAULT 0,
  telegram_user_id varchar(128) NOT NULL DEFAULT '',
  telegram_username varchar(128) NOT NULL DEFAULT '',
  chat_id varchar(128) NOT NULL DEFAULT '',
  chat_type varchar(32) NOT NULL DEFAULT '',
  message_id bigint NOT NULL DEFAULT 0,
  message_type varchar(32) NOT NULL DEFAULT '',
  text text,
  raw_json text,
  created_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_ybbm_bot ON hg_youban_bot_message (bot_id,id);
CREATE INDEX IF NOT EXISTS idx_ybbm_user ON hg_youban_bot_message (telegram_user_id,id);
CREATE INDEX IF NOT EXISTS idx_ybbm_message ON hg_youban_bot_message (bot_id,message_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbm_chat_message ON hg_youban_bot_message (chat_id,message_id);

CREATE TABLE IF NOT EXISTS hg_youban_bot_channel_cache (
  id bigserial PRIMARY KEY,
  bot_id bigint NOT NULL DEFAULT 0,
  chat_id varchar(128) NOT NULL DEFAULT '',
  chat_type varchar(32) NOT NULL DEFAULT '',
  chat_title varchar(255) NOT NULL DEFAULT '',
  chat_username varchar(128) NOT NULL DEFAULT '',
  is_broadcast smallint NOT NULL DEFAULT 0,
  is_megagroup smallint NOT NULL DEFAULT 0,
  message_count integer NOT NULL DEFAULT 0,
  last_message_text text,
  last_message_at timestamp,
  created_at timestamp,
  updated_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbcc_bot_chat ON hg_youban_bot_channel_cache (bot_id,chat_id);
CREATE INDEX IF NOT EXISTS idx_ybbcc_last ON hg_youban_bot_channel_cache (bot_id,last_message_at,id);

ALTER TABLE hg_youban_bot_bot ADD COLUMN IF NOT EXISTS run_mode varchar(32) NOT NULL DEFAULT 'auto';
ALTER TABLE hg_youban_bot_bot ADD COLUMN IF NOT EXISTS webhook_url varchar(500) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_bot_user ADD COLUMN IF NOT EXISTS is_super_admin smallint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS hg_youban_bot_invite_code (
  id bigserial PRIMARY KEY,
  code varchar(16) NOT NULL DEFAULT '',
  source varchar(16) NOT NULL DEFAULT 'web',
  inviter_app varchar(32) NOT NULL DEFAULT 'api',
  inviter_tenant_id bigint NOT NULL DEFAULT 0,
  inviter_account_id bigint NOT NULL DEFAULT 0,
  inviter_username varchar(128) NOT NULL DEFAULT '',
  inviter_nickname varchar(128) NOT NULL DEFAULT '',
  used_tenant_id bigint NOT NULL DEFAULT 0,
  used_account_id bigint NOT NULL DEFAULT 0,
  used_username varchar(128) NOT NULL DEFAULT '',
  status varchar(16) NOT NULL DEFAULT 'active',
  expires_at timestamp,
  used_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbic_code ON hg_youban_bot_invite_code (code);
CREATE INDEX IF NOT EXISTS idx_ybbic_inviter ON hg_youban_bot_invite_code (inviter_app,inviter_account_id,source,status,id);
CREATE INDEX IF NOT EXISTS idx_ybbic_status ON hg_youban_bot_invite_code (status,expires_at,id);

CREATE TABLE IF NOT EXISTS hg_youban_bot_profile_session (
  id bigserial PRIMARY KEY,
  bot_id bigint NOT NULL DEFAULT 0,
  telegram_user_id varchar(128) NOT NULL DEFAULT '',
  chat_id varchar(128) NOT NULL DEFAULT '',
  app varchar(32) NOT NULL DEFAULT 'api',
  account_id bigint NOT NULL DEFAULT 0,
  tenant_id bigint NOT NULL DEFAULT 0,
  scene varchar(32) NOT NULL DEFAULT '',
  step varchar(64) NOT NULL DEFAULT '',
  profile_id bigint NOT NULL DEFAULT 0,
  profile_no varchar(32) NOT NULL DEFAULT '',
  payload_json text,
  expires_at timestamp,
  status varchar(32) NOT NULL DEFAULT 'active',
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_ybbps_user ON hg_youban_bot_profile_session (bot_id,telegram_user_id,chat_id,status);
CREATE INDEX IF NOT EXISTS idx_ybbps_expire ON hg_youban_bot_profile_session (status,expires_at,id);

CREATE TABLE IF NOT EXISTS hg_youban_bot_inline_share (
  id bigserial PRIMARY KEY,
  bot_id bigint NOT NULL DEFAULT 0,
  token varchar(64) NOT NULL DEFAULT '',
  profile_id bigint NOT NULL DEFAULT 0,
  profile_no varchar(32) NOT NULL DEFAULT '',
  telegram_user_id varchar(128) NOT NULL DEFAULT '',
  account_id bigint NOT NULL DEFAULT 0,
  tenant_id bigint NOT NULL DEFAULT 0,
  usage_count integer NOT NULL DEFAULT 0,
  expires_at timestamp,
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbis_token ON hg_youban_bot_inline_share (token);
CREATE INDEX IF NOT EXISTS idx_ybbis_profile ON hg_youban_bot_inline_share (profile_no,status,id);
CREATE INDEX IF NOT EXISTS idx_ybbis_owner ON hg_youban_bot_inline_share (tenant_id,account_id,id);
