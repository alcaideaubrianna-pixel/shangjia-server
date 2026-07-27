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
	publishSuccessTypeCollect = "collect_publish"
	publishSuccessTypeCycle   = "cycle_publish"
	publishSuccessTypeFull    = "full_push"
	publishSuccessTypeProfile = "profile_publish"
)

func ensurePublishSuccessRecordSchema(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS "hg_youban_publish_success_record" (
				"id" bigserial PRIMARY KEY,
				"job_id" bigint NOT NULL DEFAULT 0,
				"task_id" bigint NOT NULL DEFAULT 0,
				"tenant_id" bigint NOT NULL DEFAULT 0,
				"account_id" bigint NOT NULL DEFAULT 0,
				"profile_id" bigint NOT NULL DEFAULT 0,
				"channel_id" bigint NOT NULL DEFAULT 0,
				"bot_id" bigint NOT NULL DEFAULT 0,
				"operation_no" varchar(128) NOT NULL DEFAULT '',
				"target_chat_id" varchar(128) NOT NULL DEFAULT '',
				"action" varchar(32) NOT NULL DEFAULT 'profile_publish',
				"status" varchar(16) NOT NULL DEFAULT 'success',
				"message" varchar(255) NOT NULL DEFAULT '',
				"created_at" timestamp DEFAULT NULL
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_success_record_job" ON "hg_youban_publish_success_record" ("job_id")`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_owner" ON "hg_youban_publish_success_record" ("tenant_id", "account_id", "id")`,
			`CREATE INDEX IF NOT EXISTS "idx_ybp_success_record_profile" ON "hg_youban_publish_success_record" ("profile_id", "id")`,
		}
		for _, statement := range statements {
			if _, err := g.DB().Exec(ctx, statement); err != nil {
				return gerror.Wrap(err, "初始化成功发布记录表失败")
			}
		}
		return nil
	}
	statements := []string{
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_success_record` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`job_id` bigint NOT NULL DEFAULT 0,`task_id` bigint NOT NULL DEFAULT 0,`tenant_id` bigint NOT NULL DEFAULT 0,`account_id` bigint NOT NULL DEFAULT 0,`profile_id` bigint NOT NULL DEFAULT 0,`channel_id` bigint NOT NULL DEFAULT 0,`bot_id` bigint NOT NULL DEFAULT 0,`operation_no` varchar(128) NOT NULL DEFAULT '',`target_chat_id` varchar(128) NOT NULL DEFAULT '',`action` varchar(32) NOT NULL DEFAULT 'profile_publish',`status` varchar(16) NOT NULL DEFAULT 'success',`message` varchar(255) NOT NULL DEFAULT '',`created_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_success_record_job` (`job_id`),KEY `idx_ybp_success_record_owner` (`tenant_id`,`account_id`,`id`),KEY `idx_ybp_success_record_profile` (`profile_id`,`id`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "初始化成功发布记录表失败")
		}
	}
	return nil
}

func (s *sSysPublish) appendPublishSuccessRecord(ctx context.Context, job telegramJobRecord) error {
	if err := ensurePublishSuccessRecordSchema(ctx); err != nil {
		return err
	}
	action := publishSuccessRecordAction(job.OperationNo)
	_, err := g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":         job.Id,
		"task_id":        job.TaskId,
		"tenant_id":      job.TenantId,
		"account_id":     job.AccountId,
		"profile_id":     job.ProfileId,
		"channel_id":     job.ChannelId,
		"bot_id":         job.BotId,
		"operation_no":   job.OperationNo,
		"target_chat_id": job.TargetChatId,
		"action":         action,
		"status":         "success",
		"message":        publishSuccessRecordMessage(action),
		"created_at":     gtime.Now(),
	}).OnConflict("job_id").OnDuplicateEx("id", "created_at").Save()
	if err != nil {
		return gerror.Wrap(err, "保存成功发布记录失败")
	}
	return nil
}

func (s *sSysPublish) backfillPublishSuccessRecords(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 1000
	}
	var jobs []telegramJobRecord
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.id,j.task_id,j.operation_no,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.bot_id,j.target_chat_id").
		Where("j.status", "sent").
		Where("NOT EXISTS (SELECT 1 FROM " + publishSuccessRecordTable + " r WHERE r.job_id=j.id)").
		OrderDesc("j.id").
		Limit(limit).
		Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取待补写成功发布记录失败")
	}
	for _, job := range jobs {
		if err := s.appendPublishSuccessRecord(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func publishSuccessRecordAction(operationNo string) string {
	operationNo = strings.ToLower(strings.TrimSpace(operationNo))
	switch {
	case strings.HasPrefix(operationNo, "cycle_batch:"):
		return publishSuccessTypeCycle
	case strings.HasPrefix(operationNo, "full_push:"):
		return publishSuccessTypeFull
	case strings.HasPrefix(operationNo, "collect:"):
		return publishSuccessTypeCollect
	default:
		return publishSuccessTypeProfile
	}
}

func publishSuccessRecordMessage(action string) string {
	switch action {
	case publishSuccessTypeCycle:
		return "循环上架推送成功"
	case publishSuccessTypeFull:
		return "全量推送成功"
	case publishSuccessTypeCollect:
		return "采集推送成功"
	default:
		return "上架推送成功"
	}
}
