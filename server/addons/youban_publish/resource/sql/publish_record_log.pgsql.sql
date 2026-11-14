CREATE TABLE IF NOT EXISTS hg_youban_publish_record_log (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL,
    task_id BIGINT NOT NULL DEFAULT 0,
    tenant_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL DEFAULT 0,
    profile_id BIGINT NOT NULL DEFAULT 0,
    channel_id BIGINT NOT NULL DEFAULT 0,
    bot_id BIGINT NOT NULL DEFAULT 0,
    operation_no VARCHAR(255) NOT NULL DEFAULT '',
    target_chat_id VARCHAR(128) NOT NULL DEFAULT '',
    action VARCHAR(64) NOT NULL DEFAULT '',
    title VARCHAR(512) NOT NULL DEFAULT '',
    account_name VARCHAR(255) NOT NULL DEFAULT '',
    tenant_name VARCHAR(255) NOT NULL DEFAULT '',
    channel_title VARCHAR(255) NOT NULL DEFAULT '',
    channel_username VARCHAR(255) NOT NULL DEFAULT '',
    bot_name VARCHAR(255) NOT NULL DEFAULT '',
    bot_username VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    message VARCHAR(255) NOT NULL DEFAULT '',
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_ybp_record_log_job UNIQUE (job_id)
);
CREATE INDEX IF NOT EXISTS idx_ybp_record_log_tenant_id ON hg_youban_publish_record_log (tenant_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_ybp_record_log_tenant_account_id ON hg_youban_publish_record_log (tenant_id, account_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_ybp_record_log_tenant_status_id ON hg_youban_publish_record_log (tenant_id, status, id DESC);
