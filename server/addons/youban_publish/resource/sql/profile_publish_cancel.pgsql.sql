-- Apply before enabling unified single-profile publish cancellation.
-- This is an intent/tombstone, not a second Job queue. Existing TG Outbox
-- workers perform deletion; the existing publish recovery role settles it.
CREATE TABLE IF NOT EXISTS hg_youban_publish_profile_cancel (
    id bigserial PRIMARY KEY,
    tenant_id bigint NOT NULL,
    account_id bigint NOT NULL,
    profile_id bigint NOT NULL,
    uuid varchar(128) NOT NULL DEFAULT '',
    operation_no varchar(128) NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'canceling',
    error_message text NOT NULL DEFAULT '',
    next_check_at timestamp NOT NULL DEFAULT LOCALTIMESTAMP,
    created_at timestamp NOT NULL DEFAULT LOCALTIMESTAMP,
    updated_at timestamp NOT NULL DEFAULT LOCALTIMESTAMP,
    UNIQUE (tenant_id, account_id, profile_id, operation_no),
    CHECK (status IN ('canceling', 'canceled'))
);
CREATE INDEX IF NOT EXISTS idx_ybp_profile_cancel_due
    ON hg_youban_publish_profile_cancel(next_check_at, id) WHERE status = 'canceling';
