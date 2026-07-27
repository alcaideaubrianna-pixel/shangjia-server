CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_bot" (
  "id" bigserial PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "welcome_message" varchar(1000) NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_cooperation_config" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "two_way_bot_id" bigint NOT NULL DEFAULT 0,
  "notification_type" varchar(20) NOT NULL DEFAULT 'two_way',
  "review_required" smallint NOT NULL DEFAULT 1,
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybtwb_coop_config_tenant" ON "hg_youban_two_way_bot_cooperation_config" ("tenant_id") WHERE "deleted_at" IS NULL;

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_cooperation_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "config_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybtwb_coop_channel" ON "hg_youban_two_way_bot_cooperation_channel" ("config_id", "channel_id") WHERE "deleted_at" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybtwb_coop_channel_tenant" ON "hg_youban_two_way_bot_cooperation_channel" ("tenant_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_cooperation_blacklist" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "config_id" bigint NOT NULL DEFAULT 0,
  "applicant_tg_user_id" varchar(64) NOT NULL DEFAULT '',
  "applicant_username" varchar(128) NOT NULL DEFAULT '',
  "applicant_first_name" varchar(128) NOT NULL DEFAULT '',
  "applicant_last_name" varchar(128) NOT NULL DEFAULT '',
  "reason" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybtwb_coop_blacklist_user" ON "hg_youban_two_way_bot_cooperation_blacklist" ("config_id", "applicant_tg_user_id");
CREATE INDEX IF NOT EXISTS "idx_ybtwb_coop_blacklist_tenant" ON "hg_youban_two_way_bot_cooperation_blacklist" ("tenant_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_cooperation_application" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "config_id" bigint NOT NULL DEFAULT 0,
  "applicant_tg_user_id" varchar(64) NOT NULL DEFAULT '',
  "applicant_username" varchar(128) NOT NULL DEFAULT '',
  "applicant_first_name" varchar(128) NOT NULL DEFAULT '',
  "applicant_last_name" varchar(128) NOT NULL DEFAULT '',
  "submitted_bot_user_id" varchar(64) NOT NULL DEFAULT '',
  "submitted_bot_username" varchar(128) NOT NULL DEFAULT '',
  "submitted_bot_name" varchar(255) NOT NULL DEFAULT '',
  "review_status" varchar(24) NOT NULL DEFAULT 'pending',
  "join_status" varchar(24) NOT NULL DEFAULT 'not_started',
  "topic_thread_id" bigint NOT NULL DEFAULT 0,
  "reviewed_by" bigint NOT NULL DEFAULT 0,
  "review_remark" varchar(500) NOT NULL DEFAULT '',
  "error_message" text,
  "submitted_at" timestamp DEFAULT NULL,
  "reviewed_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybtwb_coop_app_tenant" ON "hg_youban_two_way_bot_cooperation_application" ("tenant_id", "review_status", "join_status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybtwb_coop_app_bot" ON "hg_youban_two_way_bot_cooperation_application" ("config_id", "submitted_bot_user_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_two_way_bot_cooperation_application_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "application_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(24) NOT NULL DEFAULT 'not_started',
  "error_message" text,
  "retry_count" integer NOT NULL DEFAULT 0,
  "joined_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybtwb_coop_app_channel" ON "hg_youban_two_way_bot_cooperation_application_channel" ("application_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybtwb_coop_app_channel_status" ON "hg_youban_two_way_bot_cooperation_application_channel" ("tenant_id", "status", "id");
INSERT INTO "hg_sys_addons_install" ("name","version","status","created_at","updated_at") VALUES ('youban_tg_bot_gateway','v1.0.0',1,NOW(),NOW()) ON CONFLICT ("name") DO UPDATE SET "version"=EXCLUDED."version","status"=1,"updated_at"=NOW();
