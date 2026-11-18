CREATE TABLE IF NOT EXISTS hg_youban_publish_collect_identity (
    id bigint unsigned NOT NULL AUTO_INCREMENT,
    tenant_id bigint NOT NULL,
    account_id bigint NOT NULL,
    source_id bigint NOT NULL,
    rule_id bigint NOT NULL,
    event_id bigint NOT NULL,
    profile_id bigint NOT NULL,
    canonical_event_id bigint NOT NULL DEFAULT 0,
    source_chat_id varchar(128) NOT NULL DEFAULT '',
    match_layer varchar(32) NOT NULL DEFAULT '',
    created_at datetime NULL,
    updated_at datetime NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_ybp_collect_identity_branch (tenant_id, account_id, source_id, rule_id, event_id),
    KEY idx_ybp_collect_identity_event (event_id, tenant_id, source_id),
    KEY idx_ybp_collect_identity_profile (tenant_id, account_id, source_id, rule_id, profile_id),
    KEY idx_ybp_collect_identity_canonical (tenant_id, account_id, source_id, rule_id, canonical_event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
