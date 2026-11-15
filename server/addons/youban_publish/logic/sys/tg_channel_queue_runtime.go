package sys

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// runTelegramChannelQueueWorker is the migration consumer for the logical
// per-channel Redis queues. It is opt-in so the legacy Asynq consumer remains
// the sole sender until a channel is explicitly migrated.
func (s *sSysPublish) runTelegramChannelQueueWorker(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	// The Redis active-channel set is only a fast index. Jobs created before
	// the index write, or jobs whose Redis enqueue was interrupted, remain in
	// PostgreSQL with dispatch_status=idle. Recover those channel heads
	// frequently so one lost wake-up cannot leave an entire channel invisible
	// to the worker for minutes.
	recoveryTicker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer recoveryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processTelegramChannelQueuePass(ctx)
		case <-recoveryTicker.C:
			if err := s.rehydrateMissingTelegramChannelQueue(ctx, telegramChannelRecoveryBatchSize()); err != nil {
				g.Log().Warningf(ctx, "频道队列低频恢复失败 err:%+v", err)
			}
		}
	}
}

func telegramChannelRecoveryBatchSize() int {
	const (
		defaultSize = 500
		maxSize     = 2000
	)
	raw := strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_TG_CHANNEL_RECOVERY_BATCH_SIZE"))
	if raw == "" {
		return defaultSize
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultSize
	}
	if value > maxSize {
		return maxSize
	}
	return value
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
	// Channel ordering is enforced by the per-channel Redis lease. Different
	// channels must run concurrently; a single global loop here otherwise turns
	// one slow upload into a backlog for every channel.
	limit := channelQueueWorkerConcurrency()
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, channelID := range channels {
		if channelID <= 0 {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			processed, processErr := s.processTelegramChannelQueueHead(ctx, channelID)
			if processErr != nil {
				g.Log().Warningf(ctx, "频道队列消费失败 channel:%d err:%+v", channelID, processErr)
				return
			}
			if !processed {
				normal, normalErr := g.Redis().Do(ctx, "ZCARD", telegramChannelQueueKey(channelID))
				urgent, urgentErr := g.Redis().Do(ctx, "ZCARD", telegramChannelUrgentQueueKey(channelID))
				history, historyErr := g.Redis().Do(ctx, "ZCARD", telegramChannelHistoryQueueKey(channelID))
				if normalErr == nil && urgentErr == nil && historyErr == nil && normal.Int()+urgent.Int()+history.Int() == 0 {
					if removeErr := removeActiveTelegramChannel(ctx, channelID); removeErr != nil {
						g.Log().Warningf(ctx, "频道活跃集合清理失败 channel:%d err:%+v", channelID, removeErr)
					}
				}
			}
		}()
	}
	wg.Wait()
}

func channelQueueWorkerConcurrency() int {
	limit := 128
	if raw := os.Getenv("YOUBAN_PUBLISH_CHANNEL_WORKER_CONCURRENCY"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}
	return limit
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
	if limit > 2000 {
		limit = 2000
	}
	// Recover one database head per tenant/channel, rather than taking the
	// oldest N jobs globally. A global LIMIT can select hundreds of rows from
	// the same few backlogged channels and leave every other channel absent
	// from the Redis active set. The channel worker can only make progress for
	// channels that have an active-set member, so this query must maximize
	// channel coverage first.
	err := g.DB().GetScan(ctx, &jobs, `
		SELECT DISTINCT ON (tenant_id, channel_id)
			id, channel_id, priority
		FROM `+publishTgJobTable+`
		WHERE channel_id > 0
		  AND status IN ('pending','failed_retry','unknown')
		  AND COALESCE(dispatch_status,'') IN ('idle','')
		ORDER BY tenant_id, channel_id, created_at ASC, id ASC
		LIMIT ?`, limit)
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
			Where("dispatch_status IS DISTINCT FROM ? OR queue_name IS DISTINCT FROM ?", tgDispatchStatusQueued, telegramChannelQueueKeyForPriority(job.ChannelId, job.Priority)).
			Data(g.Map{"dispatch_status": tgDispatchStatusQueued, "queue_name": telegramChannelQueueKeyForPriority(job.ChannelId, job.Priority), "updated_at": gtime.Now()}).Update()
		g.Log().Infof(ctx, "频道队列对账补入成功 channel:%d job:%d", job.ChannelId, job.Id)
	}
	return nil
}
