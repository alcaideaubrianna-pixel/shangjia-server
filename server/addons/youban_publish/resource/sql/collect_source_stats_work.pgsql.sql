-- Explicit migration before deploying source statistics workers. No startup DDL.
CREATE TABLE IF NOT EXISTS hg_youban_publish_collect_source_stats_work (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL UNIQUE,
    generation VARCHAR(64) NOT NULL,
    pending SMALLINT NOT NULL DEFAULT 1,
    available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_collect_source_stats_work_due
    ON hg_youban_publish_collect_source_stats_work (available_at, source_id)
    WHERE pending=1;
