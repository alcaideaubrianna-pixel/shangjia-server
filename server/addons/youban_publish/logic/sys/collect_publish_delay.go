package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) collectRealtimePushDelay(ctx context.Context, task gdb.Record) time.Duration {
	if task.IsEmpty() || task["collect_source_id"].Int64() <= 0 {
		return 0
	}
	conf, err := s.CollectConfig(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取实时采集推送延迟配置失败：%+v", err)
		return time.Minute
	}
	if conf == nil {
		return time.Minute
	}
	seconds := normalizeCollectRealtimePushDelaySec(conf.RealtimePushDelaySec)
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (s *sSysPublish) collectTelegramJobHasPreviousActiveTask(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.CollectSourceId <= 0 || job.CollectSourceMessageId <= 0 || strings.TrimSpace(job.CollectSourceChatId) == "" {
		return false, nil
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id <> ?", job.TaskId).
		Where("collect_source_id", job.CollectSourceId).
		Where("collect_source_chat_id", strings.TrimSpace(job.CollectSourceChatId)).
		Where("collect_source_message_id < ?", job.CollectSourceMessageId).
		WhereIn("status", []string{sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing}).
		WhereIn("tg_status", []string{"pending", "sending", "failed_retry"}).
		WhereNull("deleted_at")
	if since := collectEventOrderWindowSince(ctx); since != nil {
		mod = mod.WhereGTE("submitted_at", since)
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集TG前序上架任务失败")
	}
	return count > 0, nil
}
