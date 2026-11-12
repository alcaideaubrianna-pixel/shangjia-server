package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	publishdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// streamMediaFailureCanSettle 只允许终止尚未开始外部发送的任务，部分成功任务仍交由原对账流程处理。
func streamMediaFailureCanSettle(job telegramJobRecord) bool {
	return (job.Status == telegramJobStatusPending || job.Status == telegramJobStatusRetryWait) && strings.TrimSpace(job.SendPhase) == ""
}

// settleStreamFailedMediaJob 将媒体终态失败收口为可见失败，避免频道永远等待不会再次完成的媒体任务。
// 该检查适用于所有发送引擎；只有 pending/retry_wait 且尚未开始发送的任务
// 才能安全终止，processing/retry 媒体继续等待既有媒体恢复和就绪唤醒。
func (s *sSysPublish) settleStreamFailedMediaJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	if !streamMediaFailureCanSettle(job) {
		return false, nil
	}
	columns := publishdao.YoubanPublishMedia.Columns()
	failed, err := publishdao.YoubanPublishMedia.Ctx(ctx).
		Where(columns.ProfileId, job.ProfileId).Where(publishdao.YoubanPublishMediaProcessingStatusColumn, mediaProcessingFailed).Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查资料媒体终态失败")
	}
	if failed == 0 {
		return false, nil
	}
	message := "资料媒体处理已终态失败，本次推送停止；修复媒体后可重新上架"
	changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []string{telegramJobStatusPending, telegramJobStatusRetryWait},
		To: telegramJobStatusFailed, SendPhases: []string{""}, ErrorMessage: message,
		Additional: g.Map{"last_dispatch_error": message, "dispatch_status": "done", "send_phase": "skipped_missing_profile"},
	})
	if err != nil || !changed {
		return true, err
	}
	s.appendTelegramJobLog(ctx, job, "media_ready", "failed", message)
	if recordErr := s.upsertPublishJobRecord(ctx, job, "failed", message); recordErr != nil {
		g.Log().Warningf(ctx, "记录媒体终态失败推送结果失败 jobId:%d profileId:%d err:%+v", job.Id, job.ProfileId, recordErr)
	}
	stateErr := s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	wakeErr := s.wakeNextTelegramChannelJob(ctx, job)
	if wakeErr != nil {
		g.Log().Warningf(ctx, "媒体终态失败后唤醒频道失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return true, stateErr
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
