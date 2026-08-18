-- Backfill schema for installations created before external visitor support.
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
