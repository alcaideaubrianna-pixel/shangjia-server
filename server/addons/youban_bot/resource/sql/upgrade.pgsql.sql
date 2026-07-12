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

ALTER TABLE hg_youban_bot_bot ADD COLUMN IF NOT EXISTS run_mode varchar(32) NOT NULL DEFAULT 'auto';
ALTER TABLE hg_youban_bot_bot ADD COLUMN IF NOT EXISTS webhook_url varchar(500) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_bot_user ADD COLUMN IF NOT EXISTS is_super_admin smallint NOT NULL DEFAULT 0;
