CREATE TABLE IF NOT EXISTS hg_youban_publish_channel_permission_circuit (
    tenant_id bigint NOT NULL,
    channel_id bigint NOT NULL,
    paused smallint NOT NULL DEFAULT 0,
    generation varchar(32) NOT NULL DEFAULT '',
    account_id bigint NOT NULL DEFAULT 0,
    last_job_id bigint NOT NULL DEFAULT 0,
    resumed_at datetime NULL,
    notified smallint NOT NULL DEFAULT 0,
    created_at datetime NULL,
    updated_at datetime NULL,
    PRIMARY KEY (tenant_id, channel_id)
);
