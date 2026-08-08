package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func ensureCollectSourceColumns(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureCollectSourcePgsqlColumns(ctx)
	}
	return ensureCollectSourceMysqlColumns(ctx)
}

func ensureCollectSourcePgsqlColumns(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "bot_collect_scope" varchar(16) NOT NULL DEFAULT 'chat'`,
		`ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "history_collect_enabled" smallint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "history_collect_mode" varchar(32) NOT NULL DEFAULT 'recent_days'`,
		`ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "history_collect_days" integer NOT NULL DEFAULT 30`,
	}
	for _, sql := range statements {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func ensureCollectSourceMysqlColumns(ctx context.Context) error {
	statements := []string{
		"ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN `bot_collect_scope` varchar(16) NOT NULL DEFAULT 'chat' COMMENT 'Bot采集范围：chat/private' AFTER `bot_id`",
		"ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN `history_collect_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '账号历史采集开关' AFTER `collect_enabled`",
		"ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN `history_collect_mode` varchar(32) NOT NULL DEFAULT 'recent_days' COMMENT '账号历史采集模式' AFTER `history_collect_enabled`",
		"ALTER TABLE `hg_youban_publish_collect_source` ADD COLUMN `history_collect_days` int(11) NOT NULL DEFAULT '30' COMMENT '账号历史采集天数' AFTER `history_collect_mode`",
	}
	for _, sql := range statements {
		if _, err := g.DB().Exec(ctx, sql); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}
