package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func ensurePublishChannelColumns(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensurePublishChannelPgsqlColumns(ctx)
	}
	return ensurePublishChannelMysqlColumns(ctx)
}

func ensurePublishTgChannelColumns(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensurePublishTgChannelPgsqlColumns(ctx)
	}
	return ensurePublishTgChannelMysqlColumns(ctx)
}

func ensurePublishChannelPgsqlColumns(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_enabled" smallint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_days" integer NOT NULL DEFAULT 4`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_publish_time" varchar(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_next_run_at" timestamp DEFAULT NULL`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_run_at" timestamp DEFAULT NULL`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_active_run_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "cycle_last_error_message" text`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "publish_visible" smallint NOT NULL DEFAULT 1`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "anti_scan_enabled" smallint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "text_obfuscation_enabled" smallint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "auto_delete_enabled" smallint NOT NULL DEFAULT 1`,
		`ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "bot_permission_status_json" text NOT NULL DEFAULT '[]'`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func ensurePublishChannelMysqlColumns(ctx context.Context) error {
	statements := []string{
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否循环上架' AFTER `publish_direction`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_days` int(11) NOT NULL DEFAULT '4' COMMENT '循环上架天数' AFTER `cycle_publish_enabled`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_publish_time` varchar(16) NOT NULL DEFAULT '' COMMENT '循环上架时间' AFTER `cycle_publish_days`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_next_run_at` datetime DEFAULT NULL COMMENT '下次循环上架时间' AFTER `cycle_publish_time`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_last_run_at` datetime DEFAULT NULL COMMENT '上次循环上架时间' AFTER `cycle_next_run_at`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_active_run_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '当前循环批次ID' AFTER `cycle_last_run_at`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `cycle_last_error_message` text COMMENT '循环上架最近错误' AFTER `cycle_active_run_id`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `publish_visible` tinyint(1) NOT NULL DEFAULT '1' COMMENT '上架端资料选择可见' AFTER `is_default_selected`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `anti_scan_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道防扫图开关' AFTER `publish_visible`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `text_obfuscation_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '频道文本混淆开关' AFTER `anti_scan_enabled`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `auto_delete_enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '频道自动删除开关' AFTER `text_obfuscation_enabled`",
		"ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `bot_permission_status_json` text NOT NULL COMMENT '频道Bot权限检测结果JSON' AFTER `bot_id_json`",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isPublishChannelSchemaExistsError(err) {
			return err
		}
	}
	return nil
}

func ensurePublishTgChannelPgsqlColumns(ctx context.Context) error {
	_, err := g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_tg_channel" ADD COLUMN IF NOT EXISTS "management_role" varchar(16) NOT NULL DEFAULT 'member'`)
	return err
}

func ensurePublishTgChannelMysqlColumns(ctx context.Context) error {
	_, err := g.DB().Exec(ctx, "ALTER TABLE `hg_youban_publish_tg_channel` ADD COLUMN `management_role` varchar(16) NOT NULL DEFAULT 'member' COMMENT '当前TG账号角色：owner/admin/member' AFTER `channel_username`")
	if err != nil && isPublishChannelSchemaExistsError(err) {
		return nil
	}
	return err
}

func isPublishChannelSchemaExistsError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") || strings.Contains(message, "already exists")
}
