package sys

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
)

var (
	collectTelegramOrderSchemaMu      sync.Mutex
	collectTelegramOrderSchemaChecked bool
)

func ensureCollectTelegramOrderColumns(ctx context.Context) error {
	collectTelegramOrderSchemaMu.Lock()
	defer collectTelegramOrderSchemaMu.Unlock()
	if collectTelegramOrderSchemaChecked {
		return nil
	}
	if err := ensureCollectTelegramOrderColumnsLocked(ctx); err != nil {
		return err
	}
	collectTelegramOrderSchemaChecked = true
	return nil
}

func ensureCollectTelegramOrderColumnsLocked(ctx context.Context) error {
	if strings.EqualFold(g.DB().GetConfig().Type, consts.DBPgsql) {
		statements := []string{
			`ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0`,
			`ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0`,
			`ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT ''`,
			`ALTER TABLE "hg_youban_publish_task" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0`,
			`ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_event_id" bigint NOT NULL DEFAULT 0`,
			`ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_id" bigint NOT NULL DEFAULT 0`,
			`ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_chat_id" varchar(128) NOT NULL DEFAULT ''`,
			`ALTER TABLE "hg_youban_publish_tg_job" ADD COLUMN IF NOT EXISTS "collect_source_message_id" bigint NOT NULL DEFAULT 0`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_task_collect_order" ON "hg_youban_publish_task" ("collect_source_id","collect_source_chat_id","collect_source_message_id","id")`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_tg_job_collect_order" ON "hg_youban_publish_tg_job" ("channel_id","target_chat_id","collect_source_id","collect_source_chat_id","collect_source_message_id","status","id")`,
			`UPDATE "hg_youban_publish_tg_job" j SET
				"collect_event_id"=t."collect_event_id",
				"collect_source_id"=t."collect_source_id",
				"collect_source_chat_id"=t."collect_source_chat_id",
				"collect_source_message_id"=t."collect_source_message_id"
			FROM "hg_youban_publish_task" t
			WHERE j."task_id"=t."id" AND j."collect_source_message_id"=0 AND t."collect_source_message_id">0`,
		}
		for _, statement := range statements {
			if _, err := g.DB().Exec(ctx, statement); err != nil {
				return gerror.Wrap(err, "检查采集TG顺序字段失败")
			}
		}
		return nil
	}
	statements := []string{
		"ALTER TABLE `hg_youban_publish_task` ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID'",
		"ALTER TABLE `hg_youban_publish_task` ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID'",
		"ALTER TABLE `hg_youban_publish_task` ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID'",
		"ALTER TABLE `hg_youban_publish_task` ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID'",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `collect_event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID'",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `collect_source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID'",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `collect_source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '采集来源Chat ID'",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD COLUMN `collect_source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集来源消息ID'",
		"ALTER TABLE `hg_youban_publish_task` ADD KEY `idx_ybp_task_collect_order` (`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`id`)",
		"ALTER TABLE `hg_youban_publish_tg_job` ADD KEY `idx_ybp_tg_job_collect_order` (`channel_id`,`target_chat_id`,`collect_source_id`,`collect_source_chat_id`,`collect_source_message_id`,`status`,`id`)",
		"UPDATE `hg_youban_publish_tg_job` j JOIN `hg_youban_publish_task` t ON t.`id`=j.`task_id` SET j.`collect_event_id`=t.`collect_event_id`, j.`collect_source_id`=t.`collect_source_id`, j.`collect_source_chat_id`=t.`collect_source_chat_id`, j.`collect_source_message_id`=t.`collect_source_message_id` WHERE j.`collect_source_message_id`=0 AND t.`collect_source_message_id`>0",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableTelegramOperationSchemaError(err) {
			return gerror.Wrap(err, "检查采集TG顺序字段失败")
		}
	}
	return nil
}

func collectTelegramOrderDataFromTask(task gdb.Record) g.Map {
	return g.Map{
		"collect_event_id":          task["collect_event_id"].Int64(),
		"collect_source_id":         task["collect_source_id"].Int64(),
		"collect_source_chat_id":    strings.TrimSpace(task["collect_source_chat_id"].String()),
		"collect_source_message_id": task["collect_source_message_id"].Int64(),
		"updated_at":                gtime.Now(),
	}
}

func (s *sSysPublish) collectTelegramJobHasPreviousActive(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.CollectSourceId <= 0 || job.CollectSourceMessageId <= 0 || strings.TrimSpace(job.CollectSourceChatId) == "" {
		return false, nil
	}
	taskActive, err := s.collectTelegramJobHasPreviousActiveTask(ctx, job)
	if err != nil || taskActive {
		return taskActive, err
	}
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id <> ?", job.Id).
		Where("collect_source_id", job.CollectSourceId).
		Where("collect_source_chat_id", strings.TrimSpace(job.CollectSourceChatId)).
		Where("collect_source_message_id < ?", job.CollectSourceMessageId).
		WhereIn("status", []string{"pending", "sending", "failed_retry"})
	if job.ChannelId > 0 {
		mod = mod.Where("channel_id", job.ChannelId)
	} else {
		mod = mod.Where("target_chat_id", job.TargetChatId)
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集TG前序任务失败")
	}
	return count > 0, nil
}
