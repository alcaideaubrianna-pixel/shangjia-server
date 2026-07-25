package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

const (
	messageListenPlanTable   = "hg_youban_publish_message_listen_plan"
	messageListenTargetTable = "hg_youban_publish_message_listen_target"
	messageListenNoticeTable = "hg_youban_publish_message_listen_notice"
	messageListenSenderTable = "hg_youban_publish_message_listen_sender"
)

func ensureMessageListenTables(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureMessageListenPgsqlTables(ctx)
	}
	return ensureMessageListenMysqlTables(ctx)
}

func ensureMessageListenPgsqlTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_plan" (
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
		)`,
		`ALTER TABLE "hg_youban_publish_message_listen_plan" ADD COLUMN IF NOT EXISTS "bind_code" varchar(32) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_message_listen_plan" ADD COLUMN IF NOT EXISTS "notify_chat_id" varchar(128) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_message_listen_plan" ADD COLUMN IF NOT EXISTS "notify_chat_type" varchar(32) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_message_listen_plan" ADD COLUMN IF NOT EXISTS "notify_chat_title" varchar(255) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_message_listen_plan" ADD COLUMN IF NOT EXISTS "notify_bound_at" timestamp DEFAULT NULL`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_plan_owner" ON "hg_youban_publish_message_listen_plan" ("tenant_id", "status", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_plan_account" ON "hg_youban_publish_message_listen_plan" ("tg_account_id", "status", "id")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_plan_code" ON "hg_youban_publish_message_listen_plan" ("bind_code")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_target" (
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
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_target_chat" ON "hg_youban_publish_message_listen_target" ("plan_id", "target_chat_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_target_plan" ON "hg_youban_publish_message_listen_target" ("plan_id", "status", "id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_notice" (
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
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_notice_dedupe" ON "hg_youban_publish_message_listen_notice" ("dedupe_key")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_notice_plan" ON "hg_youban_publish_message_listen_notice" ("plan_id", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_notice_cooldown" ON "hg_youban_publish_message_listen_notice" ("plan_id", "sender_user_id", "normalized_text_hash", "created_at")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_listen_sender" (
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
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_msg_listen_sender_user" ON "hg_youban_publish_message_listen_sender" ("tg_account_id", "telegram_user_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_listen_sender_tenant" ON "hg_youban_publish_message_listen_sender" ("tenant_id", "tg_account_id")`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "检查消息监听表失败")
		}
	}
	return nil
}

func ensureMessageListenMysqlTables(ctx context.Context) error {
	statements := []string{
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_plan` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`name` varchar(128) NOT NULL DEFAULT '',`tg_account_id` bigint NOT NULL DEFAULT 0,`bot_id` bigint NOT NULL DEFAULT 0,`bind_code` varchar(32) NOT NULL DEFAULT '' COMMENT '通知目标绑定ID',`notify_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '通知目标Chat ID',`notify_chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT '通知目标类型',`notify_chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT '通知目标标题',`notify_bound_at` datetime DEFAULT NULL COMMENT '通知目标绑定时间',`keywords_json` text,`status` tinyint NOT NULL DEFAULT 1,`last_trigger_at` datetime DEFAULT NULL,`last_result` text,`created_by` bigint NOT NULL DEFAULT 0,`updated_by` bigint NOT NULL DEFAULT 0,`deleted_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_msg_listen_plan_code` (`bind_code`),KEY `idx_ybp_msg_listen_plan_owner` (`tenant_id`,`status`,`id`),KEY `idx_ybp_msg_listen_plan_account` (`tg_account_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"ALTER TABLE `hg_youban_publish_message_listen_plan` ADD COLUMN `bind_code` varchar(32) NOT NULL DEFAULT '' COMMENT '通知目标绑定ID' AFTER `bot_id`",
		"ALTER TABLE `hg_youban_publish_message_listen_plan` ADD COLUMN `notify_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '通知目标Chat ID' AFTER `bind_code`",
		"ALTER TABLE `hg_youban_publish_message_listen_plan` ADD COLUMN `notify_chat_type` varchar(32) NOT NULL DEFAULT '' COMMENT '通知目标类型' AFTER `notify_chat_id`",
		"ALTER TABLE `hg_youban_publish_message_listen_plan` ADD COLUMN `notify_chat_title` varchar(255) NOT NULL DEFAULT '' COMMENT '通知目标标题' AFTER `notify_chat_type`",
		"ALTER TABLE `hg_youban_publish_message_listen_plan` ADD COLUMN `notify_bound_at` datetime DEFAULT NULL COMMENT '通知目标绑定时间' AFTER `notify_chat_title`",
		"CREATE UNIQUE INDEX `uk_ybp_msg_listen_plan_code` ON `hg_youban_publish_message_listen_plan` (`bind_code`)",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_target` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`plan_id` bigint NOT NULL DEFAULT 0,`tenant_id` bigint NOT NULL DEFAULT 0,`target_chat_id` varchar(128) NOT NULL DEFAULT '',`target_chat_type` varchar(32) NOT NULL DEFAULT '',`target_chat_title` varchar(255) NOT NULL DEFAULT '',`target_chat_username` varchar(255) NOT NULL DEFAULT '',`last_matched_at` datetime DEFAULT NULL,`last_matched_text` text,`last_matched_user_id` varchar(128) NOT NULL DEFAULT '',`status` tinyint NOT NULL DEFAULT 1,`created_by` bigint NOT NULL DEFAULT 0,`updated_by` bigint NOT NULL DEFAULT 0,`deleted_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_msg_listen_target_chat` (`plan_id`,`target_chat_id`),KEY `idx_ybp_msg_listen_target_plan` (`plan_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_notice` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`plan_id` bigint NOT NULL DEFAULT 0,`target_id` bigint NOT NULL DEFAULT 0,`tg_account_id` bigint NOT NULL DEFAULT 0,`source_chat_id` varchar(128) NOT NULL DEFAULT '',`source_message_id` bigint NOT NULL DEFAULT 0,`sender_user_id` varchar(128) NOT NULL DEFAULT '',`sender_username` varchar(128) NOT NULL DEFAULT '',`normalized_text_hash` varchar(128) NOT NULL DEFAULT '',`media_hash` varchar(128) NOT NULL DEFAULT '',`dedupe_key` varchar(255) NOT NULL DEFAULT '',`match_keywords_json` text,`notify_result` text,`created_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_msg_listen_notice_dedupe` (`dedupe_key`),KEY `idx_ybp_msg_listen_notice_plan` (`plan_id`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_listen_sender` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`tg_account_id` bigint NOT NULL DEFAULT 0,`telegram_user_id` varchar(128) NOT NULL DEFAULT '',`telegram_username` varchar(128) NOT NULL DEFAULT '',`telegram_first_name` varchar(128) NOT NULL DEFAULT '',`telegram_last_name` varchar(128) NOT NULL DEFAULT '',`display_name` varchar(255) NOT NULL DEFAULT '',`last_seen_at` datetime DEFAULT NULL,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_msg_listen_sender_user` (`tg_account_id`,`telegram_user_id`),KEY `idx_ybp_msg_listen_sender_tenant` (`tenant_id`,`tg_account_id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE INDEX `idx_ybp_msg_listen_notice_cooldown` ON `hg_youban_publish_message_listen_notice` (`plan_id`,`sender_user_id`,`normalized_text_hash`,`created_at`)",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			if isIgnorableMessageListenSchemaError(err) {
				continue
			}
			return gerror.Wrap(err, "检查消息监听表失败")
		}
	}
	return nil
}

func isIgnorableMessageListenSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "duplicate key name") ||
		strings.Contains(msg, "already exists")
}
