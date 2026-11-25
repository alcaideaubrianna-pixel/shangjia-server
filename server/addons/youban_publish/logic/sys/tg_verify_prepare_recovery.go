package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Only an absent verify Attempt proves preparation never crossed the durable
// send boundary. Existing/legacy verify Attempts must retain reconciliation.
func (s *sSysPublish) recoverTelegramVerifyPreparation(ctx context.Context, job telegramJobRecord, cutoff *gtime.Time) (bool, error) {
	if job.SendPhase != telegramSendPhaseDisplayConfirmed {
		return false, nil
	}
	recovered := false
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if current.Status != telegramJobStatusSending || current.SendPhase != telegramSendPhaseDisplayConfirmed {
			return nil
		}
		count, err := tx.Model(publishTgAttemptTable).Where("job_id", job.Id).Where("phase", "verify").Count()
		if err != nil || count != 0 {
			return err
		}
		recovered, err = s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
			JobId: job.Id, From: []JobStatus{telegramJobStatusSending}, To: telegramJobStatusRetryWait,
			UpdatedBefore:  cutoff,
			NotExpiredAt:   gtime.Now(),
			DispatchStatus: jobDispatch(tgDispatchStatusIdle),
			ErrorMessage:   "验证视频准备阶段中断，尚未创建验证Attempt，已恢复续传",
			Additional:     g.Map{"asynq_task_id": "", "last_dispatch_error": "验证视频准备阶段中断，等待安全续传"},
		})
		return err
	})
	if err != nil || !recovered {
		return recovered, err
	}
	g.Log().Info(ctx, "恢复未发起远端发送的验证视频准备任务", g.Map{
		"event": "telegram_verify_prepare_recovered", "jobId": job.Id,
		"profileId": job.ProfileId, "channelId": job.ChannelId, "reason": "no_verify_attempt",
	})
	// The durable idle job remains recoverable if Redis or rearming fails.
	if err := s.rearmTelegramJobOutbox(ctx, job.Id, 0); err != nil {
		return true, err
	}
	return true, s.EnqueueTelegramJob(ctx, job.Id, 0, false)
}
