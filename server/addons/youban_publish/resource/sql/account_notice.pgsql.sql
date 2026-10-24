CREATE TABLE IF NOT EXISTS hg_youban_publish_account_notice (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,account_id bigint NOT NULL,
 event_key varchar(128) NOT NULL,category varchar(32) NOT NULL DEFAULT 'account',text_content text NOT NULL,
 status varchar(16) NOT NULL DEFAULT 'pending',attempts integer NOT NULL DEFAULT 0,
 next_retry_at timestamp,lease_until timestamp,lease_token varchar(64) NOT NULL DEFAULT '',
 error_message varchar(512) NOT NULL DEFAULT '',created_at timestamp,updated_at timestamp,
 UNIQUE(event_key,account_id)
);
CREATE INDEX IF NOT EXISTS idx_ybp_account_notice_due ON hg_youban_publish_account_notice(status,next_retry_at,id);
CREATE INDEX IF NOT EXISTS idx_ybp_account_notice_created ON hg_youban_publish_account_notice(created_at,id);
