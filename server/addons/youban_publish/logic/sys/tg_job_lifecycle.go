package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) deleteTelegramJobMessagesForResubmit(ctx context.Context, job telegramResubmitJob) error {
	return s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), "编辑资料")
}

func (s *sSysPublish) markTelegramJobSuperseded(ctx context.Context, jobId int64) error {
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: jobId,
		From: []string{
			telegramJobStatusPending, telegramJobStatusSending, telegramJobStatusRetryWait,
			telegramJobStatusConfirming, telegramJobStatusFailed, telegramJobStatusSent,
		},
		To: telegramJobStatusSuperseded,
	})
	if err != nil {
		return gerror.Wrap(err, "废弃TG推送任务失败")
	}
	if !transitioned {
		return nil
	}
	if removeErr := (tgChannelPendingIndex{}).remove(ctx, job.TenantId, job.ChannelId, job.Id); removeErr != nil {
		g.Log().Warningf(ctx, "移除已废弃TG任务频道索引失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, removeErr)
	}
	if err := s.upsertPublishJobRecord(ctx, job, "superseded", ""); err != nil {
		return err
	}
	// 任务从任意业务入口被废弃后都必须唤醒同频道下一条任务。
	// 不能只依赖发送 Worker 的 defer，否则资料下架、编辑覆盖等入口会留下空闲队首。
	if err := s.wakeNextTelegramChannelJob(ctx, job); err != nil {
		return gerror.Wrap(err, "废弃TG任务后唤醒频道下一条任务失败")
	}
	if err := s.clearSupersededProfileOperationIfTerminal(ctx, job); err != nil {
		return err
	}
	return nil
}

// clearSupersededProfileOperationIfTerminal releases the durable operation lock
// after every job of the same operation has been superseded. The profile's
// business status is intentionally untouched.
func (s *sSysPublish) clearSupersededProfileOperationIfTerminal(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ProfileId <= 0 || strings.TrimSpace(job.OperationNo) == "" {
		return nil
	}
	var summary struct {
		Total      int `orm:"total"`
		Superseded int `orm:"superseded"`
		Active     int `orm:"active"`
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("operation_no", strings.TrimSpace(job.OperationNo)).
		Fields("COUNT(*) AS total, SUM(CASE WHEN status='superseded' THEN 1 ELSE 0 END) AS superseded, SUM(CASE WHEN status IN ('pending','sending','retry_wait','confirming','failed_retry','unknown') THEN 1 ELSE 0 END) AS active").
		Scan(&summary); err != nil {
		return gerror.Wrap(err, "检查已废弃资料操作状态失败")
	}
	if summary.Total == 0 || summary.Total != summary.Superseded || summary.Active != 0 {
		return nil
	}
	now := gtime.Now()
	if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Where("publish_operation_no", strings.TrimSpace(job.OperationNo)).
		WhereIn("publish_task_status", []string{"pending", "publishing", "sending"}).
		Data(g.Map{"publish_operation_no": "", "publish_task_status": "", "publish_task_updated_at": now, "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "清理已废弃资料操作状态失败")
	}
	if _, err := g.DB().Model(publishNoteIndexTable).Unscoped().Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).WhereIn("task_status", []string{"pending", "publishing", "sending"}).
		Data(g.Map{"task_status": "", "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "同步已废弃资料列表状态失败")
	}
	return nil
}

func (s *sSysPublish) supersedeTelegramJobAndCompleteOperation(ctx context.Context, job telegramJobRecord) error {
	if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
		return err
	}
	_, completeErr := s.completeProfileTelegramOperation(ctx, job, isCycleBatchOperation(job.OperationNo))
	return completeErr
}

func (s *sSysPublish) completeTelegramJobAndWakeChannel(ctx context.Context, job telegramJobRecord) error {
	if err := s.completeTelegramJob(ctx, job); err != nil {
		return err
	}
	if removeErr := (tgChannelPendingIndex{}).remove(ctx, job.TenantId, job.ChannelId, job.Id); removeErr != nil {
		g.Log().Warningf(ctx, "移除已完成TG任务频道索引失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, removeErr)
	}
	if err := s.wakeNextTelegramChannelJob(ctx, job); err != nil {
		g.Log().Warningf(ctx, "TG任务完成后唤醒频道下一条任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, err)
	}
	return nil
}

type telegramResubmitJob struct {
	Id           int64  `json:"id"`
	TaskId       int64  `json:"taskId"`
	OperationNo  string `json:"operationNo"`
	TenantId     int64  `json:"tenantId"`
	AccountId    int64  `json:"accountId"`
	ProfileId    int64  `json:"profileId"`
	ChannelId    int64  `json:"channelId"`
	BotId        int64  `json:"botId"`
	TargetChatId string `json:"targetChatId"`
	Status       string `json:"status"`
}

func (job telegramResubmitJob) telegramJobRecord() telegramJobRecord {
	return telegramJobRecord{
		Id:           job.Id,
		TaskId:       job.TaskId,
		OperationNo:  job.OperationNo,
		TenantId:     job.TenantId,
		AccountId:    job.AccountId,
		ProfileId:    job.ProfileId,
		ChannelId:    job.ChannelId,
		BotId:        job.BotId,
		TargetChatId: job.TargetChatId,
	}
}
