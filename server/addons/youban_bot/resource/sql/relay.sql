CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_config (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL UNIQUE, tenant_id bigint NOT NULL DEFAULT 0, account_id bigint NOT NULL DEFAULT 0, enabled integer NOT NULL DEFAULT 0, block_links integer NOT NULL DEFAULT 0, audit_database integer NOT NULL DEFAULT 1, audit_group_id varchar(64) NOT NULL DEFAULT '',
 created_at datetime, updated_at datetime
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_admin (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL, user_id varchar(64) NOT NULL DEFAULT '', username varchar(128) NOT NULL DEFAULT '',
 created_at datetime, updated_at datetime, UNIQUE(bot_id,username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_binding (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL, pair_key varchar(140) NOT NULL, left_chat_id varchar(64) NOT NULL, right_chat_id varchar(64) NOT NULL, left_name varchar(255) NOT NULL, right_name varchar(255) NOT NULL,
 created_at datetime, updated_at datetime, UNIQUE(bot_id,pair_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_endpoint (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL, binding_id bigint NOT NULL, chat_id varchar(64) NOT NULL,
 created_at datetime, updated_at datetime, UNIQUE(bot_id,chat_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_rule (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL, kind varchar(16) NOT NULL, value varchar(255) NOT NULL, label varchar(255) NOT NULL,
 created_at datetime, updated_at datetime, UNIQUE(bot_id,kind,value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_audit_topic (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL, group_id varchar(64) NOT NULL, pair_key varchar(140) NOT NULL, thread_id bigint NOT NULL DEFAULT 0,
 created_at datetime, updated_at datetime, UNIQUE(bot_id,group_id,pair_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hg_youban_bot_relay_delivery (
 id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
 bot_id bigint NOT NULL, binding_id bigint NOT NULL DEFAULT 0, source_chat_id varchar(64) NOT NULL, source_message_id bigint NOT NULL, media_group_id varchar(128) NOT NULL DEFAULT '', target_chat_id varchar(64) NOT NULL DEFAULT '', kind varchar(16) NOT NULL, pair_key varchar(140) NOT NULL DEFAULT '', topic_title varchar(128) NOT NULL DEFAULT '', raw_json text NOT NULL, status varchar(16) NOT NULL DEFAULT 'pending', attempts integer NOT NULL DEFAULT 0, available_at datetime, lease_token varchar(64) NOT NULL DEFAULT '', target_message_id bigint NOT NULL DEFAULT 0, error_message varchar(1000) NOT NULL DEFAULT '',
 created_at datetime, updated_at datetime, UNIQUE(bot_id,source_chat_id,source_message_id,kind,target_chat_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_ybb_relay_delivery_ready ON hg_youban_bot_relay_delivery (status,available_at,id);
