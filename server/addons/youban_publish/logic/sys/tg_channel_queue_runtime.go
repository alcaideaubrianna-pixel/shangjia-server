package sys

import (
	"context"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
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
	// Rehydrate jobs created before the channel queue cutover. They may have a
	// delivered outbox but no ZSET member, so the normal outbox dispatcher will
	// never see them again. Only non-active jobs are eligible.
	if err = s.rehydrateMissingTelegramChannelQueue(ctx, 500); err != nil {
		g.Log().Warningf(ctx, "频道队列历史任务补入失败 err:%+v", err)
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

func (s *sSysPublish) rehydrateMissingTelegramChannelQueue(ctx context.Context, limit int) error {
	var jobs []struct {
		Id        int64 `json:"id"`
		ChannelId int64 `json:"channel_id"`
		Priority  int   `json:"priority"`
	}
	err := g.DB().Model(publishTgJobTable).Ctx(ctx).
		Fields("id,channel_id,priority").
		Where("channel_id > 0").
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("COALESCE(dispatch_status,'') IN ('','idle','queued')").
		Where("COALESCE(asynq_task_id,'') = ''").
		OrderAsc("updated_at").Limit(limit).Scan(&jobs)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, job := range jobs {
		member := strconv.FormatInt(job.Id, 10)
		value, scoreErr := g.Redis().Do(ctx, "ZSCORE", telegramChannelQueueKey(job.ChannelId), member)
		if scoreErr != nil || !value.IsNil() {
			continue
		}
		score := now*1000 + int64(max(0, 100000-job.Priority))
		if err = enqueueTelegramChannelJob(ctx, job.ChannelId, job.Id, score); err != nil {
			continue
		}
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Where("COALESCE(asynq_task_id,'') = ''").
			Data(g.Map{"dispatch_status": tgDispatchStatusQueued, "queue_name": telegramChannelQueueKey(job.ChannelId), "updated_at": gtime.Now()}).Update()
	}
	return nil
}
