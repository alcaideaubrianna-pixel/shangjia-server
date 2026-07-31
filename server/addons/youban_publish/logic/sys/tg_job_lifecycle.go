package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) deleteTelegramJobMessagesForResubmit(ctx context.Context, job telegramResubmitJob) error {
	return s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), "编辑资料")
}

func (s *sSysPublish) markTelegramJobSuperseded(ctx context.Context, jobId int64) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Data(g.Map{
			"status":          "superseded",
			"dispatch_status": tgDispatchStatusDone,
			"next_retry_at":   nil,
			"updated_at":      gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "废弃TG推送任务失败")
	}
	return nil
}

func (s *sSysPublish) supersedeTelegramJobAndCompleteOperation(ctx context.Context, job telegramJobRecord) error {
	if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
		return err
	}
	_, err := s.completeProfileTelegramOperation(ctx, job, isCycleBatchOperation(job.OperationNo))
	return err
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
