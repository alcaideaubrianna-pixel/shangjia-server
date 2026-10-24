ALTER TABLE hg_youban_publish_account_setting ADD COLUMN daily_report_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_account_setting ADD COLUMN channel_alert_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_account_setting ADD COLUMN account_event_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_account_setting ADD COLUMN task_failure_enabled smallint NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS hg_youban_publish_daily_report (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,tenant_id bigint NOT NULL,account_id bigint NOT NULL,report_date date NOT NULL,
 collected_total integer NOT NULL DEFAULT 0,publish_success integer NOT NULL DEFAULT 0,publish_failed integer NOT NULL DEFAULT 0,publish_processing integer NOT NULL DEFAULT 0,
 source_breakdown_json longtext,generated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE KEY uk_ybp_daily_report_owner(tenant_id,account_id,report_date)
);
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_status varchar(16) NOT NULL DEFAULT 'pending';
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_retry_count integer NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_next_retry_at datetime NULL;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_lease_until datetime NULL;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_token varchar(64) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_cursor integer NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN notify_error varchar(512) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_daily_report ADD KEY idx_ybp_daily_report_delivery(notify_status,notify_next_retry_at,id);
