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
ALTER TABLE hg_youban_bot_message ADD COLUMN IF NOT EXISTS retained_at timestamp;
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

DROP INDEX IF EXISTS uk_ybbab_account;
DROP INDEX IF EXISTS uk_ybbab_telegram;
CREATE UNIQUE INDEX uk_ybbab_account ON hg_youban_bot_account_bind (app,account_id) WHERE status=1 AND deleted_at IS NULL;
CREATE UNIQUE INDEX uk_ybbab_telegram ON hg_youban_bot_account_bind (app,telegram_user_id) WHERE status=1 AND deleted_at IS NULL;

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
  registration_telegram_user_id varchar(128) NOT NULL DEFAULT '',
  registration_telegram_username varchar(128) NOT NULL DEFAULT '',
  registration_telegram_first_name varchar(128) NOT NULL DEFAULT '',
  registration_telegram_last_name varchar(128) NOT NULL DEFAULT '',
  registration_bot_id bigint NOT NULL DEFAULT 0,
  registration_bound_at timestamp,
  status varchar(16) NOT NULL DEFAULT 'active',
  expires_at timestamp,
  used_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
ALTER TABLE hg_youban_bot_invite_code ADD COLUMN IF NOT EXISTS registration_telegram_user_id varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_bot_invite_code ADD COLUMN IF NOT EXISTS registration_telegram_username varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_bot_invite_code ADD COLUMN IF NOT EXISTS registration_telegram_first_name varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_bot_invite_code ADD COLUMN IF NOT EXISTS registration_telegram_last_name varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_bot_invite_code ADD COLUMN IF NOT EXISTS registration_bot_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_bot_invite_code ADD COLUMN IF NOT EXISTS registration_bound_at timestamp;
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbic_code ON hg_youban_bot_invite_code (code);
CREATE INDEX IF NOT EXISTS idx_ybbic_inviter ON hg_youban_bot_invite_code (inviter_app,inviter_account_id,source,status,id);
CREATE INDEX IF NOT EXISTS idx_ybbic_status ON hg_youban_bot_invite_code (status,expires_at,id);
CREATE INDEX IF NOT EXISTS idx_ybbic_self_register ON hg_youban_bot_invite_code (source,registration_telegram_user_id,status,expires_at,id);

CREATE TABLE IF NOT EXISTS hg_youban_bot_invite_usage (
  id bigserial PRIMARY KEY,
  invite_id bigint NOT NULL DEFAULT 0,
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
  used_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbiu_used_tenant ON hg_youban_bot_invite_usage (used_tenant_id);
CREATE INDEX IF NOT EXISTS idx_ybbiu_invite ON hg_youban_bot_invite_usage (invite_id,id);
CREATE INDEX IF NOT EXISTS idx_ybbiu_inviter ON hg_youban_bot_invite_usage (inviter_tenant_id,used_at,id);
INSERT INTO hg_youban_bot_invite_usage (invite_id,code,source,inviter_app,inviter_tenant_id,inviter_account_id,inviter_username,inviter_nickname,used_tenant_id,used_account_id,used_username,used_at,created_at,updated_at)
SELECT id,code,source,inviter_app,inviter_tenant_id,inviter_account_id,inviter_username,inviter_nickname,used_tenant_id,used_account_id,used_username,used_at,COALESCE(used_at,updated_at,created_at),COALESCE(used_at,updated_at,created_at)
FROM hg_youban_bot_invite_code WHERE source IN ('web','bot') AND used_tenant_id>0
ON CONFLICT (used_tenant_id) DO NOTHING;
UPDATE hg_youban_bot_invite_code SET expires_at=COALESCE(created_at,updated_at,CURRENT_TIMESTAMP)+INTERVAL '7 days',updated_at=CURRENT_TIMESTAMP WHERE source IN ('web','bot') AND expires_at IS NULL;
UPDATE hg_youban_bot_invite_code SET status='active',updated_at=CURRENT_TIMESTAMP WHERE source IN ('web','bot') AND status='used' AND (expires_at IS NULL OR expires_at>CURRENT_TIMESTAMP);
UPDATE hg_youban_bot_invite_code SET status='expired',updated_at=CURRENT_TIMESTAMP WHERE source IN ('web','bot') AND expires_at IS NOT NULL AND expires_at<=CURRENT_TIMESTAMP AND status<>'expired';

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

CREATE TABLE IF NOT EXISTS hg_youban_bot_custom_emoji (
  id bigserial PRIMARY KEY,
  custom_emoji_id varchar(64) NOT NULL DEFAULT '',
  file_unique_id varchar(128) NOT NULL DEFAULT '',
  attachment_id bigint NOT NULL DEFAULT 0,
  storage_path varchar(500) NOT NULL DEFAULT '',
  file_url varchar(1000) NOT NULL DEFAULT '',
  file_format varchar(16) NOT NULL DEFAULT '',
  render_type varchar(16) NOT NULL DEFAULT '',
  fallback_emoji varchar(64) NOT NULL DEFAULT '',
  width integer NOT NULL DEFAULT 0,
  height integer NOT NULL DEFAULT 0,
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybbce_emoji ON hg_youban_bot_custom_emoji (custom_emoji_id);
CREATE INDEX IF NOT EXISTS idx_ybbce_file ON hg_youban_bot_custom_emoji (file_unique_id);
CREATE INDEX IF NOT EXISTS idx_ybbce_status ON hg_youban_bot_custom_emoji (status,id);

CREATE TABLE IF NOT EXISTS hg_youban_bot_broadcast_task (
  id bigserial PRIMARY KEY,
  text text NOT NULL,
  disable_notice smallint NOT NULL DEFAULT 0,
  status varchar(32) NOT NULL DEFAULT 'pending',
  total_count integer NOT NULL DEFAULT 0,
  success_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  blocked_count integer NOT NULL DEFAULT 0,
  last_error text,
  created_at timestamp,
  started_at timestamp,
  finished_at timestamp,
  updated_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_ybbbt_status ON hg_youban_bot_broadcast_task (status,id);

CREATE TABLE IF NOT EXISTS hg_youban_bot_broadcast_task_bot (
  id bigserial PRIMARY KEY,
  task_id bigint NOT NULL,
  bot_id bigint NOT NULL,
  created_at timestamp,
  CONSTRAINT uk_ybbtb_task_bot UNIQUE (task_id,bot_id),
  CONSTRAINT chk_ybbtb_bot_positive CHECK (bot_id > 0)
);
DELETE FROM hg_youban_bot_broadcast_task_bot WHERE bot_id <= 0;
CREATE INDEX IF NOT EXISTS idx_ybbtb_bot ON hg_youban_bot_broadcast_task_bot (bot_id,task_id);
ALTER TABLE hg_youban_bot_broadcast_task DROP COLUMN IF EXISTS bot_ids_json;

CREATE TABLE IF NOT EXISTS hg_youban_bot_broadcast_recipient (
  id bigserial PRIMARY KEY,
  task_id bigint NOT NULL,
  bot_id bigint NOT NULL,
  telegram_user_id varchar(128) NOT NULL DEFAULT '',
  telegram_username varchar(255) NOT NULL DEFAULT '',
  telegram_first_name varchar(255) NOT NULL DEFAULT '',
  telegram_last_name varchar(255) NOT NULL DEFAULT '',
  chat_id varchar(128) NOT NULL DEFAULT '',
  status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  sent_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  CONSTRAINT uk_ybbr_task_user UNIQUE (task_id,telegram_user_id)
);
CREATE INDEX IF NOT EXISTS idx_ybbr_task_status ON hg_youban_bot_broadcast_recipient (task_id,status,id);
