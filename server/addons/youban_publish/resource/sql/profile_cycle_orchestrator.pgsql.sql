-- 资料级循环批次：同一资料一轮循环的全部频道共享 run_id/due_at。
CREATE TABLE IF NOT EXISTS hg_youban_publish_profile_cycle_run (
    id bigserial PRIMARY KEY,
    tenant_id bigint NOT NULL DEFAULT 0,
    account_id bigint NOT NULL DEFAULT 0,
    profile_id bigint NOT NULL DEFAULT 0,
    run_key varchar(128) NOT NULL DEFAULT '',
    status varchar(32) NOT NULL DEFAULT 'pending',
    due_at timestamp without time zone NOT NULL,
    claimed_at timestamp without time zone,
    completed_at timestamp without time zone,
    error_message text NOT NULL DEFAULT '',
    created_at timestamp without time zone,
    updated_at timestamp without time zone,
    CONSTRAINT uk_ybp_profile_cycle_run_key UNIQUE (run_key)
);

CREATE INDEX IF NOT EXISTS idx_ybp_profile_cycle_run_due
    ON hg_youban_publish_profile_cycle_run (status, due_at, id);
CREATE INDEX IF NOT EXISTS idx_ybp_profile_cycle_run_profile
    ON hg_youban_publish_profile_cycle_run (tenant_id, account_id, profile_id, id);

CREATE TABLE IF NOT EXISTS hg_youban_publish_profile_cycle_run_channel (
    id bigserial PRIMARY KEY,
    run_id bigint NOT NULL,
    tenant_id bigint NOT NULL DEFAULT 0,
    account_id bigint NOT NULL DEFAULT 0,
    profile_id bigint NOT NULL DEFAULT 0,
    channel_id bigint NOT NULL DEFAULT 0,
    job_id bigint NOT NULL DEFAULT 0,
    status varchar(32) NOT NULL DEFAULT 'pending',
    error_message text NOT NULL DEFAULT '',
    created_at timestamp without time zone,
    updated_at timestamp without time zone,
    CONSTRAINT uk_ybp_profile_cycle_run_channel UNIQUE (run_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_ybp_profile_cycle_run_channel_status
    ON hg_youban_publish_profile_cycle_run_channel (run_id, status, id);
CREATE INDEX IF NOT EXISTS idx_ybp_profile_cycle_run_channel_profile
    ON hg_youban_publish_profile_cycle_run_channel (tenant_id, account_id, profile_id, id);
