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

func ensurePublishChannelPgsqlColumns(ctx context.Context) error {
	_, err := g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_channel" ADD COLUMN IF NOT EXISTS "publish_visible" smallint NOT NULL DEFAULT 1`)
	return err
}

func ensurePublishChannelMysqlColumns(ctx context.Context) error {
	_, err := g.DB().Exec(ctx, "ALTER TABLE `hg_youban_publish_channel` ADD COLUMN `publish_visible` tinyint(1) NOT NULL DEFAULT '1' COMMENT '上架端资料选择可见' AFTER `is_default_selected`")
	if err != nil && isPublishChannelSchemaExistsError(err) {
		return nil
	}
	return err
}

func isPublishChannelSchemaExistsError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") || strings.Contains(message, "already exists")
}
