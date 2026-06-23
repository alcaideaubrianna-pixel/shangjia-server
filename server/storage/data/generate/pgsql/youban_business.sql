CREATE TABLE IF NOT EXISTS hg_app_announcement (
  id bigserial PRIMARY KEY,
  title varchar(255) NOT NULL,
  content text,
  is_banner smallint NOT NULL DEFAULT 0,
  banner_img varchar(500),
  banner_url varchar(500),
  publish_at timestamp,
  expire_at timestamp,
  sort integer NOT NULL DEFAULT 0,
  status smallint NOT NULL DEFAULT 1,
  created_by bigint,
  updated_by bigint,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_app_announcement_public ON hg_app_announcement (status, publish_at, expire_at, sort, id);
CREATE INDEX IF NOT EXISTS idx_app_announcement_banner ON hg_app_announcement (is_banner, status, publish_at, expire_at, sort, id);

CREATE TABLE IF NOT EXISTS hg_content_channel (
  id bigserial PRIMARY KEY,
  source_channel_id bigint NOT NULL,
  tg_chat_id varchar(128),
  title varchar(255),
  username varchar(255),
  invite_link varchar(512),
  source_type varchar(32) NOT NULL DEFAULT 'feiniu',
  public_status varchar(32) NOT NULL DEFAULT 'hidden',
  auth_status varchar(32) NOT NULL DEFAULT 'none',
  remark varchar(500),
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_channel_source ON hg_content_channel (source_type, source_channel_id);
CREATE INDEX IF NOT EXISTS idx_content_channel_status ON hg_content_channel (status, public_status);
CREATE INDEX IF NOT EXISTS idx_content_channel_source_query ON hg_content_channel (source_channel_id, id);
CREATE INDEX IF NOT EXISTS idx_content_channel_username ON hg_content_channel (username);

CREATE TABLE IF NOT EXISTS hg_content_profile (
  id bigserial PRIMARY KEY,
  profile_no varchar(64) NOT NULL,
  source_type varchar(32) NOT NULL DEFAULT 'feiniu',
  source_note_id bigint,
  source_note_uuid varchar(64),
  source_key varchar(255),
  source_text_hash varchar(64),
  channel_id bigint,
  duplicate_of_id bigint,
  title varchar(255),
  summary text,
  plain_text text,
  html_text text,
  source_category_code varchar(64),
  days_with_escort integer,
  expected_living_cost integer,
  can_fly_to_province smallint NOT NULL DEFAULT 0,
  can_go_abroad smallint NOT NULL DEFAULT 0,
  can_overnight smallint NOT NULL DEFAULT 0,
  can_cohabitate smallint NOT NULL DEFAULT 0,
  has_health_check smallint NOT NULL DEFAULT 0,
  is_full_month smallint NOT NULL DEFAULT 0,
  is_virgin smallint NOT NULL DEFAULT 0,
  accept_sm smallint NOT NULL DEFAULT 0,
  no_condom_after_check smallint NOT NULL DEFAULT 0,
  allow_creampie smallint NOT NULL DEFAULT 0,
  has_tattoo smallint NOT NULL DEFAULT 0,
  is_favorite smallint NOT NULL DEFAULT 0,
  source_edited_at timestamp,
  group_params text,
  tag_params text,
  text_block_count integer NOT NULL DEFAULT 0,
  storage_policy varchar(32),
  source_remark varchar(500),
  source_create_by varchar(64),
  source_update_by varchar(64),
  source_created_at timestamp,
  source_updated_at timestamp,
  province varchar(64),
  city varchar(64),
  age integer,
  height integer,
  weight integer,
  cup_size varchar(32),
  has_verification_video smallint NOT NULL DEFAULT 0,
  member_only_video smallint NOT NULL DEFAULT 1,
  cover_media_id bigint,
  image_count integer NOT NULL DEFAULT 0,
  video_count integer NOT NULL DEFAULT 0,
  visibility varchar(32) NOT NULL DEFAULT 'private',
  review_status varchar(32) NOT NULL DEFAULT 'approved',
  import_status varchar(32) NOT NULL DEFAULT 'imported',
  admin_remark varchar(500),
  published_at timestamp,
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_profile_no ON hg_content_profile (profile_no);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_profile_source_note ON hg_content_profile (source_type, source_note_id);
CREATE INDEX IF NOT EXISTS idx_content_profile_public ON hg_content_profile (status, visibility, review_status, published_at);
CREATE INDEX IF NOT EXISTS idx_content_profile_city ON hg_content_profile (province, city);
CREATE INDEX IF NOT EXISTS idx_content_profile_duplicate ON hg_content_profile (duplicate_of_id);
CREATE INDEX IF NOT EXISTS idx_content_profile_public_area ON hg_content_profile (status, review_status, import_status, visibility, province, city, published_at, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_public_latest_partial ON hg_content_profile (source_created_at DESC, source_note_id DESC, id DESC)
  WHERE status = 1
    AND review_status = 'approved'
    AND import_status IN ('imported', 'duplicate')
    AND visibility IN ('public', 'member_only')
    AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_content_profile_public_area_latest_partial ON hg_content_profile (province, city, source_created_at DESC, source_note_id DESC, id DESC)
  WHERE status = 1
    AND review_status = 'approved'
    AND import_status IN ('imported', 'duplicate')
    AND visibility IN ('public', 'member_only')
    AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_status ON hg_content_profile (review_status, visibility, import_status, status, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_created ON hg_content_profile (created_at, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_area ON hg_content_profile (province, city, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_video ON hg_content_profile (video_count, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_channel ON hg_content_profile (channel_id, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_age ON hg_content_profile (age, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_height ON hg_content_profile (height, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_weight ON hg_content_profile (weight, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_days ON hg_content_profile (days_with_escort, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_cost ON hg_content_profile (expected_living_cost, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_admin_flags ON hg_content_profile (can_fly_to_province, can_go_abroad, can_overnight, has_health_check, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_public_filters ON hg_content_profile (status, review_status, import_status, visibility, age, height, weight, cup_size, video_count, has_verification_video, id);
CREATE INDEX IF NOT EXISTS idx_content_profile_keyword ON hg_content_profile USING gin (to_tsvector('simple', coalesce(profile_no,'') || ' ' || coalesce(title,'') || ' ' || coalesce(summary,'') || ' ' || coalesce(plain_text,'') || ' ' || coalesce(province,'') || ' ' || coalesce(city,'') || ' ' || coalesce(cup_size,'')));

CREATE TABLE IF NOT EXISTS hg_content_media (
  id bigserial PRIMARY KEY,
  profile_id bigint NOT NULL,
  source_asset_id bigint,
  duplicate_of_media_id bigint,
  media_type varchar(16) NOT NULL,
  sort_index integer NOT NULL DEFAULT 0,
  original_storage_path varchar(1024),
  display_storage_path varchar(1024),
  preview_storage_path varchar(1024),
  binary_md5 varchar(64),
  perceptual_hash varchar(64),
  width integer,
  height integer,
  duration integer,
  process_status varchar(32) NOT NULL DEFAULT 'raw',
  encrypt_status varchar(32) NOT NULL DEFAULT 'none',
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_media_profile_asset ON hg_content_media (profile_id, source_asset_id);
CREATE INDEX IF NOT EXISTS idx_content_media_profile ON hg_content_media (profile_id, sort_index);
CREATE INDEX IF NOT EXISTS idx_content_media_duplicate ON hg_content_media (duplicate_of_media_id);
CREATE INDEX IF NOT EXISTS idx_content_media_md5 ON hg_content_media (binary_md5);
CREATE INDEX IF NOT EXISTS idx_content_media_phash ON hg_content_media (perceptual_hash);

CREATE TABLE IF NOT EXISTS hg_content_source_map (
  id bigserial PRIMARY KEY,
  profile_id bigint NOT NULL,
  source_type varchar(32) NOT NULL,
  source_key varchar(255) NOT NULL,
  source_channel_id bigint,
  source_message_id bigint,
  source_grouped_id bigint,
  source_text_hash varchar(64),
  raw_text text,
  raw_message_json text,
  created_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_source_key ON hg_content_source_map (source_key);
CREATE INDEX IF NOT EXISTS idx_content_source_profile ON hg_content_source_map (profile_id);
CREATE INDEX IF NOT EXISTS idx_content_source_text_hash ON hg_content_source_map (source_channel_id, source_text_hash);

CREATE TABLE IF NOT EXISTS hg_content_import_checkpoint (
  id bigserial PRIMARY KEY,
  source_name varchar(64) NOT NULL,
  last_source_note_id bigint NOT NULL DEFAULT 0,
  last_success_at timestamp,
  last_error varchar(500),
  created_at timestamp,
  updated_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_import_source ON hg_content_import_checkpoint (source_name);

CREATE TABLE IF NOT EXISTS hg_content_import_run (
  id bigserial PRIMARY KEY,
  source_name varchar(64) NOT NULL,
  trigger_type varchar(32) NOT NULL DEFAULT 'manual',
  batch_size integer NOT NULL DEFAULT 0,
  scanned integer NOT NULL DEFAULT 0,
  imported integer NOT NULL DEFAULT 0,
  duplicate integer NOT NULL DEFAULT 0,
  media_imported integer NOT NULL DEFAULT 0,
  last_source_note_id bigint NOT NULL DEFAULT 0,
  status varchar(32) NOT NULL DEFAULT 'running',
  error_message varchar(500),
  started_at timestamp,
  finished_at timestamp,
  cost_ms integer NOT NULL DEFAULT 0,
  created_at timestamp,
  updated_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_content_import_run_source_time ON hg_content_import_run (source_name, started_at);
CREATE INDEX IF NOT EXISTS idx_content_import_run_status ON hg_content_import_run (status, started_at);

CREATE TABLE IF NOT EXISTS hg_member_favorite (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL DEFAULT 0,
  profile_id bigint NOT NULL DEFAULT 0,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_member_favorite_active ON hg_member_favorite (member_id, profile_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_member_favorite_created ON hg_member_favorite (member_id, created_at);
CREATE INDEX IF NOT EXISTS idx_member_favorite_profile ON hg_member_favorite (profile_id);

CREATE TABLE IF NOT EXISTS hg_member_setting (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL DEFAULT 0,
  message_enabled smallint NOT NULL DEFAULT 1,
  hide_online smallint NOT NULL DEFAULT 0,
  hide_view_history smallint NOT NULL DEFAULT 1,
  match_chat_only smallint NOT NULL DEFAULT 1,
  profile_scope varchar(32) NOT NULL DEFAULT 'all',
  photo_scope varchar(32) NOT NULL DEFAULT 'verified',
  theme_mode varchar(16) NOT NULL DEFAULT 'system',
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_member_setting_active ON hg_member_setting (member_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS hg_member_profile_action (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL DEFAULT 0,
  profile_id bigint NOT NULL DEFAULT 0,
  action_type varchar(32) NOT NULL DEFAULT '',
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_member_profile_action_active ON hg_member_profile_action (member_id, profile_id, action_type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_member_action_created ON hg_member_profile_action (member_id, action_type, created_at);
CREATE INDEX IF NOT EXISTS idx_member_profile_action_profile ON hg_member_profile_action (profile_id);

CREATE TABLE IF NOT EXISTS hg_member_profile_view (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL DEFAULT 0,
  profile_id bigint NOT NULL DEFAULT 0,
  view_count integer NOT NULL DEFAULT 0,
  last_view_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_member_profile_view_active ON hg_member_profile_view (member_id, profile_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_member_last_view ON hg_member_profile_view (member_id, last_view_at);
CREATE INDEX IF NOT EXISTS idx_member_profile_view_profile ON hg_member_profile_view (profile_id);

CREATE TABLE IF NOT EXISTS hg_content_profile_stats (
  id bigserial PRIMARY KEY,
  profile_id bigint NOT NULL DEFAULT 0,
  view_total integer NOT NULL DEFAULT 0,
  view_24h integer NOT NULL DEFAULT 0,
  click_total integer NOT NULL DEFAULT 0,
  hot_score integer NOT NULL DEFAULT 0,
  last_view_at timestamp,
  created_at timestamp,
  updated_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_content_profile_stats_profile ON hg_content_profile_stats (profile_id);
CREATE INDEX IF NOT EXISTS idx_content_profile_stats_hot ON hg_content_profile_stats (hot_score, view_24h, profile_id);
CREATE INDEX IF NOT EXISTS idx_content_profile_stats_view ON hg_content_profile_stats (last_view_at, profile_id);

INSERT INTO hg_content_profile_stats (profile_id, view_total, view_24h, click_total, hot_score, last_view_at, created_at, updated_at)
SELECT profile_id,
       COALESCE(SUM(view_count), 0),
       COALESCE(SUM(view_count), 0),
       0,
       COALESCE(SUM(CASE WHEN last_view_at >= NOW() - INTERVAL '24 HOURS' THEN view_count ELSE 0 END), 0),
       MAX(last_view_at),
       NOW(),
       NOW()
FROM hg_member_profile_view
WHERE deleted_at IS NULL
GROUP BY profile_id
ON CONFLICT (profile_id) DO UPDATE SET
  view_total = EXCLUDED.view_total,
  hot_score = EXCLUDED.hot_score,
  view_24h = EXCLUDED.view_24h,
  last_view_at = EXCLUDED.last_view_at,
  updated_at = EXCLUDED.updated_at;

CREATE TABLE IF NOT EXISTS hg_member_share (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL DEFAULT 0,
  profile_id bigint NOT NULL DEFAULT 0,
  share_token varchar(64) NOT NULL DEFAULT '',
  visit_count integer NOT NULL DEFAULT 0,
  register_count integer NOT NULL DEFAULT 0,
  last_visit_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_share_token ON hg_member_share (share_token);
CREATE INDEX IF NOT EXISTS idx_member_share_member_profile ON hg_member_share (member_id, profile_id);
CREATE INDEX IF NOT EXISTS idx_member_share_profile ON hg_member_share (profile_id);

CREATE TABLE IF NOT EXISTS hg_member_vip (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL,
  level integer NOT NULL DEFAULT 1,
  status smallint NOT NULL DEFAULT 2,
  opened_at timestamp,
  expired_at timestamp,
  remark varchar(500) NOT NULL DEFAULT '',
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_member_vip_member_id ON hg_member_vip (member_id);
CREATE INDEX IF NOT EXISTS idx_member_vip_status_expired_at ON hg_member_vip (status, expired_at);
CREATE INDEX IF NOT EXISTS idx_member_vip_deleted_at ON hg_member_vip (deleted_at);

CREATE TABLE IF NOT EXISTS hg_member_vip_log (
  id bigserial PRIMARY KEY,
  member_id bigint NOT NULL,
  operator_id bigint NOT NULL DEFAULT 0,
  source varchar(32) NOT NULL DEFAULT '',
  action varchar(32) NOT NULL DEFAULT '',
  before_status smallint NOT NULL DEFAULT 2,
  after_status smallint NOT NULL DEFAULT 2,
  before_level integer NOT NULL DEFAULT 0,
  after_level integer NOT NULL DEFAULT 0,
  before_expired_at timestamp,
  after_expired_at timestamp,
  remark varchar(500) NOT NULL DEFAULT '',
  created_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_member_vip_log_member_id ON hg_member_vip_log (member_id);
CREATE INDEX IF NOT EXISTS idx_member_vip_log_operator_id ON hg_member_vip_log (operator_id);
CREATE INDEX IF NOT EXISTS idx_member_vip_log_source ON hg_member_vip_log (source);
CREATE INDEX IF NOT EXISTS idx_member_vip_log_created_at ON hg_member_vip_log (created_at);

UPDATE hg_content_media
SET display_storage_path = original_storage_path,
    updated_at = NOW()
WHERE media_type = 'video'
  AND deleted_at IS NULL
  AND COALESCE(original_storage_path, '') <> ''
  AND COALESCE(display_storage_path, '') <> original_storage_path;

INSERT INTO hg_sys_cron (group_id, title, name, params, pattern, policy, count, sort, remark, status, created_at, updated_at)
SELECT 1, 'FeiNiu 内容自动同步', 'content_import_feiniu', '', '0 */1 * * * *', 2, 0, 20, '每分钟从 FeiNiu_bot 增量同步最多 200 条资料', 2, NOW(), NOW()
WHERE EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'hg_sys_cron')
  AND NOT EXISTS (SELECT 1 FROM hg_sys_cron WHERE name = 'content_import_feiniu');
