-- Explicit migration: run before enabling youbanPublish.profileOriginEnabled.
-- No business backfill, no foreign key cascade, no startup DDL.
CREATE TABLE IF NOT EXISTS hg_youban_publish_profile_origin (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 tenant_id bigint NOT NULL,
 account_id bigint NOT NULL,
 profile_id bigint NOT NULL,
 origin_key varchar(160) NOT NULL,
 origin_kind varchar(24) NOT NULL DEFAULT 'collect',
 collect_method varchar(24) NOT NULL DEFAULT 'unknown',
 source_id bigint NOT NULL DEFAULT 0,
 event_id bigint NOT NULL DEFAULT 0,
 bot_id bigint NOT NULL DEFAULT 0,
 tg_account_id bigint NOT NULL DEFAULT 0,
 follow_account_id bigint NOT NULL DEFAULT 0,
 chat_id_raw varchar(128) NOT NULL DEFAULT '',
 chat_id varchar(128) NOT NULL DEFAULT '',
 message_id bigint NOT NULL DEFAULT 0,
 grouped_id varchar(128) NOT NULL DEFAULT '',
 channel_title varchar(255) NOT NULL DEFAULT '',
 channel_username varchar(128) NOT NULL DEFAULT '',
 received_at datetime NULL,
 evidence_kind varchar(32) NOT NULL DEFAULT 'event',
 completeness varchar(16) NOT NULL DEFAULT 'partial',
 created_at datetime NULL,
 KEY idx_ybp_profile_origin_message (message_id,chat_id,profile_id),
 KEY idx_ybp_profile_origin_lookup (profile_id,tenant_id,account_id,id),
 UNIQUE (tenant_id,account_id,profile_id,origin_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
