CREATE TABLE IF NOT EXISTS hg_youban_publish_collect_identity (
    id bigserial PRIMARY KEY,
    tenant_id bigint NOT NULL,
    account_id bigint NOT NULL,
    source_id bigint NOT NULL,
    rule_id bigint NOT NULL,
    event_id bigint NOT NULL,
    profile_id bigint NOT NULL,
    canonical_event_id bigint NOT NULL DEFAULT 0,
    source_chat_id varchar(128) NOT NULL DEFAULT '',
    match_layer varchar(32) NOT NULL DEFAULT '',
    created_at timestamp NULL,
    updated_at timestamp NULL,
    UNIQUE (tenant_id, account_id, source_id, rule_id, event_id)
);
CREATE INDEX IF NOT EXISTS idx_ybp_collect_identity_profile
    ON hg_youban_publish_collect_identity (tenant_id, account_id, source_id, rule_id, profile_id);
CREATE INDEX IF NOT EXISTS idx_ybp_collect_identity_canonical
    ON hg_youban_publish_collect_identity (tenant_id, account_id, source_id, rule_id, canonical_event_id);

CREATE INDEX IF NOT EXISTS idx_ybp_collect_identity_event ON hg_youban_publish_collect_identity (event_id, tenant_id, source_id);
