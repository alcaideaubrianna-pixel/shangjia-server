package sys

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

// telegramOutboxRecoveryBatchSize 控制异常恢复每轮领取数量；即时事件
// 唤醒和正常频道 Stream 调度不受该值影响。
func telegramOutboxRecoveryBatchSize() int {
	// Recovery is the safety net for jobs that were persisted before Redis/Asynq
	// became unavailable. A batch of 10 every five seconds leaves large
	// backlogs effectively stalled (only ~2 jobs/s) and can starve realtime
	// delivery behind historical rows. Keep the scan bounded, but restore a
	// useful refill rate by default; operators can still tune it via ENV.
	const defaultSize = 500
	value, err := strconv.Atoi(strings.TrimSpace(g.Cfg().MustGet(context.Background(), "youbanPublish.queue.tgRecoveryBatchSize", defaultSize).String()))
	if err != nil || value <= 0 {
		return defaultSize
	}
	if value > 500 {
		return 500
	}
	return value
}

func (s *sSysPublish) runTelegramOutboxDispatcher(ctx context.Context) {
	// 手动/BOT 上架通过事件立即唤醒；定时器只负责进程重启、Redis
	// 抖动等异常后的持久化兜底，降低无效轮询对数据库的压力。
	// New Outbox rows wake the dispatcher immediately. The ticker is only a
	// recovery safety net, so keep it low frequency to avoid repeated full
	// channel-head scans while preserving realtime latency.
	ticker := time.NewTicker(telegramOutboxScanInterval())
	defer ticker.Stop()
	for {
		s.runTelegramOutboxPass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-s.telegramOutboxWake:
			// 新任务写入 Outbox 后立即调度；定时器仍负责崩溃和丢信号恢复。
		case <-ticker.C:
		}
	}
}

func (s *sSysPublish) wakeTelegramOutbox() {
	if s == nil || s.telegramOutboxWake == nil {
		return
	}
	select {
	case s.telegramOutboxWake <- struct{}{}:
	default:
	}
}

func (s *sSysPublish) startTelegramOutboxDispatcher(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.telegramOutboxStarted {
		s.tgQueueMu.Unlock()
		return
	}
	s.telegramOutboxStarted = true
	s.tgQueueMu.Unlock()
	g.Log().Info(ctx, "TG Outbox dispatcher started")
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				g.Log().Errorf(ctx, "TG Outbox调度器异常退出，正在自动恢复：%v", recovered)
				s.tgQueueMu.Lock()
				s.telegramOutboxStarted = false
				s.tgQueueMu.Unlock()
				if ctx.Err() == nil {
					time.Sleep(time.Second)
					s.startTelegramOutboxDispatcher(ctx)
				}
			}
		}()
		s.runTelegramOutboxDispatcher(ctx)
	}()
}

func (s *sSysPublish) runTelegramOutboxPass(ctx context.Context) {
	passCtx, cancel := context.WithTimeout(ctx, telegramOutboxQueryTimeout+telegramOutboxLeaseTTL)
	defer cancel()
	// Outbox 可能已经持久化，但 Redis/Asynq 任务在进程重启、Redis抖动或
	// 稳定 TaskID 冲突时丢失。每轮先用有界恢复补灌 pending/idle 任务，
	// 再处理普通到期 Outbox，避免任务永久停在 pending 且没有 asynq_task_id。
	// Unified channel queues own TG publish recovery. The legacy DB scan only
	// understands the old Asynq dispatch state and performs an expensive query
	// every pass; running it alongside channel ZSET dispatch can requeue the
	// same jobs and add database pressure. Channel rehydration remains the
	// bounded repair path for missing Redis members.
	if !telegramChannelQueueEnabled() {
		if err := s.recoverPendingIdleTelegramJobs(passCtx, telegramOutboxRecoveryBatchSize()); err != nil {
			g.Log().Warningf(ctx, "恢复待入队TG任务失败：%+v", err)
		}
	}
	if err := s.dispatchDueTelegramJobOutbox(passCtx, 0); err != nil {
		g.Log().Warningf(ctx, "调度TG任务Outbox失败：%+v", err)
	}
}

// RecoverTelegramDispatch runs one bounded scheduler pass. It is used by
// operational repair tooling and never sends Telegram messages itself.
func (s *sSysPublish) RecoverTelegramDispatch(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	if err := s.recoverPendingIdleTelegramJobs(ctx, limit); err != nil {
		return err
	}
	return s.dispatchDueTelegramJobOutbox(ctx, limit)
}

// RecoverTelegramJobDispatch schedules one existing job through its Outbox,
// preserving available_at and the normal idempotent queue path.
func (s *sSysPublish) RecoverTelegramJobDispatch(ctx context.Context, jobId int64) error {
	return s.dispatchTelegramJobOutboxByJob(ctx, jobId)
}

func telegramOutboxOwner() string {
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = "worker"
	}
	return hostname + ":" + guid.S()
}
