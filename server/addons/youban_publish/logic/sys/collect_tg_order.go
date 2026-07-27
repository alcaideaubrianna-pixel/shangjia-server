package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) collectTelegramJobHasPreviousActive(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.CollectSourceId <= 0 || job.CollectSourceMessageId <= 0 || strings.TrimSpace(job.CollectSourceChatId) == "" {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Where("j.id <> ?", job.Id).
		Where("j.collect_source_id", job.CollectSourceId).
		Where("j.collect_source_chat_id", strings.TrimSpace(job.CollectSourceChatId)).
		Where("j.collect_source_message_id < ?", job.CollectSourceMessageId).
		WhereIn("j.status", []string{"pending", "sending", "failed_retry"})
	if job.ChannelId > 0 {
		mod = mod.Where("j.channel_id", job.ChannelId)
	} else {
		mod = mod.Where("j.target_chat_id", job.TargetChatId)
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集TG前序任务失败")
	}
	return count > 0, nil
}
