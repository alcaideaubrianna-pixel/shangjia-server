ALTER TABLE hg_youban_publish_bot ADD COLUMN IF NOT EXISTS require_publish_review smallint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS hg_youban_publish_profile_review (
 id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, bot_id bigint NOT NULL,
 owner_account_id bigint NOT NULL, account_id bigint NOT NULL, profile_id bigint NOT NULL,
 profile_no varchar(128) NOT NULL, title varchar(512) NOT NULL, applicant varchar(256) NOT NULL,
 chat_id varchar(32) NOT NULL, status varchar(16) NOT NULL DEFAULT 'pending',
 created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ybp_profile_review_profile ON hg_youban_publish_profile_review (profile_id, tenant_id, id);
