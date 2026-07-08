package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
)

func newTelegramOperationNo(prefix string, taskId int64) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "publish"
	}
	return fmt.Sprintf("%s:%d:%d", prefix, taskId, gtime.Now().TimestampNano())
}

func ensureTelegramOperationColumns(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		statements := []string{
			`ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "tg_operation_no" varchar(128) NOT NULL DEFAULT ''`,
			`ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "operation_no" varchar(128) NOT NULL DEFAULT ''`,
			`DROP INDEX IF EXISTS "uk_ybp_tg_job_task_channel"`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_task_channel" ON "hg_youban_publish_tg_job" ("task_id", "channel_id", "id")`,
			`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_tg_job_operation_channel" ON "hg_youban_publish_tg_job" ("task_id", "operation_no", "channel_id") WHERE "operation_no" <> ''`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_task_tg_operation" ON "hg_youban_publish_task" ("tg_operation_no")`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_operation" ON "hg_youban_publish_tg_job" ("operation_no", "status", "id")`,
		}
		for _, statement := range statements {
			if _, err := g.DB().Exec(ctx, statement); err != nil {
				return gerror.Wrap(err, "检查TG操作隔离字段失败")
			}
		}
		return nil
	}
	statements := []string{
		"ALTER TABLE `hg_youban_publish_task` ADD COLUMN `tg_operation_no` varchar(128) NOT NULL DEFAULT '' COMMENT '当前TG操作号' AFTER `tg_status`",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `operation_no` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG操作号' AFTER `task_id`",
		"ALTER TABLE `hg_youban_publish_tg_job` DROP INDEX `uk_ybp_tg_job_task_channel`",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_task_channel` (`task_id`,`channel_id`,`id`)",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD UNIQUE KEY `uk_ybp_tg_job_operation_channel` (`task_id`,`operation_no`,`channel_id`)",
		"ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_tg_operation` (`tg_operation_no`)",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_operation` (`operation_no`,`status`,`id`)",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableTelegramOperationSchemaError(err) {
			return gerror.Wrap(err, "检查TG操作隔离字段失败")
		}
	}
	return nil
}

func isIgnorableTelegramOperationSchemaError(err error) bool {
	if isIgnorableImportTaskServerIPColumnError(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "duplicate key name") ||
		strings.Contains(message, "can't drop") ||
		strings.Contains(message, "check that column/key exists") ||
		strings.Contains(message, "doesn't exist")
}
