package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
)

const (
	messageTemplateTable = "hg_youban_publish_message_template"
	messageMediaTable    = "hg_youban_publish_message_media"
	messagePushPlanTable = "hg_youban_publish_message_push_plan"
	quickPushPlanTable   = "hg_youban_publish_quick_push_plan"
)

func ensureMessagePushTables(ctx context.Context) error {
	var err error
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		err = ensureMessagePushPgsqlTables(ctx)
	} else {
		err = ensureMessagePushMysqlTables(ctx)
	}
	if err != nil {
		return err
	}
	return ensureMessageTemplateSerials(ctx)
}

func ensureMessageTemplateSerials(ctx context.Context) error {
	var rows []struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
		Fields("id").Where("serial_no", "").WhereNull("deleted_at").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取历史消息模板编号失败")
	}
	for _, row := range rows {
		serial, err := newInlineTemplateSerial()
		if err != nil {
			return err
		}
		if _, err = g.DB().Model(messageTemplateTable).Safe().Ctx(ctx).
			Where("id", row.Id).Where("serial_no", "").Data(g.Map{"serial_no": serial, "updated_at": gtime.Now()}).Update(); err != nil {
			return gerror.Wrap(err, "回填历史消息模板编号失败")
		}
	}
	return nil
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
		`ALTER TABLE "hg_youban_publish_message_template" ADD COLUMN IF NOT EXISTS "serial_no" varchar(32) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_message_template" ADD COLUMN IF NOT EXISTS "push_mode" varchar(16) NOT NULL DEFAULT 'bot'`,
		`ALTER TABLE "hg_youban_publish_message_template" ADD COLUMN IF NOT EXISTS "source_message_record_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_bot_id"`,
		`ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_chat_id"`,
		`ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_message_id"`,
		`ALTER TABLE "hg_youban_publish_message_template" DROP COLUMN IF EXISTS "source_text_hash"`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uq_ybp_msg_tpl_serial" ON "hg_youban_publish_message_template" ("serial_no") WHERE "serial_no" <> ''`,
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
		`ALTER TABLE "hg_youban_publish_message_media" ADD COLUMN IF NOT EXISTS "source_message_record_id" bigint NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_message_push_plan" (
			"id" BIGSERIAL PRIMARY KEY,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"name" varchar(128) NOT NULL DEFAULT '',
			"account_id" bigint NOT NULL DEFAULT 0,
			"template_ids" text,
			"target_chat_ids" text,
			"times" text,
			"interval_days" integer NOT NULL DEFAULT 1,
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
		`ALTER TABLE "hg_youban_publish_message_push_plan" ADD COLUMN IF NOT EXISTS "interval_days" integer NOT NULL DEFAULT 1`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_plan_due" ON "hg_youban_publish_message_push_plan" ("status", "next_run_at", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_msg_plan_owner" ON "hg_youban_publish_message_push_plan" ("tenant_id", "status", "id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_quick_push_plan" (
			"id" BIGSERIAL PRIMARY KEY,
			"tenant_id" bigint NOT NULL DEFAULT 0,
			"name" varchar(128) NOT NULL DEFAULT '',
			"account_id" bigint NOT NULL DEFAULT 0,
			"target_chat_ids" text,
			"status" smallint NOT NULL DEFAULT 1,
			"created_by" bigint NOT NULL DEFAULT 0,
			"updated_by" bigint NOT NULL DEFAULT 0,
			"deleted_by" bigint NOT NULL DEFAULT 0,
			"created_at" timestamp DEFAULT NULL,
			"updated_at" timestamp DEFAULT NULL,
			"deleted_at" timestamp DEFAULT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_quick_plan_owner" ON "hg_youban_publish_quick_push_plan" ("tenant_id", "status", "id")`,
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
		"ALTER TABLE `hg_youban_publish_message_template` ADD COLUMN `serial_no` varchar(32) NOT NULL DEFAULT ''",
		"ALTER TABLE `hg_youban_publish_message_template` ADD COLUMN `push_mode` varchar(16) NOT NULL DEFAULT 'bot'",
		"ALTER TABLE `hg_youban_publish_message_template` ADD COLUMN `source_message_record_id` bigint NOT NULL DEFAULT 0",
		"ALTER TABLE `hg_youban_publish_message_template` DROP COLUMN `source_bot_id`",
		"ALTER TABLE `hg_youban_publish_message_template` DROP COLUMN `source_chat_id`",
		"ALTER TABLE `hg_youban_publish_message_template` DROP COLUMN `source_message_id`",
		"ALTER TABLE `hg_youban_publish_message_template` DROP COLUMN `source_text_hash`",
		"ALTER TABLE `hg_youban_publish_message_media` ADD COLUMN `source_message_record_id` bigint NOT NULL DEFAULT 0",
		"ALTER TABLE `hg_youban_publish_message_template` ADD KEY `idx_ybp_msg_tpl_serial` (`serial_no`)",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_message_push_plan` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`name` varchar(128) NOT NULL DEFAULT '',`account_id` bigint NOT NULL DEFAULT 0,`template_ids` text,`target_chat_ids` text,`times` text,`interval_days` int NOT NULL DEFAULT 1,`interval_seconds` int NOT NULL DEFAULT 60,`status` tinyint NOT NULL DEFAULT 1,`next_run_at` datetime DEFAULT NULL,`last_run_at` datetime DEFAULT NULL,`last_result` text,`locked_at` datetime DEFAULT NULL,`created_by` bigint NOT NULL DEFAULT 0,`updated_by` bigint NOT NULL DEFAULT 0,`deleted_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),KEY `idx_ybp_msg_plan_due` (`status`,`next_run_at`,`id`),KEY `idx_ybp_msg_plan_owner` (`tenant_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"ALTER TABLE `hg_youban_publish_message_push_plan` ADD COLUMN `interval_days` int NOT NULL DEFAULT 1",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_quick_push_plan` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT 0,`name` varchar(128) NOT NULL DEFAULT '',`account_id` bigint NOT NULL DEFAULT 0,`target_chat_ids` text,`status` tinyint NOT NULL DEFAULT 1,`created_by` bigint NOT NULL DEFAULT 0,`updated_by` bigint NOT NULL DEFAULT 0,`deleted_by` bigint NOT NULL DEFAULT 0,`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,`deleted_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),KEY `idx_ybp_quick_plan_owner` (`tenant_id`,`status`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableMessageTemplateSchemaError(err) {
			return gerror.Wrap(err, "检查消息推送表失败")
		}
	}
	return nil
}

func isIgnorableMessageTemplateSchemaError(err error) bool {
	if err == nil {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") ||
		strings.Contains(message, "duplicate key name") ||
		strings.Contains(message, "can't drop") ||
		strings.Contains(message, "check that column/key exists") ||
		strings.Contains(message, "already exists") ||
		isIgnorableImportTaskServerIPColumnError(err)
}
