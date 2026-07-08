package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func ensureCollectRuleColumns(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureCollectRulePgsqlColumns(ctx)
	}
	return ensureCollectRuleMysqlColumns(ctx)
}

func ensureCollectRulePgsqlColumns(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE "hg_youban_publish_collect_rule" ADD COLUMN IF NOT EXISTS "full_match_enabled" smallint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_rule" ADD COLUMN IF NOT EXISTS "delete_text_json" text`,
	}
	for _, sql := range statements {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func ensureCollectRuleMysqlColumns(ctx context.Context) error {
	statements := []string{
		"ALTER TABLE `hg_youban_publish_collect_rule` ADD COLUMN `full_match_enabled` tinyint(1) NOT NULL DEFAULT '0' COMMENT '全量匹配' AFTER `dedupe_days`",
		"ALTER TABLE `hg_youban_publish_collect_rule` ADD COLUMN `delete_text_json` text COMMENT '删除文本JSON' AFTER `replace_json`",
	}
	for _, sql := range statements {
		if _, err := g.DB().Exec(ctx, sql); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}
