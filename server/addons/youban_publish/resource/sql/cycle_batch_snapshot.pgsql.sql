-- Apply explicitly before deploying snapshot producers; never run at startup.
BEGIN;
ALTER TABLE hg_youban_publish_cycle_run ADD COLUMN IF NOT EXISTS scheduled_day date;
CREATE TABLE IF NOT EXISTS hg_youban_publish_cycle_batch_item (
    run_id bigint NOT NULL,
    ordinal bigint NOT NULL CHECK (ordinal > 0),
    tenant_id bigint NOT NULL,
    account_id bigint NOT NULL,
    channel_id bigint NOT NULL,
    profile_id bigint NOT NULL,
    batch_order bigint NOT NULL,
    job_id bigint NOT NULL DEFAULT 0,
    item_state varchar(24) NOT NULL DEFAULT 'selected'
        CHECK (item_state IN ('selected','admitted','deferred','failed','skipped')),
    error_message text NOT NULL DEFAULT '',
    PRIMARY KEY (run_id, ordinal),
    UNIQUE (run_id, profile_id)
);
COMMENT ON COLUMN hg_youban_publish_cycle_batch_item.job_id IS
    'Actual admitted job, including reuse from another operation; immutable once attached';
COMMIT;
