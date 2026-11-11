package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"

	"hotgo/addons/youban_publish/library/streamgray"
)

// clearTelegramReadyStreamDedupe 重新激活任务时清理旧的传输幂等键，允许它再次进入 Stream。
func clearTelegramReadyStreamDedupe(ctx context.Context, jobID int64) {
	if jobID <= 0 || !cache.Initialized() {
		return
	}
	for _, stream := range []string{streamgray.ReadyRealtimeStream, streamgray.ReadyBulkStream} {
		key := fmt.Sprintf("%s%s:%d", publishconsts.TelegramReadyStreamDedupeKeyPrefix, stream, jobID)
		_, _ = cache.Instance().Remove(ctx, key)
	}
}

// publishTelegramReadyStream 将已持久化的任务发布为 Stream 就绪事件。
func (s *sSysPublish) publishTelegramReadyStream(ctx context.Context, job telegramJobRecord) error {
	// Outbox 可能被多个实例同时领取，写 Stream 前按 job+stream 做短期幂等。
	// Redis 已成功写入而数据库确认失败时，后续重试只需确认 Outbox，不应制造大量重复消息。
	stream := streamgray.StreamForOperation(job.OperationNo)
	if telegramJobPriorityValue(job) == tgJobPriorityHistory {
		stream = streamgray.ReadyBulkStream
	}
	dedupeKey := fmt.Sprintf("%s%s:%d", publishconsts.TelegramReadyStreamDedupeKeyPrefix, stream, job.Id)
	ok, err := cache.Instance().SetIfNotExist(ctx, dedupeKey, 1, 10*time.Minute)
	if err != nil {
		return gerror.Wrap(err, "记录TG任务 Stream 幂等键失败")
	}
	if !ok {
		return nil
	}
	producer, err := streamgray.NewRedisProducerFromCache()
	if err != nil {
		return err
	}
	event := streamgray.ReadyEvent{
		TenantID: job.TenantId, AccountID: job.AccountId, JobID: job.Id,
		ChannelID: job.ChannelId, Priority: job.Priority, QueueName: job.QueueName,
	}
	if _, err = streamgray.PublishReadyEvent(ctx, producer, stream, event); err != nil {
		_, _ = cache.Instance().Remove(ctx, dedupeKey)
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
