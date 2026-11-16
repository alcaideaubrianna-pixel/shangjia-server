package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) settleStreamFailedMediaJob(ctx context.Context, job telegramJobRecord, causes ...error) (bool, error) {
	// 不得用媒体校验覆盖已开始发送或未知结果的任务。
	if !telegramJobLifecycleNeedsRetry(job.Status.String()) || strings.TrimSpace(job.SendPhase.String()) != "" || job.ProfileId <= 0 {
		return false, nil
	}
	var cause error
	if len(causes) > 0 {
		cause = causes[0]
	} else {
		_, cause = s.profileMediaReady(ctx, job.ProfileId)
	}
	if !isProfileMediaValidationError(cause) {
		return false, cause
	}
	message := cause.Error()
	changed := false
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		changed, err = s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
			JobId: job.Id, From: telegramJobRetryableStatuses(),
			To: telegramJobStatusFailed, SendPhases: []SendPhase{""}, ErrorMessage: message,
			Additional: g.Map{"last_dispatch_error": message},
		})
		if err != nil || !changed {
			return err
		}
		if err = s.upsertPublishJobRecord(ctx, job, telegramJobStatusFailed, message); err != nil {
			return err
		}
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	})
	if err != nil {
		return false, gerror.Wrap(err, "终止媒体校验不合格推送任务失败")
	}
	if !changed {
		return false, nil
	}
	s.appendTelegramJobLog(ctx, job, "media_ready", "failed", message)
	g.Log().Warning(ctx, "资料媒体校验不合格，已终止推送", g.Map{
		"event": "publish_media_validation_failed", "jobId": job.Id, "profileId": job.ProfileId,
		"channelId": job.ChannelId, "operationNo": job.OperationNo, "error": message,
	})
	if err := s.wakeNextTelegramChannelJob(ctx, job); err != nil {
		g.Log().Warningf(ctx, "媒体校验失败后唤醒频道失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, err)
	}
	return true, nil
}

// settleMissingTelegramProfileJob closes a stale queue head created for a
// profile that has already been deleted. It is safe before any Telegram send
// has started and immediately wakes the next job for the same channel.
func (s *sSysPublish) settleMissingTelegramProfileJob(ctx context.Context, job telegramJobRecord) error {
	message := "资料已删除，跳过本次推送"
	changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []JobStatus{telegramJobStatusPending, telegramJobStatusRetryWait},
		To: telegramJobStatusFailed, SendPhases: []SendPhase{""}, ErrorMessage: message,
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
