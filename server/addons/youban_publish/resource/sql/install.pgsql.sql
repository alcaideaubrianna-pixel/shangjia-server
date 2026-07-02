CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant" (
  "id" BIGSERIAL PRIMARY KEY,
  "name" varchar(128) NOT NULL DEFAULT '',
  "contact_name" varchar(128) NOT NULL DEFAULT '',
  "contact_phone" varchar(64) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_tenant_status" ON "hg_youban_publish_tenant" ("status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_merchant" (
  "id" BIGSERIAL PRIMARY KEY,
  "name" varchar(128) NOT NULL DEFAULT '',
  "contact_name" varchar(128) NOT NULL DEFAULT '',
  "contact_phone" varchar(64) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_merchant_status" ON "hg_youban_publish_merchant" ("status", "id");

INSERT INTO "hg_youban_publish_tenant" ("id", "name", "contact_name", "contact_phone", "remark", "status", "created_by", "updated_by", "deleted_by", "created_at", "updated_at", "deleted_at")
SELECT m."id", m."name", m."contact_name", m."contact_phone", m."remark", m."status", m."created_by", m."updated_by", m."deleted_by", m."created_at", m."updated_at", m."deleted_at"
FROM "hg_youban_publish_merchant" m
WHERE NOT EXISTS (SELECT 1 FROM "hg_youban_publish_tenant" t WHERE t."id" = m."id");

SELECT setval(pg_get_serial_sequence('"hg_youban_publish_tenant"', 'id'), COALESCE((SELECT MAX("id") FROM "hg_youban_publish_tenant"), 1), true);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_account" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "parent_id" bigint NOT NULL DEFAULT 0,
  "account_type" varchar(32) NOT NULL DEFAULT 'uploader',
  "nickname" varchar(128) NOT NULL DEFAULT '',
  "username" varchar(128) NOT NULL DEFAULT '',
  "password_hash" varchar(128) NOT NULL DEFAULT '',
  "salt" varchar(16) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "daily_publish_limit" integer NOT NULL DEFAULT 0,
  "can_direct_publish" smallint NOT NULL DEFAULT 0,
  "allowed_channel_json" text,
  "allowed_region_json" text,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "password_hash" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_account" ADD COLUMN IF NOT EXISTS "salt" varchar(16) NOT NULL DEFAULT '';
UPDATE "hg_youban_publish_account" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_account_tenant" ON "hg_youban_publish_account" ("tenant_id", "account_type", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "client_request_id" varchar(128) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "province" varchar(64) NOT NULL DEFAULT '',
  "city" varchar(64) NOT NULL DEFAULT '',
  "plain_text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "channel_id_json" text,
  "customer_remark" text,
  "anti_scan_enabled" smallint NOT NULL DEFAULT 0,
  "tg_push_enabled" smallint NOT NULL DEFAULT 1,
  "tg_status" varchar(32) NOT NULL DEFAULT 'pending',
  "status" varchar(32) NOT NULL DEFAULT 'draft',
  "error_message" text,
  "submitted_at" timestamp DEFAULT NULL,
  "published_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "channel_id_json" text;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "customer_remark" text;
ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "anti_scan_enabled" smallint NOT NULL DEFAULT 0;
UPDATE "hg_youban_publish_task" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_task_tenant_client_request" ON "hg_youban_publish_task" ("tenant_id", "client_request_id") WHERE "client_request_id" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_task_tenant_status" ON "hg_youban_publish_task" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_task_account_status" ON "hg_youban_publish_task" ("account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "attachment_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT 'image',
  "purpose" varchar(16) NOT NULL DEFAULT 'display',
  "name" varchar(255) NOT NULL DEFAULT '',
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "mime_type" varchar(128) NOT NULL DEFAULT '',
  "md5" varchar(64) NOT NULL DEFAULT '',
  "perceptual_hash" varchar(64) NOT NULL DEFAULT '',
  "size" bigint NOT NULL DEFAULT 0,
  "sort_index" integer NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "perceptual_hash" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "purpose" varchar(16) NOT NULL DEFAULT 'display';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "poster_url" varchar(1024) NOT NULL DEFAULT '';
UPDATE "hg_youban_publish_media" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_attachment" ON "hg_youban_publish_media" ("task_id", "attachment_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_task_sort" ON "hg_youban_publish_media" ("task_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile" ON "hg_youban_publish_media" ("profile_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash" ON "hg_youban_publish_media" ("perceptual_hash");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_purpose" ON "hg_youban_publish_media" ("task_id", "purpose", "sort_index", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_face" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "face_index" integer NOT NULL DEFAULT 0,
  "bbox_json" text NOT NULL DEFAULT '',
  "embedding_model" varchar(64) NOT NULL DEFAULT '',
  "embedding_vector" text NOT NULL DEFAULT '',
  "feature_hash" varchar(128) NOT NULL DEFAULT '',
  "quality_score" numeric(10,4) NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_media_face_media" ON "hg_youban_publish_media_face" ("media_id", "face_index");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_face_profile" ON "hg_youban_publish_media_face" ("tenant_id", "account_id", "profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_face_feature" ON "hg_youban_publish_media_face" ("feature_hash");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tag" (
  "id" BIGSERIAL PRIMARY KEY,
  "name" varchar(64) NOT NULL DEFAULT '',
  "review_status" varchar(32) NOT NULL DEFAULT 'pending',
  "status" smallint NOT NULL DEFAULT 1,
  "use_count" integer NOT NULL DEFAULT 0,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tag_name_deleted" ON "hg_youban_publish_tag" ("name", "deleted_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_tag_review_status" ON "hg_youban_publish_tag" ("review_status", "status", "id");

ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "name" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "review_status" varchar(32) NOT NULL DEFAULT 'pending';
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "status" smallint NOT NULL DEFAULT 1;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "use_count" integer NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "created_by" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "updated_by" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "deleted_by" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "created_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "updated_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_tag" ADD COLUMN IF NOT EXISTS "deleted_at" timestamp DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tag_name_deleted" ON "hg_youban_publish_tag" ("name", "deleted_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_tag_review_status" ON "hg_youban_publish_tag" ("review_status", "status", "id");

INSERT INTO "hg_youban_publish_tag" ("name", "review_status", "status", "use_count", "created_by", "updated_by", "deleted_by", "created_at", "updated_at", "deleted_at")
SELECT seed."name", 'approved', 1, 0, 0, 0, 0, NOW(), NOW(), NULL
FROM (
  VALUES
    ('颜值'),
    ('穿搭'),
    ('美食'),
    ('探店'),
    ('旅行'),
    ('运动'),
    ('健身'),
    ('摄影'),
    ('音乐'),
    ('舞蹈'),
    ('日常'),
    ('生活'),
    ('情感'),
    ('职场'),
    ('学习'),
    ('数码'),
    ('游戏'),
    ('电影'),
    ('宠物'),
    ('家居')
) AS seed("name")
WHERE NOT EXISTS (
  SELECT 1 FROM "hg_youban_publish_tag"
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_job" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "tg_message_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "retry_count" integer NOT NULL DEFAULT 0,
  "next_retry_at" timestamp DEFAULT NULL,
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
UPDATE "hg_youban_publish_tg_job" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_status_retry" ON "hg_youban_publish_tg_job" ("status", "next_retry_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task" ON "hg_youban_publish_tg_job" ("task_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_bot" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_name" varchar(128) NOT NULL DEFAULT '',
  "bot_username" varchar(128) NOT NULL DEFAULT '',
  "bot_token" varchar(255) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_bot" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_tenant" ON "hg_youban_publish_bot" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_status" ON "hg_youban_publish_bot" ("status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_login" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "login_token" varchar(128) NOT NULL DEFAULT '',
  "qr_url" varchar(1024) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "session_key" varchar(255) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "expires_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_login" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_login" ADD COLUMN IF NOT EXISTS "merchant_id" bigint NOT NULL DEFAULT 0;
UPDATE "hg_youban_publish_tg_login" SET "tenant_id" = "merchant_id" WHERE "tenant_id" = 0 AND "merchant_id" > 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_login_token" ON "hg_youban_publish_tg_login" ("login_token");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_login_account" ON "hg_youban_publish_tg_login" ("account_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_account" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "display_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "telegram_first_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_last_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_phone" varchar(64) NOT NULL DEFAULT '',
  "telegram_is_bot" smallint NOT NULL DEFAULT 0,
  "session_key" varchar(255) NOT NULL DEFAULT '',
  "login_token" varchar(128) NOT NULL DEFAULT '',
  "qr_url" varchar(1024) NOT NULL DEFAULT '',
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "last_login_at" timestamp DEFAULT NULL,
  "expires_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_first_name" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_last_name" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_phone" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_account" ADD COLUMN IF NOT EXISTS "telegram_is_bot" smallint NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_account_tenant" ON "hg_youban_publish_tg_account" ("tenant_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_account_login" ON "hg_youban_publish_tg_account" ("login_token");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_title" varchar(128) NOT NULL DEFAULT '',
  "channel_username" varchar(128) NOT NULL DEFAULT '',
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "publish_direction" varchar(16) NOT NULL DEFAULT 'up',
  "cycle_publish_enabled" smallint NOT NULL DEFAULT 0,
  "cycle_publish_days" integer NOT NULL DEFAULT 4,
  "cycle_publish_time" varchar(16) NOT NULL DEFAULT '',
  "is_default_selected" smallint NOT NULL DEFAULT 1,
  "bot_id_json" text,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "last_refresh_status" varchar(32) NOT NULL DEFAULT '',
  "last_refresh_message" varchar(500) NOT NULL DEFAULT '',
  "last_refresh_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_tenant_account" ON "hg_youban_publish_channel" ("tenant_id", "tg_account_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_tenant_direction" ON "hg_youban_publish_channel" ("tenant_id", "publish_direction", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_chat" ON "hg_youban_publish_channel" ("tenant_id", "target_chat_id");
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "is_default_selected" smallint NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tg_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "merchant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" varchar(128) NOT NULL DEFAULT '',
  "access_hash" varchar(128) NOT NULL DEFAULT '',
  "channel_title" varchar(128) NOT NULL DEFAULT '',
  "channel_username" varchar(128) NOT NULL DEFAULT '',
  "is_broadcast" smallint NOT NULL DEFAULT 0,
  "is_megagroup" smallint NOT NULL DEFAULT 0,
  "can_post_messages" smallint NOT NULL DEFAULT 0,
  "can_invite_users" smallint NOT NULL DEFAULT 0,
  "can_add_admins" smallint NOT NULL DEFAULT 0,
  "last_sync_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_channel_account_channel" ON "hg_youban_publish_tg_channel" ("tenant_id", "tg_account_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_channel_search" ON "hg_youban_publish_tg_channel" ("tenant_id", "tg_account_id", "channel_title", "channel_username");

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Telegram App ID', 'int', 'appId', '0', '0', 10, '扫码登录使用的 Telegram API ID，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='appId');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Telegram App Hash', 'string', 'appHash', '', '', 20, '扫码登录使用的 Telegram App Hash，来自 my.telegram.org', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='appHash');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', '代理地址', 'string', 'proxyUrl', '', '', 30, '本地开发可配置 socks5://127.0.0.1:7890，也支持 http/https 代理', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='proxyUrl');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Bot运行模式', 'string', 'botRuntimeMode', 'auto', 'auto', 40, 'auto/develop 使用 pull，production 使用 webhook；也可显式配置 pull/webhook', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='botRuntimeMode');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Webhook Base URL', 'string', 'webhookBaseUrl', '', '', 50, '线上 webhook 的公网域名，例如 https://api.example.com', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='webhookBaseUrl');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', 'Webhook Secret', 'string', 'webhookSecret', '', '', 60, 'Telegram webhook secret token，可选', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='webhookSecret');
INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'telegram', '默认推送 Chat ID', 'string', 'defaultTargetChat', '', '', 70, '资料发布后默认推送的 Telegram chat_id，可由后续频道配置覆盖', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='defaultTargetChat');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT seed."addon_name", seed."group", seed."name", seed."type", seed."key", seed."value", seed."default_value", seed."sort", seed."tip", 0, 1, NOW(), NOW()
FROM (
  VALUES
    ('youban_publish', 'publish', '循环上架开关', 'int', 'cyclePublishEnabled', '0', '0', 100, '是否启用全局循环上架'),
    ('youban_publish', 'publish', '循环上架天数', 'int', 'cyclePublishDays', '4', '4', 110, '循环上架间隔天数'),
    ('youban_publish', 'publish', '循环上架时间', 'string', 'cyclePublishTime', '09:00', '09:00', 120, '每天循环上架执行时间'),
    ('youban_publish', 'publish', '下架不推送到下架频道', 'int', 'skipDownChannelEnabled', '1', '1', 130, '资料下架时是否跳过下架频道推送'),
    ('youban_publish', 'publish', '发送间隔秒数', 'int', 'sendIntervalSeconds', '3', '3', 140, 'Telegram 消息发送间隔'),
    ('youban_publish', 'publish', '发送时间窗口开关', 'int', 'sendWindowEnabled', '0', '0', 150, '是否限制自动发送执行时间段'),
    ('youban_publish', 'publish', '发送开始时间', 'string', 'sendWindowStart', '', '', 160, '自动发送允许开始时间'),
    ('youban_publish', 'publish', '发送结束时间', 'string', 'sendWindowEnd', '', '', 170, '自动发送允许结束时间'),
    ('youban_publish', 'publish', '失败处理策略', 'string', 'failureStrategy', 'continue', 'continue', 180, 'continue 继续后续任务，stop 停止后续任务'),
    ('youban_publish', 'publish', '失败重试开关', 'int', 'retryEnabled', '1', '1', 190, '发送失败后是否重试'),
    ('youban_publish', 'publish', '最大重试次数', 'int', 'maxRetryCount', '3', '3', 200, '发送失败最大重试次数'),
    ('youban_publish', 'publish', '重试间隔分钟', 'int', 'retryIntervalMinutes', '5', '5', 210, '发送失败重试间隔'),
    ('youban_publish', 'publish', '默认防扫图开关', 'int', 'defaultAntiScanEnabled', '1', '1', 220, '新发布内容默认是否启用防扫图'),
    ('youban_publish', 'autoDelete', '频道自动删除开关', 'int', 'autoDeleteEnabled', '0', '0', 200, '是否启用频道自动删除'),
    ('youban_publish', 'autoDelete', '自动删除 Bot ID', '[]int64', 'botIds', '[]', '[]', 210, '执行自动删除的 Bot ID 列表'),
    ('youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '[]', '[]', 220, '命中后自动删除的关键词列表'),
    ('youban_publish', 'antiScan', '防扫图总开关', 'int', 'antiScanEnabled', '1', '1', 300, '是否启用防扫图能力'),
    ('youban_publish', 'antiScan', '新笔记默认防扫图', 'int', 'defaultNewNoteEnabled', '1', '1', 310, '新笔记默认是否开启防扫图'),
    ('youban_publish', 'antiScan', '移除图片元信息', 'int', 'metadataStripEnabled', '1', '1', 320, '是否移除 EXIF 等图片元信息'),
    ('youban_publish', 'antiScan', '人像背景贴图', 'int', 'portraitBackgroundEnabled', '1', '1', 330, '是否启用人像背景贴图'),
    ('youban_publish', 'antiScan', '体验替换背景', 'int', 'backgroundReplaceEnabled', '0', '0', 340, '是否预留替换背景处理'),
    ('youban_publish', 'antiScan', '打码方式', 'string', 'maskMode', 'qr', 'qr', 350, '打码方式：qr二维码模式，sticker贴图模式'),
    ('youban_publish', 'antiScan', '打码数量', 'int', 'maskCount', '1', '1', 360, '同一张图打码数量，最多2个'),
    ('youban_publish', 'antiScan', '二维码文案', 'string', 'qrText', '仅供本频道查看', '仅供本频道查看', 370, '二维码模式的展示文案'),
    ('youban_publish', 'antiScan', '贴图图片', 'string', 'stickerImage', '', '', 380, '贴图模式使用的正方形贴图'),
    ('youban_publish', 'antiScan', '贴图透明度', 'int', 'stickerOpacity', '18', '18', 390, '防扫图贴图透明度，1-100'),
    ('youban_publish', 'antiScan', '水印开关', 'int', 'watermarkEnabled', '1', '1', 400, '是否启用轻水印扰动'),
    ('youban_publish', 'antiScan', '水印文案', 'string', 'watermarkText', 'youban', 'youban', 410, '防扫图水印文案'),
    ('youban_publish', 'antiScan', '贴纸文案', 'string', 'stickerText', '', '', 420, '防扫图贴纸文案'),
    ('youban_publish', 'antiScan', '噪点扰动', 'int', 'noiseEnabled', '1', '1', 430, '是否启用轻微噪点扰动'),
    ('youban_publish', 'antiScan', '噪点强度', 'int', 'noiseStrength', '18', '18', 440, '噪点扰动强度'),
    ('youban_publish', 'antiScan', '压缩重采样', 'int', 'compressionEnabled', '1', '1', 450, '是否启用压缩重采样'),
    ('youban_publish', 'antiScan', '输出质量', 'int', 'compressionQuality', '82', '82', 460, '压缩重采样输出质量'),
    ('youban_publish', 'antiScan', '色彩轻扰动', 'int', 'colorJitterEnabled', '1', '1', 470, '是否启用色彩轻扰动')
) AS seed("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip")
WHERE NOT EXISTS (
  SELECT 1 FROM "hg_sys_addons_config" c
  WHERE c."addon_name" = seed."addon_name"
    AND c."group" = seed."group"
    AND c."key" = seed."key"
);

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'account'
  AND "key" IN ('defaultRoleId', 'defaultDeptId');

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'antiScan'
  AND "key" IN ('enabled', 'mode', 'method', 'stickerStyle', 'stickerDensity', 'qrMaskEnabled', 'qrCustomStickerEnabled', 'qrCount', 'customStickerEnabled');

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'autoDelete'
  AND "key" = 'enabled';
