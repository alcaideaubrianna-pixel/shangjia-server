package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// runTelegramChannelQueueWorker is the migration consumer for the logical
// per-channel Redis queues. It is opt-in so the legacy Asynq consumer remains
// the sole sender until a channel is explicitly migrated.
func (s *sSysPublish) runTelegramChannelQueueWorker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processTelegramChannelQueuePass(ctx)
		}
	}
}

func (s *sSysPublish) processTelegramChannelQueuePass(ctx context.Context) {
	if !telegramChannelQueueEnabled() {
		return
	}
	var channels []struct {
		ChannelID int64 `json:"channel_id"`
	}
	err := g.DB().Model(publishTgJobTable).Ctx(ctx).
		Fields("channel_id").
		Where("channel_id > 0 AND status IN ('pending','failed_retry','unknown')").
		Group("channel_id").OrderAsc("channel_id").Limit(200).Scan(&channels)
	if err != nil {
		g.Log().Warningf(ctx, "频道队列扫描失败 err:%+v", err)
		return
	}
	for _, channel := range channels {
		if channel.ChannelID <= 0 {
			continue
		}
		if _, err = s.processTelegramChannelQueueHead(ctx, channel.ChannelID); err != nil {
			g.Log().Warningf(ctx, "频道队列消费失败 channel:%d err:%+v", channel.ChannelID, err)
		}
	}
}
