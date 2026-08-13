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
		Where(`(
			(j.status = 'sending' AND j.updated_at > ?) OR
			(j.status IN ('pending', 'failed_retry', 'unknown') AND j.dispatch_status = ? AND j.dispatched_at > ?) OR
			(j.status IN ('pending', 'failed_retry', 'unknown') AND j.dispatch_status = ? AND j.updated_at > ?)
		)`,
			gtime.Now().Add(-telegramSendingJobRecoverAfter),
			tgDispatchStatusQueued,
			gtime.Now().Add(-telegramDispatchJobRecoverAfter),
			tgDispatchStatusProcessing,
			gtime.Now().Add(-telegramDispatchJobRecoverAfter),
		)
	if job.ChannelId > 0 {
		mod = mod.Where("j.channel_id", job.ChannelId)
	} else {
		mod = mod.Where("j.target_chat_id", normalizeTelegramChannelChatID(job.TargetChatId))
	}
	if job.CollectSourceId > 0 && job.CollectSourceMessageId > 0 && strings.TrimSpace(job.CollectSourceChatId) != "" {
		mod = mod.Where(
			`j.collect_source_id = ? AND j.collect_source_chat_id = ? AND j.collect_source_message_id > 0 AND j.collect_source_message_id < ?`,
			job.CollectSourceId,
			strings.TrimSpace(job.CollectSourceChatId),
			job.CollectSourceMessageId,
		)
	} else {
		mod = mod.Where(`(j.created_at < ? OR (j.created_at = ? AND j.id < ?))`, createdAt, createdAt, job.Id)
	}
	if isTelegramUrgentJob(job) {
		mod = mod.Where("j.priority <= ?", tgJobPriorityUrgent)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	earlier, err := mod.Fields("j.id").Limit(1).One()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道前序TG任务失败")
	}
	return !earlier.IsEmpty(), nil
}

func (s *sSysPublish) wakeNextTelegramChannelJob(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.ChannelId <= 0 {
		return nil
	}
	now := gtime.Now()
	nextRecord, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("channel_id", job.ChannelId).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now).
		OrderAsc("priority").OrderAsc("id").
		Limit(1).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取频道下一条TG任务失败")
	}
	if nextRecord.IsEmpty() {
		return nil
	}
	var next telegramJobRecord
	if err = nextRecord.Struct(&next); err != nil {
		return gerror.Wrap(err, "解析频道下一条TG任务失败")
	}
	if next.Id <= 0 {
		return nil
	}
	if err = s.enqueueTelegramJob(ctx, next.Id, 0); err != nil {
		return gerror.Wrap(err, "唤醒频道下一条TG任务失败")
	}
	return nil
}

func telegramChannelDispatchKey(tenantId, channelId int64) string {
	return fmt.Sprintf("youban_publish:tg_channel_dispatch:%d:%d", tenantId, channelId)
}

func (s *sSysPublish) shouldEnqueueTelegramChannelJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.TenantId <= 0 || job.ChannelId <= 0 {
		return true, nil
	}
	lease := hglock.NewConfig(telegramChannelDispatchLeaseTTL, 100*time.Millisecond).
		Mutex(telegramChannelDispatchKey(job.TenantId, job.ChannelId))
	if err := lease.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return false, nil
		}
		return false, gerror.Wrap(err, "获取频道任务调度租约失败")
	}
	defer func() {
		if err := lease.Unlock(ctx); err != nil && !gerror.Is(err, hglock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放频道任务调度租约失败 tenantId:%d channelId:%d err:%+v", job.TenantId, job.ChannelId, err)
		}
	}()

	active, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("channel_id", job.ChannelId).
		Where("id <> ?", job.Id).
		Where(`(
			status = 'sending' OR
			(status IN ('pending', 'failed_retry', 'unknown') AND dispatch_status IN (?, ?))
		)`, tgDispatchStatusQueued, tgDispatchStatusProcessing).
		Fields("id").Limit(1).One()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道活动TG任务失败")
	}
	return active.IsEmpty(), nil
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
	return nil
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
	s.appendTelegramJobLog(ctx, job, "publish", "paused", "采集推送已暂停，任务保留等待恢复")
	return nil
}
