package sys

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

func (s *sSysPublish) runTelegramOutboxDispatcher(ctx context.Context) {
	// 手动/BOT 上架需要在提交后尽快进入发送队列；持久化 Outbox
	// 仍负责崩溃恢复，因此将兜底轮询缩短到 200ms，避免用户等待整秒。
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.runTelegramOutboxPass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
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
	if err := s.recoverPendingIdleTelegramJobs(passCtx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复待入队TG任务失败：%+v", err)
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
