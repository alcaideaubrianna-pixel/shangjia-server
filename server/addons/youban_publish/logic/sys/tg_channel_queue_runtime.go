package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// runTelegramChannelQueueWorker is the migration consumer for the logical
// per-channel Redis queues. It is opt-in so the legacy Asynq consumer remains
// the sole sender until a channel is explicitly migrated.
func (s *sSysPublish) runTelegramChannelQueueWorker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	recoveryTicker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer recoveryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processTelegramChannelQueuePass(ctx)
		case <-recoveryTicker.C:
			if err := s.rehydrateMissingTelegramChannelQueue(ctx, 500); err != nil {
				g.Log().Warningf(ctx, "频道队列低频恢复失败 err:%+v", err)
			}
		}
	}
}

func (s *sSysPublish) processTelegramChannelQueuePass(ctx context.Context) {
	if !telegramChannelQueueEnabled() {
		return
	}
	channels, err := activeTelegramChannelIDs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "频道活跃集合读取失败 err:%+v", err)
		return
	}
	for _, channelID := range channels {
		if channelID <= 0 {
			continue
		}
		processed, processErr := s.processTelegramChannelQueueHead(ctx, channelID)
		if processErr != nil {
			g.Log().Warningf(ctx, "频道队列消费失败 channel:%d err:%+v", channelID, processErr)
			continue
		}
		if !processed {
			normal, normalErr := g.Redis().Do(ctx, "ZCARD", telegramChannelQueueKey(channelID))
			urgent, urgentErr := g.Redis().Do(ctx, "ZCARD", telegramChannelUrgentQueueKey(channelID))
			if normalErr == nil && urgentErr == nil && normal.Int()+urgent.Int() == 0 {
				if removeErr := removeActiveTelegramChannel(ctx, channelID); removeErr != nil {
					g.Log().Warningf(ctx, "频道活跃集合清理失败 channel:%d err:%+v", channelID, removeErr)
				}
			}
		}
	}
}

// telegramChannelCursorNext returns the last channel visited by a pass. It is
// kept separate from the database query so cursor wrap behavior is unit
// testable without a live database.
type telegramChannelQueueChannel struct {
	ChannelID int64 `json:"channel_id"`
}

func telegramChannelCursorNext(current int64, channels []telegramChannelQueueChannel) int64 {
	if len(channels) == 0 {
		return 0
	}
	next := current
	for _, channel := range channels {
		if channel.ChannelID > next {
			next = channel.ChannelID
		}
	}
	return next
}

func (s *sSysPublish) rehydrateMissingTelegramChannelQueue(ctx context.Context, limit int) error {
	var jobs []struct {
		Id        int64 `json:"id"`
		ChannelId int64 `json:"channel_id"`
		Priority  int   `json:"priority"`
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	// Reconcile queued rows independently of Outbox status. Outbox may already
	// be marked delivered after a Redis timeout, while the channel ZSET member
	// was never persisted. Prioritize this exact failure state so a large
	// historical backlog cannot starve realtime/manual jobs.
	err := g.DB().Model(publishTgJobTable).Ctx(ctx).
		Fields("id,channel_id,priority").
		Where("channel_id > 0").
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("COALESCE(dispatch_status,'') IN ('queued','idle','')").
		Order("CASE WHEN COALESCE(dispatch_status,'')='queued' THEN 0 ELSE 1 END, updated_at ASC, id ASC").
		Limit(limit).Scan(&jobs)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err = enqueueTelegramChannelJob(ctx, job.ChannelId, job.Id, job.Priority, time.Now()); err != nil {
			g.Log().Warningf(ctx, "频道队列对账补入失败 channel:%d job:%d err:%+v", job.ChannelId, job.Id, err)
			continue
		}
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Data(g.Map{"dispatch_status": tgDispatchStatusQueued, "queue_name": telegramChannelQueueKeyForPriority(job.ChannelId, job.Priority), "updated_at": gtime.Now()}).Update()
		g.Log().Infof(ctx, "频道队列对账补入成功 channel:%d job:%d", job.ChannelId, job.Id)
	}
	return nil
}
