package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/library/streamgray"
)

// publishTelegramReadyStream 将已持久化的任务发布为 Stream 就绪事件。
func (s *sSysPublish) publishTelegramReadyStream(ctx context.Context, job telegramJobRecord) error {
	producer, err := streamgray.NewRedisProducerFromCache()
	if err != nil {
		return err
	}
	event := streamgray.ReadyEvent{
		TenantID: job.TenantId, AccountID: job.AccountId, JobID: job.Id,
		ChannelID: job.ChannelId, Priority: job.Priority, QueueName: job.QueueName,
	}
	stream := streamgray.StreamForOperation(job.OperationNo)
	if telegramJobPriorityValue(job) == tgJobPriorityHistory {
		stream = streamgray.ReadyBulkStream
	}
	if _, err = streamgray.PublishReadyEvent(ctx, producer, stream, event); err != nil {
		return gerror.Wrap(err, "发布TG任务 Redis Stream 就绪事件失败")
	}
	return nil
}

// markTelegramOutboxDelivered 收口已经写入 Stream 的 Outbox，不执行 Asynq 入队。
func (s *sSysPublish) markTelegramOutboxDelivered(ctx context.Context, outboxID int64, owner string, jobID int64) error {
	_, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", outboxID).Where("status", telegramOutboxProcessing).Where("lease_owner", owner).
		Data(g.Map{"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil, "delivered_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "确认TG任务 Stream 投递失败")
	}
	g.Log().Info(ctx, "TG Outbox Stream 投递成功", g.Map{"event": "stream_ready_published", "outboxId": outboxID, "jobId": jobID})
	return nil
}
