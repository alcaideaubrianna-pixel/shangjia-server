ALTER TABLE hg_youban_publish_account_setting ADD COLUMN IF NOT EXISTS daily_report_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_account_setting ADD COLUMN IF NOT EXISTS channel_alert_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_account_setting ADD COLUMN IF NOT EXISTS account_event_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_publish_account_setting ADD COLUMN IF NOT EXISTS task_failure_enabled smallint NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS hg_youban_publish_daily_report (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,account_id bigint NOT NULL,report_date date NOT NULL,
 collected_total integer NOT NULL DEFAULT 0,publish_success integer NOT NULL DEFAULT 0,publish_failed integer NOT NULL DEFAULT 0,publish_processing integer NOT NULL DEFAULT 0,
 source_breakdown_json text NOT NULL DEFAULT '{}',generated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(tenant_id,account_id,report_date)
);
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_status varchar(16) NOT NULL DEFAULT 'pending';
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_retry_count integer NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_next_retry_at timestamp;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_lease_until timestamp;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_token varchar(64) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_cursor integer NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_daily_report ADD COLUMN IF NOT EXISTS notify_error varchar(512) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ybp_daily_report_delivery ON hg_youban_publish_daily_report (notify_status,notify_next_retry_at,id);
