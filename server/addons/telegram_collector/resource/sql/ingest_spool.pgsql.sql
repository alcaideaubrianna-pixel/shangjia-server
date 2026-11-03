-- Phase 1 durable Telegram acknowledgement boundary.
-- Run outside a transaction where the deployment tool requires it.
CREATE TABLE IF NOT EXISTS hg_tg_collector_ingest_spool (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    event_key VARCHAR(255) NOT NULL,
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL DEFAULT 0,
    priority INT NOT NULL DEFAULT 0,
    payload JSONB NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'received',
    attempt_count INT NOT NULL DEFAULT 0,
    next_run_at TIMESTAMP NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_tg_collector_ingest_spool_event UNIQUE (tenant_id,event_key)
);
ALTER TABLE hg_tg_collector_ingest_spool ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_tg_collector_ingest_spool_due
 ON hg_tg_collector_ingest_spool (status,priority DESC,next_run_at,id)
 WHERE status IN ('received','failed_retry');
CREATE INDEX IF NOT EXISTS idx_tg_collector_ingest_spool_lease
 ON hg_tg_collector_ingest_spool (next_run_at,id)
 WHERE status='processing';
