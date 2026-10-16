package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	hglock "hotgo/internal/library/hgrds/lock"
)

const telegramChannelDispatchLeaseTTL = 15 * time.Second
const telegramChannelActiveJobLimit = 1

func (s *sSysPublish) telegramChannelHasEarlierActiveJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if job.Id <= 0 {
		return false, nil
	}
	createdAt := job.CreatedAt
	if createdAt == nil {
		current, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Fields("created_at").
			Where("id", job.Id).
			One()
		if err != nil {
			return false, gerror.Wrap(err, "读取TG任务创建时间失败")
		}
		if current.IsEmpty() {
			return false, nil
		}
		createdAt = current["created_at"].GTime()
	}
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Unscoped().
		Where("j.id <> ?", job.Id).
		Where(telegramWebhookDetachedJobSQL("j.")).
		Where(telegramChannelBlockingJobSQL("j"))
	// Once a display is visible in Telegram, finish that operation before
	// returning it behind untouched historical backlog. Earlier operations that
	// have also started still block it, preserving media/verification ordering.
	if telegramJobRequiresChannelCompletionPriority(job.SendPhase) {
		mod = mod.Where(`(
			j.status IN (?, ?) OR
			j.send_phase IN (?, ?, ?)
		)`, telegramJobStatusSending, telegramJobStatusConfirming,
			telegramSendPhaseDisplaySending, telegramSendPhaseDisplayConfirmed,
			telegramSendPhaseVerifySending)
	}
	if job.ChannelId > 0 {
		mod = mod.Where("j.channel_id", job.ChannelId)
	} else {
		mod = mod.Where(
			"REPLACE(COALESCE(j.target_chat_id, ''), '-100', '') = ?",
			telegramChannelOrderChatKey(job.TargetChatId),
		)
	}
	// A destination channel is a single ordered stream. Source message IDs are
	// only comparable within one source channel, so using them here lets jobs
	// from another collection source bypass an older job between its display
	// and verification phases.
	mod = mod.Where(`(j.created_at < ? OR (j.created_at = ? AND j.id < ?))`, createdAt, createdAt, job.Id)
	// Priority never bypasses the destination channel's ordered stream.
	// Urgent jobs may win across different channels, but cannot overtake an
	// older display/verification operation on this channel.
	if err := ctx.Err(); err != nil {
		return false, err
	}
	earlier, err := mod.Fields("j.id").Limit(1).One()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道前序TG任务失败")
	}
	return !earlier.IsEmpty(), nil
}

func telegramChannelBlockingJobStatuses() []string {
	return []string{
		telegramJobStatusPending,
		telegramJobStatusRetryWait,
		telegramJobStatusConfirming,
		telegramJobStatusSending,
	}
}

func telegramChannelBlockingJobSQL(alias string) string {
	// Unknown jobs are reconciled independently. They must not hold the normal
	// channel queue hostage while waiting for a webhook or account fallback.
	// Jobs tied to an unavailable protocol account are likewise left durable but
	// excluded until the account is authorized again.
	return fmt.Sprintf(`(%s AND %s)`, telegramChannelAvailableAccountSQL(alias), telegramChannelBlockingJobStateSQL(alias))
}

func telegramChannelBlockingJobStateSQL(alias string) string {
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(`(%[1]sstatus IN ('pending','failed_retry','confirming','sending') OR
		(%[1]sstatus='failed' AND %[1]ssend_phase='display_confirmed' AND EXISTS (
			SELECT 1 FROM %[2]s channel_display
			WHERE channel_display.job_id=%[1]sid AND channel_display.purpose='display'
				AND channel_display.status='sent' AND channel_display.deleted_at IS NULL
		)))`, alias, publishTgMessageTable)
}

func telegramChannelAvailableAccountSQL(alias string) string {
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM %[2]s c INNER JOIN %[3]s a ON a.id=c.tg_account_id AND a.tenant_id=c.tenant_id AND a.status='authorized' AND NULLIF(BTRIM(a.session_key), '') IS NOT NULL AND a.deleted_at IS NULL WHERE c.id=%[1]schannel_id AND c.tenant_id=%[1]stenant_id AND c.status=1 AND c.deleted_at IS NULL) OR EXISTS (SELECT 1 FROM %[4]s tc INNER JOIN %[3]s a ON a.id=tc.tg_account_id AND a.tenant_id=tc.tenant_id AND a.status='authorized' AND NULLIF(BTRIM(a.session_key), '') IS NOT NULL AND a.deleted_at IS NULL WHERE tc.id=%[1]schannel_id AND tc.tenant_id=%[1]stenant_id))`, alias, publishChannelTable, publishTgAccountTable, publishTgChannelTable)
}

func telegramChannelOrderChatKey(chatID string) string {
	return strings.TrimPrefix(normalizeTelegramChannelChatID(chatID), "-100")
}

func telegramJobRequiresChannelCompletionPriority(sendPhase string) bool {
	return telegramSendPhaseHasDisplay(sendPhase) &&
		sendPhase != telegramSendPhaseVerifyConfirmed &&
		sendPhase != telegramSendPhaseCompletedNoVerify
}

func (s *sSysPublish) wakeNextTelegramChannelJob(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.ChannelId <= 0 {
		return nil
	}
	now := gtime.Now()
	var nextJobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("channel_id", job.ChannelId).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where(telegramWebhookDetachedJobSQL("")).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now).
		OrderAsc("created_at").OrderAsc("id").
		Limit(telegramChannelActiveJobLimit).
		Scan(&nextJobs)
	if err != nil {
		return gerror.Wrap(err, "读取频道待入队TG任务失败")
	}
	for _, next := range nextJobs {
		if next.Id <= 0 {
			continue
		}
		if err = s.enqueueTelegramJob(ctx, next.Id, 0); err != nil {
			return gerror.Wrap(err, "唤醒频道待入队TG任务失败")
		}
	}
	return nil
}

func telegramChannelDispatchKey(tenantId, channelId int64) string {
	return fmt.Sprintf("youban_publish:tg_channel_dispatch:%d:%d", tenantId, channelId)
}

func (s *sSysPublish) withTelegramChannelDispatchLease(ctx context.Context, job telegramJobRecord, reserve func() (bool, error)) (bool, error) {
	if job.TenantId <= 0 || job.ChannelId <= 0 {
		return reserve()
	}
	lease := hglock.NewConfig(telegramChannelDispatchLeaseTTL, 100*time.Millisecond).
		Mutex(telegramChannelDispatchKey(job.TenantId, job.ChannelId))
	if err := lease.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			// Lock contention or an unavailable Redis lock backend is not a
			// successful enqueue. Returning nil here silently stranded the
			// Outbox in pending/idle forever. Let the caller record the error,
			// release the lease, and retry with backoff.
			return false, gerror.Wrap(err, "获取频道任务调度租约失败，稍后重试")
		}
		return false, gerror.Wrap(err, "获取频道任务调度租约失败")
	}
	defer func() {
		if err := lease.Unlock(ctx); err != nil && !gerror.Is(err, hglock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放频道任务调度租约失败 tenantId:%d channelId:%d err:%+v", job.TenantId, job.ChannelId, err)
		}
	}()
	return reserve()
}

func (s *sSysPublish) telegramChannelHasDispatchCapacity(ctx context.Context, job telegramJobRecord) (bool, error) {
	active, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("channel_id", job.ChannelId).
		Where("id <> ?", job.Id).
		Where(telegramWebhookDetachedJobSQL("")).
		Where(`(
			status = 'sending' OR
			(status IN ('pending', 'failed_retry', 'unknown') AND dispatch_status IN (?, ?))
		)`, tgDispatchStatusQueued, tgDispatchStatusProcessing).
		Fields("id").Limit(telegramChannelActiveJobLimit).Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道活动TG任务失败")
	}
	return active < telegramChannelActiveJobLimit, nil
}

func (s *sSysPublish) postponeTelegramJobForChannelOrder(ctx context.Context, job telegramJobRecord) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       gtime.Now().Add(3 * time.Second),
			"last_dispatch_error": "等待同频道前序推送任务完成",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "延后TG频道顺序任务失败")
	}
	return s.rearmTelegramJobOutbox(ctx, job.Id, 3*time.Second)
}

func (s *sSysPublish) postponeTelegramJobForCollectPushPause(ctx context.Context, job telegramJobRecord) error {
	nextRetryAt := gtime.Now().Add(30 * time.Second)
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       nextRetryAt,
			"last_dispatch_error": "采集推送已暂停，等待重新开启",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "延后采集推送任务失败")
	}
	if err = s.rearmTelegramJobOutbox(ctx, job.Id, 30*time.Second); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "publish", "paused", "采集推送已暂停，任务保留等待恢复")
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "采集推送暂停后唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return nil
}

// Ambiguous sends remain an ordering barrier until Webhook or account
// reconciliation confirms the message or moves the job to a terminal state.
func telegramWebhookDetachedJobSQL(_ string) string {
	return "1=1"
}
