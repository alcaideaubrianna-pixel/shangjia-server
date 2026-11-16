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

// telegramChannelQueueDispatcher bounds local work across polling passes.
// The existing distributed channel lease still guards cross-process sends.
type telegramChannelQueueDispatcher struct {
	sem     chan struct{}
	mu      sync.Mutex
	running map[int64]struct{}
	wg      sync.WaitGroup
	next    int
}

// dispatch resumes after the last admitted channel so a fixed active list is fair.
// Only the scheduling loop calls dispatch; submit protects in-flight state.
func (d *telegramChannelQueueDispatcher) dispatch(ctx context.Context, channels []int64, process func(int64)) {
	if len(channels) == 0 {
		d.next = 0
		return
	}
	start := d.next % len(channels)
	for i := 0; i < len(channels); i++ {
		index := (start + i) % len(channels)
		if d.submit(ctx, channels[index], process) {
			d.next = (index + 1) % len(channels)
		}
	}
}

// runTelegramChannelQueueSchedule owns all submissions and waits only at shutdown.
// Recovery has its own single slot, so a slow recovery cannot stall polling.
func runTelegramChannelQueueSchedule(ctx context.Context, ticks, recoveryTicks <-chan time.Time, pass, recover func(context.Context)) {
	var recovery sync.WaitGroup
	busy := make(chan struct{}, 1)
	defer recovery.Wait()
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if ctx.Err() == nil {
				pass(ctx)
			}
		case <-recoveryTicks:
			if ctx.Err() != nil {
				return
			}
			select {
			case busy <- struct{}{}:
				recovery.Add(1)
				go func() {
					defer recovery.Done()
					defer func() { <-busy }()
					recover(ctx)
				}()
			default:
			}
		}
	}
}

func newTelegramChannelQueueDispatcher(limit int) *telegramChannelQueueDispatcher {
	if limit <= 0 {
		limit = 1
	}
	return &telegramChannelQueueDispatcher{
		sem:     make(chan struct{}, limit),
		running: make(map[int64]struct{}),
	}
}

func (d *telegramChannelQueueDispatcher) submit(ctx context.Context, channelID int64, process func(int64)) bool {
	d.mu.Lock()
	if ctx.Err() != nil || channelID <= 0 {
		d.mu.Unlock()
		return false
	}
	if _, exists := d.running[channelID]; exists {
		d.mu.Unlock()
		return false
	}
	select {
	case d.sem <- struct{}{}:
	default:
		d.mu.Unlock()
		return false
	}
	d.running[channelID] = struct{}{}
	d.wg.Add(1)
	d.mu.Unlock()

	go func() {
		defer d.wg.Done()
		defer func() {
			<-d.sem
			d.mu.Lock()
			delete(d.running, channelID)
			d.mu.Unlock()
		}()
		if ctx.Err() == nil {
			process(channelID)
		}
	}()
	return true
}

func (d *telegramChannelQueueDispatcher) wait() {
	d.wg.Wait()
}

func (s *sSysPublish) runTelegramChannelQueueWorker(ctx context.Context) {
	dispatcher := newTelegramChannelQueueDispatcher(channelQueueWorkerConcurrency())
	ticker := time.NewTicker(500 * time.Millisecond)
	// The Redis active-channel set is only a fast index. Jobs created before
	// the index write, or jobs whose Redis enqueue was interrupted, remain in
	// PostgreSQL with dispatch_status=idle. Recover those channel heads
	// frequently so one lost wake-up cannot leave an entire channel invisible
	// to the worker for minutes.
	recoveryTicker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer recoveryTicker.Stop()
	defer dispatcher.wait()
	runTelegramChannelQueueSchedule(ctx, ticker.C, recoveryTicker.C,
		func(ctx context.Context) { s.processTelegramChannelQueuePass(ctx, dispatcher) },
		func(ctx context.Context) {
			if err := s.rehydrateMissingTelegramChannelQueue(ctx, telegramChannelRecoveryBatchSize()); err != nil {
				g.Log().Warningf(ctx, "频道队列低频恢复失败 err:%+v", err)
			}
		})
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

func (s *sSysPublish) processTelegramChannelQueuePass(ctx context.Context, dispatcher *telegramChannelQueueDispatcher) {
	if !telegramChannelQueueEnabled() {
		return
	}
	channels, err := activeTelegramChannelIDs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "频道活跃集合读取失败 err:%+v", err)
		return
	}
	dispatcher.dispatch(ctx, channels, func(channelID int64) {
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
	})
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
