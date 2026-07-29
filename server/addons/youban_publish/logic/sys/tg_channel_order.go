package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

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
		WhereIn("j.status", []string{"pending", "sending", "failed_retry"}).
		Where("(j.status <> 'failed_retry' OR j.next_retry_at IS NULL OR j.next_retry_at <= ?)", gtime.Now())
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
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道前序TG任务失败")
	}
	return count > 0, nil
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

func (s *sSysPublish) postponeTelegramJobForChannelContinuation(ctx context.Context, job telegramJobRecord) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       gtime.Now().Add(3 * time.Second),
			"last_dispatch_error": "等待同频道采集绑定视频优先推送",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "延后TG采集绑定顺序任务失败")
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
