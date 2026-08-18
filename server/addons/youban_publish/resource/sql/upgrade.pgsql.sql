ALTER TABLE "hg_youban_publish_collect_rule"
  ADD COLUMN IF NOT EXISTS "full_match_enabled" smallint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_group_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0, "group_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0,
  "purpose" varchar(16) NOT NULL DEFAULT 'display', "media_type" varchar(32) NOT NULL DEFAULT '', "sort_index" integer NOT NULL DEFAULT 0,
  "source_file_id" varchar(255) NOT NULL DEFAULT '', "file_url" varchar(1024) NOT NULL DEFAULT '', "storage_path" varchar(1024) NOT NULL DEFAULT '', "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "source_kind" varchar(32) NOT NULL DEFAULT '', "source_media_id" bigint NOT NULL DEFAULT 0, "source_access_hash" bigint NOT NULL DEFAULT 0,
  "source_file_reference" bytea, "source_thumb_size" varchar(32) NOT NULL DEFAULT '', "source_mime_type" varchar(128) NOT NULL DEFAULT '',
  "source_dc_id" integer NOT NULL DEFAULT 0, "source_size" bigint NOT NULL DEFAULT 0, "file_md5" varchar(64) NOT NULL DEFAULT '', "file_phash" varchar(128) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_group_media_group" ON "hg_youban_publish_material_import_group_media" ("group_id", "sort_index", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_material_import_group_media_task" ON "hg_youban_publish_material_import_group_media" ("task_id", "group_id", "id");

ALTER TABLE "hg_youban_publish_collect_rule"
  ADD COLUMN IF NOT EXISTS "delete_text_json" text;

ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "download_duration_ms" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "download_bytes" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "download_attempts" integer NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "cache_hit" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media"
  ADD COLUMN IF NOT EXISTS "download_error_type" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_kind" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_media_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_access_hash" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_file_reference" bytea;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_thumb_size" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_mime_type" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_dc_id" integer NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_size" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "file_md5" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "file_phash" varchar(128) NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_media_stat" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT '',
  "media_total" integer NOT NULL DEFAULT 0,
  "success_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "pending_count" integer NOT NULL DEFAULT 0,
  "cache_hit_count" integer NOT NULL DEFAULT 0,
  "retry_count" integer NOT NULL DEFAULT 0,
  "bytes" bigint NOT NULL DEFAULT 0,
  "duration_ms" bigint NOT NULL DEFAULT 0,
  "p50_ms" bigint NOT NULL DEFAULT 0,
  "p95_ms" bigint NOT NULL DEFAULT 0,
  "throughput_mbps" numeric(18,4) NOT NULL DEFAULT 0,
  "success_rate" numeric(8,5) NOT NULL DEFAULT 0,
  "failure_rate" numeric(8,5) NOT NULL DEFAULT 0,
  "error_summary_json" text,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_collect_media_stat_event" UNIQUE ("event_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_media_stat_owner" ON "hg_youban_publish_collect_media_stat" ("tenant_id", "account_id", "tg_account_id", "created_at");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_media_stat_source" ON "hg_youban_publish_collect_media_stat" ("source_id", "created_at");

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "bot_collect_scope" varchar(16) NOT NULL DEFAULT 'chat';

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_enabled" smallint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_mode" varchar(32) NOT NULL DEFAULT 'recent_days';

ALTER TABLE "hg_youban_publish_collect_source"
  ADD COLUMN IF NOT EXISTS "history_collect_days" integer NOT NULL DEFAULT 30;

CREATE INDEX IF NOT EXISTS "idx_ybp_collect_source_bot_scope" ON "hg_youban_publish_collect_source" ("bot_id", "bot_collect_scope", "source_chat_id", "status");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_bot_channel_cache" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "chat_id" varchar(128) NOT NULL DEFAULT '',
  "chat_type" varchar(32) NOT NULL DEFAULT '',
  "chat_title" varchar(255) NOT NULL DEFAULT '',
  "chat_username" varchar(128) NOT NULL DEFAULT '',
  "is_broadcast" smallint NOT NULL DEFAULT 0,
  "is_megagroup" smallint NOT NULL DEFAULT 0,
  "message_count" integer NOT NULL DEFAULT 0,
  "last_message_text" text,
  "last_message_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_bot_channel_cache_bot_chat" ON "hg_youban_publish_bot_channel_cache" ("tenant_id", "bot_id", "chat_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_bot_channel_cache_list" ON "hg_youban_publish_bot_channel_cache" ("tenant_id", "bot_id", "last_message_at", "id");

ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0;

ALTER TABLE "hg_youban_publish_tg_channel" ADD COLUMN IF NOT EXISTS "management_role" varchar(16) NOT NULL DEFAULT 'member';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "bot_permission_status_json" text NOT NULL DEFAULT '[]';
ALTER TABLE "hg_youban_publish_account" ALTER COLUMN "public_follow_enabled" SET DEFAULT 0;

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集总开关', 'int', 'collectEnabled', '1', '1', 10, '是否启用采集能力', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectEnabled');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '采集推送总开关', 'int', 'collectPushEnabled', '1', '1', 15, '是否允许采集资料推送到 Telegram 频道', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='collectPushEnabled');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'collect', '实时采集推送延迟', 'int', 'realtimePushDelaySec', '600', '600', 20, '实时采集命中规则后延迟推送的秒数，用于等待媒体组和保持来源顺序', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='realtimePushDelaySec');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'autoDelete', '自动删除规则', '[]string', 'rules', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', '["single:^编号[[:space:]]*[:：][[:space:]]*[A-Za-z0-9_-]+$"]', 230, '仅匹配整条消息的自动删除规则', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "key"='rules');

INSERT INTO "hg_sys_addons_config" ("addon_name", "group", "name", "type", "key", "value", "default_value", "sort", "tip", "is_default", "status", "created_at", "updated_at")
SELECT 'youban_publish', 'autoDelete', '自动删除关键词', '[]string', 'keywords', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', '["发现重复资料","验证视频","资料近期已上架","信息已推送成功","视频已推送成功","视频已自动转发到视频频道","小时内已提交过相同内容","内容已自动转发到发布频道","提交成功！","信息视频验证保存成功","收录失败！","处理媒体组失败:","推送媒体组到群","信息投稿推送","推送视频失败","收录成功","本频道弃用，订阅如下新频道","投稿已自动审核通过","重复投稿","重复提交","信息已存在请勿重复提交","采集失败","检测到重复资料","正在分析中","已被人脸过滤","提交成功","去重阶段","重复帖子已拦截","提交失败"]', 220, '所有租户继承的默认自动删除关键词', 0, 1, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_sys_addons_config" WHERE "addon_name"='youban_publish' AND "group"='autoDelete' AND "key"='keywords');

CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_auto_delete_config" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "enabled" smallint NOT NULL DEFAULT 0,
  "bot_ids_json" text,
  "custom_keywords_json" text,
  "custom_rules_json" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tenant_auto_delete_config_tenant" ON "hg_youban_publish_tenant_auto_delete_config" ("tenant_id");

UPDATE "hg_sys_addons_config"
SET "status" = 2,
    "updated_at" = NOW()
WHERE "addon_name" = 'youban_publish'
  AND "group" = 'autoDelete'
  AND "key" IN ('enabled', 'autoDeleteEnabled', 'botIds');

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_history_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "mode" varchar(32) NOT NULL DEFAULT 'recent_days',
  "days" integer NOT NULL DEFAULT 30,
  "offset_id" integer NOT NULL DEFAULT 0,
  "scanned_count" integer NOT NULL DEFAULT 0,
  "event_count" integer NOT NULL DEFAULT 0,
  "duplicate_count" integer NOT NULL DEFAULT 0,
  "failed_count" integer NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "next_run_at" timestamp DEFAULT NULL,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_history_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "level" varchar(16) NOT NULL DEFAULT 'info',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "meta_json" text,
  "created_at" timestamp DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "source_type" varchar(32) NOT NULL DEFAULT '',
  "event_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_id" bigint NOT NULL DEFAULT 0,
  "source_grouped_id" varchar(128) NOT NULL DEFAULT '',
  "source_media_key" varchar(255) NOT NULL DEFAULT '',
  "media_type" varchar(32) NOT NULL DEFAULT '',
  "source_ref_type" varchar(32) NOT NULL DEFAULT '',
  "source_file_id" varchar(255) NOT NULL DEFAULT '',
  "source_message_ref" varchar(255) NOT NULL DEFAULT '',
  "backup_channel_id" bigint NOT NULL DEFAULT 0,
  "backup_chat_id" varchar(128) NOT NULL DEFAULT '',
  "backup_message_id" bigint NOT NULL DEFAULT 0,
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "meta_json" text,
  "sort_index" integer NOT NULL DEFAULT 0,
  "cache_status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_type" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_chat_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_grouped_id" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "meta_json" text;
ALTER TABLE "hg_youban_publish_collect_event" ADD COLUMN IF NOT EXISTS "material_role" varchar(16) NOT NULL DEFAULT 'pending';
ALTER TABLE "hg_youban_publish_collect_event" ADD COLUMN IF NOT EXISTS "material_parent_event_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event" ADD COLUMN IF NOT EXISTS "material_group_status" varchar(32) NOT NULL DEFAULT 'pending';
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_material" ON "hg_youban_publish_collect_event" ("source_id", "source_chat_id", "material_role", "material_parent_event_id", "source_message_id");
ALTER TABLE "hg_youban_publish_collect_review" DROP COLUMN IF EXISTS "media_json";

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "stage" varchar(64) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "meta_text" text,
  "created_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "dispatch_id" bigint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_template" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "text" text,
  "media_count" integer NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_message_template" ADD COLUMN IF NOT EXISTS "serial_no" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_message_template" ADD COLUMN IF NOT EXISTS "push_mode" varchar(16) NOT NULL DEFAULT 'bot';
ALTER TABLE "hg_youban_publish_message_template" ADD COLUMN IF NOT EXISTS "source_message_record_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_bot_id";
ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_chat_id";
ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_message_id";
ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_text_hash";

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "template_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT 'image',
  "name" varchar(255) NOT NULL DEFAULT '',
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "poster_storage_path" varchar(1024) NOT NULL DEFAULT '',
  "tg_file_id" varchar(1024) NOT NULL DEFAULT '',
  "tg_thumb_file_id" varchar(1024) NOT NULL DEFAULT '',
  "asset_hash" varchar(1024) NOT NULL DEFAULT '',
  "sort_index" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
ALTER TABLE "hg_youban_publish_message_media" ADD COLUMN IF NOT EXISTS "source_message_record_id" bigint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_push_plan" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "account_id" bigint NOT NULL DEFAULT 0,
  "template_ids" text,
  "target_chat_ids" text,
  "times" text,
  "interval_seconds" integer NOT NULL DEFAULT 60,
  "status" smallint NOT NULL DEFAULT 1,
  "next_run_at" timestamp DEFAULT NULL,
  "last_run_at" timestamp DEFAULT NULL,
  "last_result" text,
  "locked_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_plan" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "name" varchar(128) NOT NULL DEFAULT '',
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "bot_id" bigint NOT NULL DEFAULT 0,
  "bind_code" varchar(32) NOT NULL DEFAULT '',
  "notify_chat_id" varchar(128) NOT NULL DEFAULT '',
  "notify_chat_type" varchar(32) NOT NULL DEFAULT '',
  "notify_chat_title" varchar(255) NOT NULL DEFAULT '',
  "notify_bound_at" timestamp DEFAULT NULL,
  "keywords_json" text,
  "status" smallint NOT NULL DEFAULT 1,
  "last_trigger_at" timestamp DEFAULT NULL,
  "last_result" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_plan_code" ON "hg_youban_publish_message_listen_plan" ("bind_code");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_target" (
  "id" BIGSERIAL PRIMARY KEY,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "target_chat_id" varchar(128) NOT NULL DEFAULT '',
  "target_chat_type" varchar(32) NOT NULL DEFAULT '',
  "target_chat_title" varchar(255) NOT NULL DEFAULT '',
  "target_chat_username" varchar(255) NOT NULL DEFAULT '',
  "last_matched_at" timestamp DEFAULT NULL,
  "last_matched_text" text,
  "last_matched_user_id" varchar(128) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_target_chat" ON "hg_youban_publish_message_listen_target" ("plan_id", "target_chat_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_notice" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "plan_id" bigint NOT NULL DEFAULT 0,
  "target_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_id" bigint NOT NULL DEFAULT 0,
  "sender_user_id" varchar(128) NOT NULL DEFAULT '',
  "sender_username" varchar(128) NOT NULL DEFAULT '',
  "normalized_text_hash" varchar(128) NOT NULL DEFAULT '',
  "media_hash" varchar(128) NOT NULL DEFAULT '',
  "dedupe_key" varchar(255) NOT NULL DEFAULT '',
  "match_keywords_json" text,
  "notify_result" text,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_notice_dedupe" ON "hg_youban_publish_message_listen_notice" ("dedupe_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_notice_cooldown" ON "hg_youban_publish_message_listen_notice" ("plan_id", "sender_user_id", "normalized_text_hash", "created_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_sender" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "telegram_user_id" varchar(128) NOT NULL DEFAULT '',
  "telegram_username" varchar(128) NOT NULL DEFAULT '',
  "telegram_first_name" varchar(128) NOT NULL DEFAULT '',
  "telegram_last_name" varchar(128) NOT NULL DEFAULT '',
  "display_name" varchar(255) NOT NULL DEFAULT '',
  "last_seen_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_sender_user" ON "hg_youban_publish_message_listen_sender" ("tg_account_id", "telegram_user_id");

CREATE TABLE IF NOT EXISTS hg_youban_publish_quick_push_plan (
  id bigserial PRIMARY KEY,
  tenant_id bigint NOT NULL DEFAULT 0,
  name varchar(128) NOT NULL DEFAULT '',
  account_id bigint NOT NULL DEFAULT 0,
  target_chat_ids text,
  status smallint NOT NULL DEFAULT 1,
  created_by bigint NOT NULL DEFAULT 0,
  updated_by bigint NOT NULL DEFAULT 0,
  deleted_by bigint NOT NULL DEFAULT 0,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);
CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_task" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "tg_account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_title" varchar(255) NOT NULL DEFAULT '',
  "source_username" varchar(128) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "pull_offset_id" bigint NOT NULL DEFAULT 0,
  "pull_limit_days" integer NOT NULL DEFAULT 365,
  "message_total" integer NOT NULL DEFAULT 0,
  "message_done" integer NOT NULL DEFAULT 0,
  "group_total" integer NOT NULL DEFAULT 0,
  "group_done" integer NOT NULL DEFAULT 0,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_failed" integer NOT NULL DEFAULT 0,
  "imported" integer NOT NULL DEFAULT 0,
  "duplicate" integer NOT NULL DEFAULT 0,
  "error_message" text,
  "next_run_at" timestamp DEFAULT NULL,
  "result_json" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_material_import_group" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_grouped_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_ids" text,
  "source_unique_key" varchar(255) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "nickname" varchar(128) NOT NULL DEFAULT '',
  "profile_no" varchar(64) NOT NULL DEFAULT '',
  "raw_text" text,
  "profile_text" text,
  "verify_text" text,
  "media_json" text,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_failed" integer NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_profile_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "message_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_material_import_group" ON "hg_youban_publish_material_import_group" ("tenant_id", "source_unique_key");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_phash_bucket" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT '',
  "hash_value" varchar(64) NOT NULL DEFAULT '',
  "bucket_pos" smallint NOT NULL DEFAULT 0,
  "bucket_value" varchar(1) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_media_phash_bucket_media_pos" ON "hg_youban_publish_media_phash_bucket" ("media_id", "bucket_pos");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_bucket_search" ON "hg_youban_publish_media_phash_bucket" ("tenant_id", "media_type", "bucket_pos", "bucket_value") INCLUDE ("media_id", "profile_id", "account_id", "hash_value");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_phash_lsh" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "media_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "media_type" varchar(16) NOT NULL DEFAULT '',
  "hash_value" varchar(64) NOT NULL DEFAULT '',
  "bucket_pos" smallint NOT NULL DEFAULT 0,
  "bucket_value" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_media_phash_lsh_media_pos" ON "hg_youban_publish_media_phash_lsh" ("media_id", "bucket_pos");
CREATE INDEX IF NOT EXISTS "idx_ybp_media_phash_lsh_search" ON "hg_youban_publish_media_phash_lsh" ("tenant_id", "media_type", "bucket_pos", "bucket_value") INCLUDE ("media_id", "profile_id", "account_id", "hash_value");
CREATE TABLE IF NOT EXISTS "hg_youban_publish_note_index" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "uuid" varchar(128) NOT NULL DEFAULT '',
  "profile_no" varchar(64) NOT NULL DEFAULT '',
  "title" varchar(255) NOT NULL DEFAULT '',
  "summary" text,
  "plain_text" text,
  "tag" text,
  "province" varchar(64) NOT NULL DEFAULT '',
  "city" varchar(64) NOT NULL DEFAULT '',
  "status" smallint NOT NULL DEFAULT 1,
  "visibility" varchar(32) NOT NULL DEFAULT '',
  "review_status" varchar(32) NOT NULL DEFAULT '',
  "task_status" varchar(32) NOT NULL DEFAULT '',
  "cover_media_id" bigint NOT NULL DEFAULT 0,
  "published_at" timestamp DEFAULT NULL,
  "source_updated_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_note_index_scope_profile" UNIQUE ("tenant_id", "account_id", "profile_id")
);
-- Repair installations created before the projection table migration. The
-- application upsert uses the business unique key, but the generated model
-- and pagination still require a stable primary key.
ALTER TABLE "hg_youban_publish_note_index"
  ADD COLUMN IF NOT EXISTS "id" BIGSERIAL;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = 'hg_youban_publish_note_index'::regclass
      AND contype = 'p'
  ) THEN
    ALTER TABLE "hg_youban_publish_note_index"
      ADD CONSTRAINT "pk_ybp_note_index" PRIMARY KEY ("id");
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS "hg_youban_publish_profile_state" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "customer_remark" text,
  "anti_scan_enabled" smallint NOT NULL DEFAULT 0,
  "publish_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_profile_state_profile" ON "hg_youban_publish_profile_state" ("profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_state_owner" ON "hg_youban_publish_profile_state" ("tenant_id", "account_id", "profile_id") WHERE "deleted_at" IS NULL;
CREATE TABLE IF NOT EXISTS "hg_youban_publish_profile_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "is_manual" smallint NOT NULL DEFAULT 1,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_profile_channel" UNIQUE ("tenant_id","profile_id","channel_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_channel_owner" ON "hg_youban_publish_profile_channel" ("tenant_id","account_id","profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_channel_channel" ON "hg_youban_publish_profile_channel" ("tenant_id","channel_id","profile_id");
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "operation_no" varchar(128) NOT NULL DEFAULT '';
DROP INDEX IF EXISTS "uk_ybp_tg_job_task_channel";
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task_channel" ON "hg_youban_publish_tg_job" ("task_id", "channel_id", "id");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_job_operation_channel" ON "hg_youban_publish_tg_job" ("task_id", "operation_no", "channel_id") WHERE "operation_no" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_operation" ON "hg_youban_publish_tg_job" ("operation_no", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_cycle_channel_status_op" ON "hg_youban_publish_tg_job" ("channel_id", "status", "operation_no", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_profile_channel_chat" ON "hg_youban_publish_tg_job" ("tenant_id", "profile_id", "target_chat_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_success_record" (
  "id" BIGSERIAL PRIMARY KEY, "job_id" bigint NOT NULL DEFAULT 0, "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0, "profile_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "bot_id" bigint NOT NULL DEFAULT 0, "operation_no" varchar(128) NOT NULL DEFAULT '',
  "target_chat_id" varchar(128) NOT NULL DEFAULT '', "action" varchar(32) NOT NULL DEFAULT 'profile_publish',
  "status" varchar(16) NOT NULL DEFAULT 'success', "message" varchar(255) NOT NULL DEFAULT '', "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_success_record_job" ON "hg_youban_publish_success_record" ("job_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_owner" ON "hg_youban_publish_success_record" ("tenant_id", "account_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_profile" ON "hg_youban_publish_success_record" ("profile_id", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_full_push_batch" (
  "id" BIGSERIAL PRIMARY KEY, "batch_no" varchar(128) NOT NULL, "tenant_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0, "requested_by" bigint NOT NULL DEFAULT 0,
  "snapshot_max_profile_id" bigint NOT NULL DEFAULT 0, "cursor_profile_id" bigint NOT NULL DEFAULT 0,
  "total_count" integer NOT NULL DEFAULT 0, "queued_count" integer NOT NULL DEFAULT 0, "retry_count" integer NOT NULL DEFAULT 0,
  "status" varchar(16) NOT NULL DEFAULT 'pending', "active_key" varchar(64) DEFAULT NULL, "error_message" text,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL, "finished_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_full_push_batch_no" ON "hg_youban_publish_full_push_batch" ("batch_no");
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_full_push_active" ON "hg_youban_publish_full_push_batch" ("active_key");
CREATE INDEX IF NOT EXISTS "idx_ybp_full_push_schedule" ON "hg_youban_publish_full_push_batch" ("status", "id");
ALTER TABLE "hg_youban_publish_full_push_batch" ADD COLUMN IF NOT EXISTS "snapshot_max_profile_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_full_push_batch" ADD COLUMN IF NOT EXISTS "cursor_profile_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_full_push_batch" ADD COLUMN IF NOT EXISTS "snapshot_max_task_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_full_push_batch" ADD COLUMN IF NOT EXISTS "cursor_task_id" bigint NOT NULL DEFAULT 0;
UPDATE "hg_youban_publish_full_push_batch" SET "snapshot_max_profile_id"="snapshot_max_task_id" WHERE "snapshot_max_profile_id"=0 AND "snapshot_max_task_id">0;
UPDATE "hg_youban_publish_full_push_batch" SET "cursor_profile_id"="cursor_task_id" WHERE "cursor_profile_id"=0 AND "cursor_task_id">0;
ALTER TABLE "hg_youban_publish_full_push_batch" DROP COLUMN IF EXISTS "snapshot_max_task_id";
ALTER TABLE "hg_youban_publish_full_push_batch" DROP COLUMN IF EXISTS "cursor_task_id";
CREATE INDEX IF NOT EXISTS "idx_ybp_media_profile_current" ON "hg_youban_publish_media" ("profile_id", "purpose", "sort_index", "id") WHERE "deleted_at" IS NULL;
ALTER TABLE "hg_youban_publish_tg_job" ALTER COLUMN "task_id" DROP NOT NULL;
ALTER TABLE "hg_youban_publish_tg_job" ALTER COLUMN "task_id" DROP DEFAULT;
ALTER TABLE "hg_youban_publish_tg_message" ALTER COLUMN "task_id" DROP NOT NULL;
ALTER TABLE "hg_youban_publish_tg_message" ALTER COLUMN "task_id" DROP DEFAULT;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_job_profile_operation_channel" ON "hg_youban_publish_tg_job" ("profile_id", "operation_no", "channel_id") WHERE "task_id" IS NULL AND "profile_id" > 0 AND "operation_no" <> '';
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_profile_operation" ON "hg_youban_publish_tg_job" ("profile_id", "operation_no", "status", "id") WHERE "task_id" IS NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_profile_cleanup" ON "hg_youban_publish_tg_job" ("profile_id", "tenant_id", "created_at", "id");
DROP INDEX IF EXISTS "idx_ybp_profile_state_current_task";
ALTER TABLE "hg_youban_publish_profile_state" DROP COLUMN IF EXISTS "current_task_id";
ALTER TABLE "hg_youban_publish_channel_profile" DROP COLUMN IF EXISTS "task_id";
DELETE FROM "hg_admin_menu" WHERE "name" = 'youbanPublishTask';

ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_days" integer NOT NULL DEFAULT 4;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_next_run_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_run_at" timestamp DEFAULT NULL;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_active_run_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_error_message" text;
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "cursor_id" bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "total_count" integer NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS "hg_youban_publish_cycle_run" ADD COLUMN IF NOT EXISTS "queued_count" integer NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS "hg_youban_publish_channel_profile" (
  "id" bigserial PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0,
  "task_id" bigint NOT NULL DEFAULT 0,
  "last_job_id" bigint NOT NULL DEFAULT 0,
  "status" varchar(16) NOT NULL DEFAULT 'active',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_channel_profile" ON "hg_youban_publish_channel_profile" ("channel_id", "profile_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_channel_profile_scan" ON "hg_youban_publish_channel_profile" ("channel_id", "status", "id");

-- 规范化本地媒体存储路径，避免把静态根目录写入业务字段。
UPDATE "hg_youban_publish_media"
SET "storage_path" = regexp_replace("storage_path", '^/?resource/public/', ''),
    "updated_at" = NOW()
WHERE "storage_path" ~ '^/?resource/public/';

UPDATE "hg_youban_publish_media"
SET "poster_storage_path" = regexp_replace("poster_storage_path", '^/?resource/public/', ''),
    "updated_at" = NOW()
WHERE "poster_storage_path" ~ '^/?resource/public/';

UPDATE "hg_youban_publish_media"
SET "original_storage_path" = regexp_replace("original_storage_path", '^/?resource/public/', ''),
    "updated_at" = NOW()
WHERE "original_storage_path" ~ '^/?resource/public/';

UPDATE "hg_youban_publish_media"
SET "edited_storage_path" = regexp_replace("edited_storage_path", '^/?resource/public/', ''),
    "updated_at" = NOW()
WHERE "edited_storage_path" ~ '^/?resource/public/';
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_text_hash" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "text_hash", "received_at", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_order" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "source_id", "source_chat_id", "source_message_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_rule_channel" ON "hg_youban_publish_collect_rule_channel" ("rule_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_channel_owner" ON "hg_youban_publish_collect_rule_channel" ("tenant_id", "account_id", "rule_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule_item" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "rule_id" bigint NOT NULL DEFAULT 0,
  "item_type" varchar(32) NOT NULL DEFAULT '',
  "value" text,
  "replacement" text,
  "sort" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_item_rule" ON "hg_youban_publish_collect_rule_item" ("rule_id", "item_type", "sort", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_item_owner" ON "hg_youban_publish_collect_rule_item" ("tenant_id", "account_id", "rule_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_dispatch_channel" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "channel_id" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_dispatch_channel" ON "hg_youban_publish_collect_dispatch_channel" ("dispatch_id", "channel_id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_channel_owner" ON "hg_youban_publish_collect_dispatch_channel" ("tenant_id", "account_id", "dispatch_id");

-- 采集媒体统一收敛到事件媒体快照和正式媒体表
ALTER TABLE IF EXISTS "hg_youban_publish_collect_content" DROP COLUMN IF EXISTS "media_signature";
ALTER TABLE IF EXISTS "hg_youban_publish_collect_content" DROP COLUMN IF EXISTS "media_json";
DROP TABLE IF EXISTS "hg_youban_publish_collect_content_media";
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_queue" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "source_id", "status", "processed_at", "source_chat_id", "source_message_id", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_dedupe" ON "hg_youban_publish_collect_dispatch" ("tenant_id", "account_id", "event_id", "status", "id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_notice" (
  "id" BIGSERIAL PRIMARY KEY,
  "type" smallint NOT NULL DEFAULT 1,
  "title" varchar(255) NOT NULL DEFAULT '',
  "content" text,
  "tag" bigint NOT NULL DEFAULT 0,
  "receiver" text,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "sort" bigint NOT NULL DEFAULT 0,
  "status" smallint NOT NULL DEFAULT 1,
  "publish_at" timestamp DEFAULT NULL,
  "expire_at" timestamp DEFAULT NULL,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS "idx_ybp_notice_list" ON "hg_youban_publish_notice" ("status","type","sort","id");
CREATE INDEX IF NOT EXISTS "idx_ybp_notice_time" ON "hg_youban_publish_notice" ("status","publish_at","expire_at");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_notice_read" (
  "id" BIGSERIAL PRIMARY KEY,
  "notice_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "clicks" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  CONSTRAINT "uk_ybp_notice_read_account" UNIQUE ("notice_id","account_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_notice_read_account" ON "hg_youban_publish_notice_read" ("account_id","notice_id");

ALTER TABLE "hg_youban_publish_profile_state" ADD COLUMN IF NOT EXISTS "publish_operation_no" varchar(128) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_profile_state" ADD COLUMN IF NOT EXISTS "publish_task_status" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_profile_state" ADD COLUMN IF NOT EXISTS "publish_task_updated_at" timestamp DEFAULT NULL;
CREATE INDEX IF NOT EXISTS "idx_ybp_profile_state_publish_active" ON "hg_youban_publish_profile_state" ("publish_task_status", "publish_task_updated_at", "profile_id") WHERE "deleted_at" IS NULL AND "publish_task_status" <> '';

ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "anti_scan_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "text_obfuscation_enabled" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "auto_delete_enabled" smallint NOT NULL DEFAULT 1;
UPDATE "hg_youban_publish_channel"
SET "auto_delete_enabled" = 1
WHERE "auto_delete_enabled" = 0
  AND "publish_direction" = 'up'
  AND "status" = 1
  AND "deleted_at" IS NULL;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "must_send" smallint NOT NULL DEFAULT 0;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "processing_status" varchar(16) NOT NULL DEFAULT 'ready';
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "processing_error" text;
ALTER TABLE "hg_youban_publish_media" ADD COLUMN IF NOT EXISTS "processing_started_at" timestamp DEFAULT NULL;
UPDATE "hg_youban_publish_media" SET "must_send" = 0 WHERE EXISTS (SELECT 1 FROM "information_schema"."columns" WHERE "table_schema" = current_schema() AND "table_name" = 'hg_youban_publish_media' AND "column_name" = 'must_send' AND "column_default" LIKE '1%');
ALTER TABLE "hg_youban_publish_media" ALTER COLUMN "must_send" SET DEFAULT 0;
CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_feature_permission" ("id" BIGSERIAL PRIMARY KEY,"tenant_id" bigint NOT NULL DEFAULT 0,"feature_code" varchar(64) NOT NULL DEFAULT '',"status" smallint NOT NULL DEFAULT 2,"created_at" timestamp DEFAULT NULL,"updated_at" timestamp DEFAULT NULL,UNIQUE("tenant_id","feature_code"));
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "send_phase" varchar(32) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "reconcile_count" integer NOT NULL DEFAULT 0;
DELETE FROM "hg_youban_publish_tg_message" older USING "hg_youban_publish_tg_message" newer WHERE older."job_id" = newer."job_id" AND older."tg_message_id" = newer."tg_message_id" AND older."id" < newer."id";
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_message_job_message" ON "hg_youban_publish_tg_message" ("job_id", "tg_message_id");
ALTER TABLE "hg_youban_publish_tg_channel_stat" ALTER COLUMN "last_error_message" TYPE text;
ALTER TABLE "hg_youban_publish_tg_bot_stat" ALTER COLUMN "last_error_message" TYPE text;
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_tenant_channel_status" ON "hg_youban_publish_tg_job" ("tenant_id", "channel_id", "status", "id");
CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_due_dispatch" ON "hg_youban_publish_tg_job" ("dispatch_status", "status", "next_retry_at", "created_at", "id");
CREATE TABLE IF NOT EXISTS "hg_youban_publish_media_phash_alias_bucket" (
  "id" BIGSERIAL PRIMARY KEY, "tenant_id" bigint NOT NULL DEFAULT 0, "account_id" bigint NOT NULL DEFAULT 0,
  "profile_id" bigint NOT NULL DEFAULT 0, "media_id" bigint NOT NULL DEFAULT 0, "media_type" varchar(16) NOT NULL DEFAULT '',
  "fingerprint_key" varchar(64) NOT NULL DEFAULT '', "hash_value" varchar(64) NOT NULL DEFAULT '',
  "bucket_pos" smallint NOT NULL DEFAULT 0, "bucket_value" integer NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_phash_alias_media_key_pos" ON "hg_youban_publish_media_phash_alias_bucket" ("media_id","fingerprint_key","bucket_pos");
CREATE INDEX IF NOT EXISTS "idx_ybp_phash_alias_search" ON "hg_youban_publish_media_phash_alias_bucket" ("tenant_id","media_type","bucket_pos","bucket_value") INCLUDE ("media_id","profile_id","account_id","hash_value");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cms_app" (
  "id" BIGSERIAL PRIMARY KEY, "app_id" varchar(64) NOT NULL DEFAULT '',
  "app_secret" varchar(255) NOT NULL DEFAULT '', "name" varchar(128) NOT NULL DEFAULT '',
  "base_url" varchar(512) NOT NULL DEFAULT '', "instance_id" varchar(128) DEFAULT NULL,
  "enroll_hash" varchar(64) NOT NULL DEFAULT '', "source_ip" varchar(64) NOT NULL DEFAULT '',
  "cms_version" varchar(64) NOT NULL DEFAULT '', "last_heartbeat_at" timestamp DEFAULT NULL,
  "status" smallint NOT NULL DEFAULT 1, "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL,
  UNIQUE ("app_id"), UNIQUE ("instance_id")
);

ALTER TABLE "hg_youban_publish_cms_app" ADD COLUMN IF NOT EXISTS "instance_id" varchar(128) DEFAULT NULL;
ALTER TABLE "hg_youban_publish_cms_app" ADD COLUMN IF NOT EXISTS "enroll_hash" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_cms_app" ADD COLUMN IF NOT EXISTS "source_ip" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_cms_app" ADD COLUMN IF NOT EXISTS "cms_version" varchar(64) NOT NULL DEFAULT '';
ALTER TABLE "hg_youban_publish_cms_app" ADD COLUMN IF NOT EXISTS "last_heartbeat_at" timestamp DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_cms_app_instance_id" ON "hg_youban_publish_cms_app" ("instance_id");

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cms_binding_code" (
  "id" BIGSERIAL PRIMARY KEY, "app_id" varchar(64) NOT NULL DEFAULT '',
  "code_hash" varchar(64) NOT NULL DEFAULT '', "code_hint" varchar(16) NOT NULL DEFAULT '',
  "version" integer NOT NULL DEFAULT 1, "status" smallint NOT NULL DEFAULT 1,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL,
  UNIQUE ("app_id"), UNIQUE ("code_hash")
);

CREATE TABLE IF NOT EXISTS "hg_youban_publish_cms_tenant_binding" (
  "id" BIGSERIAL PRIMARY KEY, "app_id" varchar(64) NOT NULL DEFAULT '',
  "tenant_id" bigint NOT NULL DEFAULT 0, "code_version" integer NOT NULL DEFAULT 1,
  "status" varchar(16) NOT NULL DEFAULT 'pending', "reason" varchar(500) NOT NULL DEFAULT '',
  "requested_at" timestamp DEFAULT NULL, "reviewed_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL, "updated_at" timestamp DEFAULT NULL,
  UNIQUE ("app_id","tenant_id")
);
CREATE INDEX IF NOT EXISTS "idx_ybp_cms_binding_tenant_status" ON "hg_youban_publish_cms_tenant_binding" ("tenant_id","status");

CREATE INDEX IF NOT EXISTS "idx_content_profile_height_active" ON "hg_content_profile" ("height") WHERE "deleted_at" IS NULL AND "height" > 0;
CREATE INDEX IF NOT EXISTS "idx_content_profile_weight_active" ON "hg_content_profile" ("weight") WHERE "deleted_at" IS NULL AND "weight" > 0;
CREATE INDEX IF NOT EXISTS "idx_content_profile_cup_active" ON "hg_content_profile" ("cup_size") WHERE "deleted_at" IS NULL AND "cup_size" <> '';
