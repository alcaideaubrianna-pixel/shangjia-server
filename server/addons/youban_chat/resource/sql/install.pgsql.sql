CREATE TABLE IF NOT EXISTS hg_youban_chat_visitor (
  id bigserial PRIMARY KEY,
  app_id varchar(128) NOT NULL,
  external_user_id varchar(128) NOT NULL,
  name varchar(128) NOT NULL DEFAULT '',
  email varchar(255) NOT NULL DEFAULT '',
  avatar_url varchar(500) NOT NULL DEFAULT '',
  created_at timestamp,
  updated_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcv_app_user ON hg_youban_chat_visitor (app_id, external_user_id);

CREATE TABLE IF NOT EXISTS hg_youban_chat_conversation (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL,
  profile_id bigint NOT NULL,
  pocketping_session_id varchar(128) NOT NULL DEFAULT '',
  chatwoot_contact_id bigint NOT NULL DEFAULT 0,
  chatwoot_conversation_id bigint NOT NULL DEFAULT 0,
  tg_chat_id varchar(128),
  tg_message_thread_id bigint NOT NULL DEFAULT 0,
  bot_id bigint NOT NULL DEFAULT 0,
  last_message varchar(500),
  last_message_at timestamp,
  unread_count integer NOT NULL DEFAULT 0,
  pinned_at timestamp,
  hidden_before_at timestamp,
  status varchar(32) NOT NULL DEFAULT 'opened',
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybc_member_profile ON hg_youban_chat_conversation (member_id, profile_id);
CREATE INDEX IF NOT EXISTS idx_ybc_pocketping_session ON hg_youban_chat_conversation (pocketping_session_id);
CREATE INDEX IF NOT EXISTS idx_ybc_member_updated ON hg_youban_chat_conversation (member_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_ybc_profile ON hg_youban_chat_conversation (profile_id);
CREATE INDEX IF NOT EXISTS idx_ybc_tg_thread ON hg_youban_chat_conversation (tg_chat_id, tg_message_thread_id);
ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS pocketping_session_id varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS routing_rule_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS assigned_operator_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS bot_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS pinned_at timestamp;
ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS hidden_before_at timestamp;
CREATE INDEX IF NOT EXISTS idx_ybc_member_pinned_updated ON hg_youban_chat_conversation (member_id, pinned_at, updated_at);

CREATE TABLE IF NOT EXISTS hg_youban_chat_message (
  id bigserial PRIMARY KEY,
  conversation_id bigint NOT NULL,
  pocketping_message_id varchar(128) NOT NULL DEFAULT '',
  direction varchar(16) NOT NULL DEFAULT 'mine',
  content text,
  content_type varchar(32) NOT NULL DEFAULT 'text',
  status varchar(32) NOT NULL DEFAULT 'sent',
  sender_name varchar(128),
  attachments_json text,
  reply_to_message_id bigint NOT NULL DEFAULT 0,
  reactions_json text,
  tg_chat_id varchar(128) NOT NULL DEFAULT '',
  tg_message_thread_id bigint NOT NULL DEFAULT 0,
  tg_message_id bigint NOT NULL DEFAULT 0,
  read_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcm_pocketping_message ON hg_youban_chat_message (pocketping_message_id);
CREATE INDEX IF NOT EXISTS idx_ybcm_conversation_id ON hg_youban_chat_message (conversation_id, id);
CREATE INDEX IF NOT EXISTS idx_ybcm_created_at ON hg_youban_chat_message (created_at);
CREATE INDEX IF NOT EXISTS idx_ybcm_tg_message ON hg_youban_chat_message (tg_chat_id, tg_message_id);
ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS tg_chat_id varchar(128) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS tg_message_thread_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS tg_message_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS read_at timestamp;
ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS reply_to_message_id bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS reactions_json text;

CREATE TABLE IF NOT EXISTS hg_youban_chat_bot (
  id bigserial PRIMARY KEY,
  bot_name varchar(128) NOT NULL DEFAULT '',
  bot_username varchar(128) NOT NULL DEFAULT '',
  bot_token varchar(255) NOT NULL DEFAULT '',
  remark varchar(255) NOT NULL DEFAULT '',
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_ybcb_username ON hg_youban_chat_bot (bot_username);
CREATE INDEX IF NOT EXISTS idx_ybcb_status ON hg_youban_chat_bot (status);
ALTER TABLE hg_youban_chat_bot ADD COLUMN IF NOT EXISTS app_id varchar(128) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ybcb_app_id ON hg_youban_chat_bot (app_id);

CREATE TABLE IF NOT EXISTS hg_youban_chat_binding (
  id bigserial PRIMARY KEY,
  bind_code varchar(64) NOT NULL DEFAULT '',
  bind_type varchar(32) NOT NULL DEFAULT 'channel',
  source_channel_id bigint NOT NULL DEFAULT 0,
  content_channel_id bigint NOT NULL DEFAULT 0,
  bot_id bigint NOT NULL DEFAULT 0,
  tg_chat_id varchar(128) NOT NULL DEFAULT '',
  tg_chat_title varchar(255) NOT NULL DEFAULT '',
  remark varchar(255) NOT NULL DEFAULT '',
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcb_bind_code ON hg_youban_chat_binding (bind_code);
CREATE INDEX IF NOT EXISTS idx_ybcb_source_channel ON hg_youban_chat_binding (source_channel_id, status);
CREATE INDEX IF NOT EXISTS idx_ybcb_content_channel ON hg_youban_chat_binding (content_channel_id, status);
CREATE INDEX IF NOT EXISTS idx_ybcb_global ON hg_youban_chat_binding (bind_type, status);
ALTER TABLE hg_youban_chat_binding ADD COLUMN IF NOT EXISTS app_id varchar(128) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ybcb_binding_app_id ON hg_youban_chat_binding (app_id);

CREATE TABLE IF NOT EXISTS hg_youban_chat_binding_channel (
  id bigserial PRIMARY KEY,
  binding_id bigint NOT NULL DEFAULT 0,
  channel_id bigint NOT NULL DEFAULT 0,
  created_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcbc_binding_channel ON hg_youban_chat_binding_channel (binding_id, channel_id);
CREATE INDEX IF NOT EXISTS idx_ybcbc_channel ON hg_youban_chat_binding_channel (channel_id);

CREATE TABLE IF NOT EXISTS hg_youban_chat_operator (
  id bigserial PRIMARY KEY,
  admin_member_id bigint NOT NULL DEFAULT 0,
  telegram_user_id varchar(128) NOT NULL DEFAULT '',
  telegram_username varchar(128) NOT NULL DEFAULT '',
  display_name varchar(128) NOT NULL DEFAULT '',
  bind_code varchar(64) NOT NULL DEFAULT '',
  remark varchar(255) NOT NULL DEFAULT '',
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_ybco_admin_member ON hg_youban_chat_operator (admin_member_id);
CREATE INDEX IF NOT EXISTS idx_ybco_telegram_user ON hg_youban_chat_operator (telegram_user_id);

CREATE TABLE IF NOT EXISTS hg_youban_chat_feature (
  id bigserial PRIMARY KEY,
  feature_key varchar(64) NOT NULL DEFAULT '',
  name varchar(128) NOT NULL DEFAULT '',
  command varchar(64) NOT NULL DEFAULT '',
  description varchar(255) NOT NULL DEFAULT '',
  config_json text,
  sort integer NOT NULL DEFAULT 0,
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcf_feature_key ON hg_youban_chat_feature (feature_key);
CREATE INDEX IF NOT EXISTS idx_ybcf_status_sort ON hg_youban_chat_feature (status, sort);
