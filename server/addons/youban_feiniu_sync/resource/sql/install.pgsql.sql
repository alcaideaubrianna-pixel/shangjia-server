
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_config (
  id bigserial PRIMARY KEY,
  name varchar(128) NOT NULL DEFAULT '',
  db_type varchar(32) NOT NULL DEFAULT 'mysql',
  db_host varchar(255) NOT NULL DEFAULT '',
  db_port integer NOT NULL DEFAULT 3306,
  db_name varchar(128) NOT NULL DEFAULT '',
  db_user varchar(128) NOT NULL DEFAULT '',
  db_password varchar(1024) NOT NULL DEFAULT '',
  target_tenant_id bigint NOT NULL DEFAULT 0,
  target_parent_account_id bigint NOT NULL DEFAULT 0,
  auto_create_account smallint NOT NULL DEFAULT 1,
  sync_media smallint NOT NULL DEFAULT 1,
  sync_verify_media smallint NOT NULL DEFAULT 1,
  auto_sync_enabled smallint NOT NULL DEFAULT 1,
  sync_interval_minutes integer NOT NULL DEFAULT 10,
  batch_size integer NOT NULL DEFAULT 100,
  last_source_note_id bigint NOT NULL DEFAULT 0,
  last_source_update_time timestamp DEFAULT NULL,
  last_source_update_note_id bigint NOT NULL DEFAULT 0,
  backfill_source_note_id bigint NOT NULL DEFAULT 0,
  backfill_target_note_id bigint NOT NULL DEFAULT 0,
  backfill_completed_at timestamp DEFAULT NULL,
  status smallint NOT NULL DEFAULT 1,
  last_run_at timestamp DEFAULT NULL,
  last_success_at timestamp DEFAULT NULL,
  last_error text,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  deleted_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_yfs_config_status ON hg_youban_feiniu_sync_config (status,id);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_channel_map (
  id bigserial PRIMARY KEY,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_tg_chat_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  feiniu_username varchar(255) NOT NULL DEFAULT '',
  youban_tenant_id bigint NOT NULL DEFAULT 0,
  youban_account_id bigint NOT NULL DEFAULT 0,
  youban_account_username varchar(128) NOT NULL DEFAULT '',
  last_source_update_time timestamp DEFAULT NULL,
  last_source_note_id bigint NOT NULL DEFAULT 0,
  sync_status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_channel UNIQUE (config_id,feiniu_channel_id)
);
CREATE INDEX IF NOT EXISTS idx_yfs_channel_chat ON hg_youban_feiniu_sync_channel_map (config_id,feiniu_tg_chat_id);
CREATE INDEX IF NOT EXISTS idx_yfs_channel_account ON hg_youban_feiniu_sync_channel_map (youban_tenant_id,youban_account_id);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_profile_map (
  id bigserial PRIMARY KEY,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_note_id bigint NOT NULL DEFAULT 0,
  feiniu_note_uuid varchar(64) NOT NULL DEFAULT '',
  feiniu_note_code varchar(32) NOT NULL DEFAULT '',
  feiniu_source_key varchar(255) NOT NULL DEFAULT '',
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_tg_chat_id bigint NOT NULL DEFAULT 0,
  youban_profile_id bigint NOT NULL DEFAULT 0,
  youban_account_id bigint NOT NULL DEFAULT 0,
  source_updated_at timestamp DEFAULT NULL,
  content_hash varchar(64) NOT NULL DEFAULT '',
  dedupe_key varchar(255) NOT NULL DEFAULT '',
  duplicate_profile_id bigint NOT NULL DEFAULT 0,
  sync_status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_profile UNIQUE (config_id,feiniu_note_id)
);
CREATE INDEX IF NOT EXISTS idx_yfs_profile_cursor ON hg_youban_feiniu_sync_profile_map (config_id,source_updated_at,feiniu_note_id);
CREATE INDEX IF NOT EXISTS idx_yfs_profile_status ON hg_youban_feiniu_sync_profile_map (config_id,sync_status,id);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_run (
  id bigserial PRIMARY KEY,
  config_id bigint NOT NULL DEFAULT 0,
  run_type varchar(32) NOT NULL DEFAULT 'manual',
  status varchar(32) NOT NULL DEFAULT 'running',
  total_count integer NOT NULL DEFAULT 0,
  created_count integer NOT NULL DEFAULT 0,
  updated_count integer NOT NULL DEFAULT 0,
  skipped_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  started_at timestamp DEFAULT NULL,
  finished_at timestamp DEFAULT NULL,
  error_message text,
  runtime_log text,
  created_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_yfs_run_config ON hg_youban_feiniu_sync_run (config_id,id);
CREATE INDEX IF NOT EXISTS idx_yfs_run_status ON hg_youban_feiniu_sync_run (status,id);

CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_daily_stat (
  id bigserial PRIMARY KEY,
  stat_date date NOT NULL,
  config_id bigint NOT NULL DEFAULT 0,
  run_count integer NOT NULL DEFAULT 0,
  total_count integer NOT NULL DEFAULT 0,
  success_count integer NOT NULL DEFAULT 0,
  created_count integer NOT NULL DEFAULT 0,
  updated_count integer NOT NULL DEFAULT 0,
  skipped_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  channel_count integer NOT NULL DEFAULT 0,
  profile_count integer NOT NULL DEFAULT 0,
  avg_duration_ms bigint NOT NULL DEFAULT 0,
  last_run_id bigint NOT NULL DEFAULT 0,
  last_run_at timestamp DEFAULT NULL,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_daily UNIQUE (config_id,stat_date)
);
CREATE INDEX IF NOT EXISTS idx_yfs_daily_date ON hg_youban_feiniu_sync_daily_stat (stat_date);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_channel_daily_stat (
  id bigserial PRIMARY KEY,
  stat_date date NOT NULL,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_tg_chat_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  youban_account_id bigint NOT NULL DEFAULT 0,
  youban_account_username varchar(128) NOT NULL DEFAULT '',
  total_count integer NOT NULL DEFAULT 0,
  created_count integer NOT NULL DEFAULT 0,
  updated_count integer NOT NULL DEFAULT 0,
  skipped_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  last_note_id bigint NOT NULL DEFAULT 0,
  last_source_update_time timestamp DEFAULT NULL,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_channel_daily UNIQUE (config_id,stat_date,feiniu_channel_id)
);
CREATE INDEX IF NOT EXISTS idx_yfs_channel_daily_rank ON hg_youban_feiniu_sync_channel_daily_stat (config_id,stat_date,total_count);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_run_item (
  id bigserial PRIMARY KEY,
  run_id bigint NOT NULL DEFAULT 0,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_note_id bigint NOT NULL DEFAULT 0,
  feiniu_note_code varchar(32) NOT NULL DEFAULT '',
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  youban_profile_id bigint NOT NULL DEFAULT 0,
  action varchar(32) NOT NULL DEFAULT '',
  status varchar(32) NOT NULL DEFAULT '',
  error_message text,
  source_updated_at timestamp DEFAULT NULL,
  duration_ms bigint NOT NULL DEFAULT 0,
  created_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_run ON hg_youban_feiniu_sync_run_item (run_id,id);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_config ON hg_youban_feiniu_sync_run_item (config_id,created_at);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_status ON hg_youban_feiniu_sync_run_item (config_id,status,created_at);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_channel ON hg_youban_feiniu_sync_run_item (config_id,feiniu_channel_id,created_at);
