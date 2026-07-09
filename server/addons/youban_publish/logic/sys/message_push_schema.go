package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

const (
	messageTemplateTable = "hg_youban_publish_message_template"
	messageMediaTable    = "hg_youban_publish_message_media"
	messagePushPlanTable = "hg_youban_publish_message_push_plan"
)

func ensureMessagePushTables(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureMessagePushPgsqlTables(ctx)
	}
	return ensureMessagePushMysqlTables(ctx)
}

func ensureMessagePushPgsqlTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_template" (
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
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_tpl_owner" ON "hg_youban_publish_message_template" ("tenant_id", "status", "id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_media" (
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
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_media_tpl" ON "hg_youban_publish_message_media" ("template_id", "sort_index", "id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_push_plan" (
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
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_plan_due" ON "hg_youban_publish_message_push_plan" ("status", "next_run_at", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_plan_owner" ON "hg_youban_publish_message_push_plan" ("tenant_id", "status", "id")`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "检查消息推送表失败")
		}
	}
	return nil
}

func ensureMessagePushMysqlTables(ctx context.Context) error {
	statements := []string{
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_template` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`name` varchar(128) NOT NULL DEFAULT '',`text` text,`media_count` int NOT NULL DEFAULT 0,`status` tinyint NOT NULL DEFAULT 1,`created_by` bigint NOT NULL DEFAULT 0,`updated_by` bigint NOT NULL DEFAULT 0,`deleted_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),KEY `idx_ybp_msg_tpl_owner` (`tenant_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_media` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`template_id` bigint NOT NULL DEFAULT 0,`tenant_id` bigint NOT NULL DEFAULT 0,`media_type` varchar(16) NOT NULL DEFAULT 'image',`name` varchar(255) NOT NULL DEFAULT '',`file_url` varchar(1024) NOT NULL DEFAULT '',`storage_path` varchar(1024) NOT NULL DEFAULT '',`poster_url` varchar(1024) NOT NULL DEFAULT '',`poster_storage_path` varchar(1024) NOT NULL DEFAULT '',`tg_file_id` varchar(1024) NOT NULL DEFAULT '',`tg_thumb_file_id` varchar(1024) NOT NULL DEFAULT '',`asset_hash` varchar(1024) NOT NULL DEFAULT '',`sort_index` int NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),KEY `idx_ybp_msg_media_tpl` (`template_id`,`sort_index`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_push_plan` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`name` varchar(128) NOT NULL DEFAULT '',`account_id` bigint NOT NULL DEFAULT 0,`template_ids` text,`target_chat_ids` text,`times` text,`interval_seconds` int NOT NULL DEFAULT 60,`status` tinyint NOT NULL DEFAULT 1,`next_run_at` datetime DEFAULT NULL,`last_run_at` datetime DEFAULT NULL,`last_result` text,`locked_at` datetime DEFAULT NULL,`created_by` bigint NOT NULL DEFAULT 0,`updated_by` bigint NOT NULL DEFAULT 0,`deleted_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),KEY `idx_ybp_msg_plan_due` (`status`,`next_run_at`,`id`),KEY `idx_ybp_msg_plan_owner` (`tenant_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableImportTaskServerIPColumnError(err) {
			return gerror.Wrap(err, "检查消息推送表失败")
		}
	}
	return nil
}
