CREATE TABLE IF NOT EXISTS hg_youban_publish_account_notice (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,tenant_id bigint NOT NULL,account_id bigint NOT NULL,
 event_key varchar(128) NOT NULL,category varchar(32) NOT NULL DEFAULT 'account',text_content longtext NOT NULL,
 status varchar(16) NOT NULL DEFAULT 'pending',attempts integer NOT NULL DEFAULT 0,
 next_retry_at datetime NULL,lease_until datetime NULL,lease_token varchar(64) NOT NULL DEFAULT '',
 error_message varchar(512) NOT NULL DEFAULT '',created_at datetime,updated_at datetime,
 UNIQUE KEY uk_ybp_account_notice_event(event_key,account_id),
 KEY idx_ybp_account_notice_due(status,next_retry_at,id),KEY idx_ybp_account_notice_created(created_at,id)
);
