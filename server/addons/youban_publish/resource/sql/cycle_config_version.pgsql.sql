ALTER TABLE hg_youban_publish_channel ADD COLUMN IF NOT EXISTS cycle_config_version bigint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_channel_profile ADD COLUMN IF NOT EXISTS applied_cycle_config_version bigint NOT NULL DEFAULT 0;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ybp_channel_profile_cycle_version ON hg_youban_publish_channel_profile (channel_id,status,applied_cycle_config_version,id);
