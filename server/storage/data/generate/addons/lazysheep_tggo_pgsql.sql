CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_bot (
  id bigserial PRIMARY KEY,
  bot_key varchar(64) NOT NULL DEFAULT '',
  role varchar(32) NOT NULL DEFAULT 'user',
  member_id bigint DEFAULT 0,
  token varchar(255) NOT NULL DEFAULT '',
  bot_name varchar(128) DEFAULT '',
  username varchar(128) DEFAULT '',
  webhook_secret varchar(128) DEFAULT '',
  webhook_path varchar(255) DEFAULT '',
  enabled smallint DEFAULT 1,
  auto_pull smallint DEFAULT 0,
  auto_forward smallint DEFAULT 0,
  review_enabled smallint DEFAULT 1,
  allow_verify smallint DEFAULT 1,
  allow_location smallint DEFAULT 1,
  member_verify smallint DEFAULT 0,
  member_points integer DEFAULT 0,
  sign_follow smallint DEFAULT 0,
  sign_channels jsonb DEFAULT NULL,
  review_text text DEFAULT '',
  publish_text text DEFAULT '',
  sort integer DEFAULT 0,
  status smallint DEFAULT 1,
  created_by bigint DEFAULT 0,
  updated_by bigint DEFAULT 0,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_bot_key ON hg_addon_lazysheep_tggo_bot (bot_key);

CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_binding (
  id bigserial PRIMARY KEY,
  binding_key varchar(255) NOT NULL DEFAULT '',
  bot_id bigint NOT NULL DEFAULT 0,
  bot_key varchar(64) NOT NULL DEFAULT '',
  source_url varchar(500) NOT NULL DEFAULT '',
  source_token varchar(255) DEFAULT '',
  source_room_id bigint DEFAULT 0,
  source_pair_id varchar(64) DEFAULT '',
  review_chat_id bigint DEFAULT 0,
  publish_chat_id bigint DEFAULT 0,
  auto_push smallint DEFAULT 0,
  review_enabled smallint DEFAULT 1,
  publish_enabled smallint DEFAULT 1,
  verify_enabled smallint DEFAULT 1,
  location_enabled smallint DEFAULT 1,
  plugin_settings text DEFAULT '',
  last_pull_id bigint DEFAULT 0,
  last_cursor varchar(255) DEFAULT '',
  status smallint DEFAULT 1,
  created_by bigint DEFAULT 0,
  updated_by bigint DEFAULT 0,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_binding_key ON hg_addon_lazysheep_tggo_binding (binding_key);
CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_binding_bot_key ON hg_addon_lazysheep_tggo_binding (bot_key);

CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_user (
  id bigserial PRIMARY KEY,
  telegram_id bigint NOT NULL,
  bot_key varchar(64) DEFAULT '',
  member_id bigint DEFAULT 0,
  username varchar(128) DEFAULT '',
  first_name varchar(128) DEFAULT '',
  last_name varchar(128) DEFAULT '',
  language_code varchar(32) DEFAULT '',
  is_bot smallint DEFAULT 0,
  member_level integer DEFAULT 0,
  points decimal(10,2) DEFAULT 0.00,
  last_active_at timestamp DEFAULT NULL,
  status smallint DEFAULT 1,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_user_bot_telegram_id ON hg_addon_lazysheep_tggo_user (bot_key, telegram_id);

CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_note (
  id bigserial PRIMARY KEY,
  bot_id bigint DEFAULT 0,
  binding_id bigint DEFAULT 0,
  content_id bigint DEFAULT 0,
  up_id bigint DEFAULT 0,
  pair_id varchar(64) DEFAULT '',
  receiver_room_id bigint DEFAULT 0,
  room_name varchar(255) DEFAULT '',
  sender varchar(64) DEFAULT '',
  sender_dno varchar(128) DEFAULT '',
  sender_user jsonb DEFAULT NULL,
  raw_payload jsonb DEFAULT NULL,
  note_payload jsonb DEFAULT NULL,
  message_type varchar(64) DEFAULT 'MESSAGE_TYPE_NOTES',
  code varchar(16) DEFAULT '',
  title varchar(500) DEFAULT '',
  text_content text DEFAULT '',
  workflow_status smallint DEFAULT 1,
  review_message_id bigint DEFAULT 0,
  publish_message_id bigint DEFAULT 0,
  approved_by bigint DEFAULT 0,
  published_by bigint DEFAULT 0,
  approved_at timestamp DEFAULT NULL,
  published_at timestamp DEFAULT NULL,
  last_error text DEFAULT '',
  sort integer DEFAULT 0,
  status smallint DEFAULT 1,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  deleted_at timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_bot_content_id ON hg_addon_lazysheep_tggo_note (bot_id, content_id);
CREATE UNIQUE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_bot_code ON hg_addon_lazysheep_tggo_note (bot_id, code);
CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_bot_binding ON hg_addon_lazysheep_tggo_note (bot_id, binding_id);

CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_note_item (
  id bigserial PRIMARY KEY,
  note_id bigint NOT NULL DEFAULT 0,
  item_index integer DEFAULT 0,
  item_type varchar(64) NOT NULL DEFAULT '',
  title text DEFAULT '',
  sub_title text DEFAULT '',
  content text DEFAULT '',
  duration integer DEFAULT 0,
  aspect_ratio decimal(10,4) DEFAULT 0.0000,
  verify_video smallint DEFAULT 0,
  attachment_id bigint DEFAULT 0,
  preview_url varchar(500) DEFAULT '',
  local_path varchar(500) DEFAULT '',
  tg_file_id varchar(255) DEFAULT '',
  status smallint DEFAULT 1,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  deleted_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_item_note_id ON hg_addon_lazysheep_tggo_note_item (note_id);

CREATE TABLE IF NOT EXISTS hg_addon_lazysheep_tggo_note_asset (
  id bigserial PRIMARY KEY,
  note_id bigint NOT NULL DEFAULT 0,
  bot_id bigint NOT NULL DEFAULT 0,
  item_id bigint DEFAULT 0,
  asset_type varchar(32) NOT NULL DEFAULT '',
  source_url varchar(500) NOT NULL DEFAULT '',
  attachment_id bigint DEFAULT 0,
  preview_url varchar(500) DEFAULT '',
  local_path varchar(500) DEFAULT '',
  mime_type varchar(128) DEFAULT '',
  file_size bigint DEFAULT 0,
  duration integer DEFAULT 0,
  aspect_ratio decimal(10,4) DEFAULT 0.0000,
  tg_file_id varchar(255) DEFAULT '',
  convert_status smallint DEFAULT 1,
  sort integer DEFAULT 0,
  status smallint DEFAULT 1,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  deleted_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_asset_note_id ON hg_addon_lazysheep_tggo_note_asset (note_id);
CREATE INDEX IF NOT EXISTS hg_addon_lazysheep_tggo_note_asset_bot_id ON hg_addon_lazysheep_tggo_note_asset (bot_id);
