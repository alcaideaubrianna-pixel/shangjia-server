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
	if job.Id <= 0 {
		return false, nil
	}
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
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id <> ?", job.Id).
		WhereIn("status", []string{"pending", "sending", "failed_retry"})
	if job.ChannelId > 0 {
		mod = mod.Where("channel_id", job.ChannelId)
	} else {
		mod = mod.Where("target_chat_id", normalizeTelegramChannelChatID(job.TargetChatId))
	}
	createdAt := current["created_at"].GTime()
	if job.CollectSourceId > 0 && job.CollectSourceMessageId > 0 && strings.TrimSpace(job.CollectSourceChatId) != "" {
		mod = mod.Where(
			`((collect_source_id = ? AND collect_source_chat_id = ? AND collect_source_message_id > 0 AND collect_source_message_id < ?) OR ((collect_source_id <> ? OR collect_source_chat_id <> ? OR collect_source_message_id = 0) AND (created_at < ? OR (created_at = ? AND id < ?))))`,
			job.CollectSourceId,
			strings.TrimSpace(job.CollectSourceChatId),
			job.CollectSourceMessageId,
			job.CollectSourceId,
			strings.TrimSpace(job.CollectSourceChatId),
			createdAt,
			createdAt,
			job.Id,
		)
	} else {
		mod = mod.Where(`(created_at < ? OR (created_at = ? AND id < ?))`, createdAt, createdAt, job.Id)
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
