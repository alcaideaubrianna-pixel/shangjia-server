package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) settleStreamFailedMediaJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	// 单个媒体处理失败不再终止整份资料的发布。发送阶段会根据媒体是否存在
	// 原始文件或 Telegram 缓存继续发送，真正不可发送的媒体由发送结果单独处理。
	return false, nil
}

// settleMissingTelegramProfileJob closes a stale queue head created for a
// profile that has already been deleted. It is safe before any Telegram send
// has started and immediately wakes the next job for the same channel.
func (s *sSysPublish) settleMissingTelegramProfileJob(ctx context.Context, job telegramJobRecord) error {
	message := "资料已删除，跳过本次推送"
	changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []string{telegramJobStatusPending, telegramJobStatusRetryWait},
		To: telegramJobStatusFailed, SendPhases: []string{""}, ErrorMessage: message,
		Additional: g.Map{"last_dispatch_error": message},
	})
	if err != nil {
		return gerror.Wrap(err, "终止失效资料推送任务失败")
	}
	if !changed {
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "media_ready", "failed", message)
	if recordErr := s.upsertPublishJobRecord(ctx, job, "failed", message); recordErr != nil {
		g.Log().Warningf(ctx, "记录失效资料推送失败结果失败 jobId:%d profileId:%d err:%+v", job.Id, job.ProfileId, recordErr)
	}
	_ = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "失效资料任务终止后唤醒频道失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return nil
}
