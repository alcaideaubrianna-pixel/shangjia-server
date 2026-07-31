package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	publishSuccessTypeCollect = "collect_publish"
	publishSuccessTypeCycle   = "cycle_publish"
	publishSuccessTypeFull    = "full_push"
	publishSuccessTypeProfile = "profile_publish"
)

func (s *sSysPublish) appendPublishSuccessRecord(ctx context.Context, job telegramJobRecord) error {
	message := s.publishSuccessRecordMessage(ctx, job)
	if err := s.upsertPublishJobRecord(ctx, job, "success", message); err != nil {
		return err
	}
	_, err := g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		Where("job_id", job.Id).
		Data(g.Map{"message": message}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新发布记录提示失败")
	}
	return nil
}

func (s *sSysPublish) publishSuccessRecordMessage(ctx context.Context, job telegramJobRecord) string {
	message := publishSuccessRecordMessage(publishSuccessRecordAction(job.OperationNo))
	policy, err := s.telegramChannelSendPolicy(ctx, job)
	if err != nil {
		return message
	}
	if policy.AntiScanEnabled {
		message += " [防扫图: 开启]"
	}
	if policy.TextObfuscationEnabled {
		message += " [图片混淆: 开启]"
	}
	return message
}

func (s *sSysPublish) upsertPublishJobRecord(ctx context.Context, job telegramJobRecord, status string, message string) error {
	if job.Id <= 0 {
		return nil
	}
	action := publishSuccessRecordAction(job.OperationNo)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "pending"
	}
	if strings.TrimSpace(message) == "" {
		message = publishJobRecordMessage(action, status)
	}
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
		"status":         status,
		"message":        message,
		"created_at":     gtime.Now(),
	}).OnConflict("job_id").OnDuplicateEx("id", "created_at").Save()
	if err != nil {
		return gerror.Wrap(err, "保存发布记录失败")
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

func publishJobRecordMessage(action string, status string) string {
	switch status {
	case "sending":
		return "TG资料正在发送"
	case "failed_retry":
		return "TG资料发送失败，等待重试"
	case "failed":
		return "TG资料发送失败"
	case "success", "sent":
		return publishSuccessRecordMessage(action)
	default:
		return "TG资料等待推送"
	}
}
