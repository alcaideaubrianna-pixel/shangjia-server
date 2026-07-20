CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_bot" (
  "id" bigserial PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "bot_token" varchar(255) NOT NULL DEFAULT '',
  "bot_user_id" varchar(64) NOT NULL DEFAULT '',
  "bot_username" varchar(128) NOT NULL DEFAULT '',
  "supergroup_id" varchar(64) NOT NULL DEFAULT '',
  "supergroup_access_hash" varchar(128) NOT NULL DEFAULT '',
  "supergroup_title" varchar(128) NOT NULL DEFAULT '',
  "invite_link" varchar(512) NOT NULL DEFAULT '',
  "setup_status" varchar(32) NOT NULL DEFAULT 'pending',
  "webhook_status" varchar(32) NOT NULL DEFAULT 'pending',
  "status" integer NOT NULL DEFAULT 1,
  "error_message" varchar(1024) NOT NULL DEFAULT '',
  "last_setup_at" timestamp,
  "last_webhook_at" timestamp,
  "created_at" timestamp,
  "updated_at" timestamp,
  "deleted_at" timestamp
);

CREATE INDEX IF NOT EXISTS "idx_ybtwb_bot_tenant" ON "hg_youban_two_way_bot_bot" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybtwb_bot_tg_account" ON "hg_youban_two_way_bot_bot" ("tenant_id", "tg_account_id");

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_topic" (
  "id" bigserial PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "telegram_user_id" varchar(64) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "telegram_first_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_last_name" varchar(128) NOT NULL DEFAULT '',
  "thread_id" bigint NOT NULL DEFAULT 0,
  "title" varchar(128) NOT NULL DEFAULT '',
  "closed" integer NOT NULL DEFAULT 0,
  "last_message_at" timestamp,
  "created_at" timestamp,
  "updated_at" timestamp,
  "deleted_at" timestamp
);

CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybtwb_topic_user" ON "hg_youban_two_way_bot_topic" ("tenant_id", "bot_id", "telegram_user_id");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybtwb_topic_thread" ON "hg_youban_two_way_bot_topic" ("tenant_id", "bot_id", "thread_id");
CREATE INDEX IF NOT EXISTS "idx_ybtwb_topic_last" ON "hg_youban_two_way_bot_topic" ("tenant_id", "bot_id", "last_message_at");

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_message" (
  "id" bigserial PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "direction" varchar(16) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(64) NOT NULL DEFAULT '',
  "thread_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(64) NOT NULL DEFAULT '',
  "source_message_id" integer NOT NULL DEFAULT 0,
  "target_chat_id" varchar(64) NOT NULL DEFAULT '',
  "target_message_id" integer NOT NULL DEFAULT 0,
  "media_group_id" varchar(128) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'sent',
  "error_message" varchar(1024) NOT NULL DEFAULT '',
  "created_at" timestamp,
  "updated_at" timestamp
);

CREATE INDEX IF NOT EXISTS "idx_ybtwb_msg_topic" ON "hg_youban_two_way_bot_message" ("tenant_id", "bot_id", "thread_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybtwb_msg_user" ON "hg_youban_two_way_bot_message" ("tenant_id", "bot_id", "telegram_user_id", "id");
