package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	telegramSendingJobRecoverAfter = telegramPublishTaskTimeout + 2*time.Minute
	// A media send has its own 60/90 second deadline. Recover its attempt
	// independently of the five-minute queue task timeout so an interrupted
	// worker cannot hold the channel ordering barrier for several minutes.
	telegramSendingAttemptRecoverAfter = 2 * time.Minute
	telegramPreAttemptRecoverAfter     = 2 * time.Minute
	telegramConfirmedJobRecoverAfter   = 30 * time.Second
	telegramPendingJobRecoverAfter     = 30 * time.Second
	telegramDispatchJobRecoverAfter    = 5 * time.Minute
	telegramMediaFallbackRecoverAfter  = 2 * time.Minute
	telegramMediaFallbackMaxBackoff    = 30 * time.Minute
	messagePushPlanJobExpireAfter      = 30 * time.Minute
)

const expiredMessagePushPlanJobMessage = "历史消息计划任务已过期，系统自动终止"

func (s *sSysPublish) recoverInterruptedTelegramJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	if err := s.supersedeExpiredMessagePushPlanJobs(ctx, limit); err != nil {
		return err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Where(telegramActiveChannelCondition()).
		Where(messagePushPlanRecoveryConditionSQL(publishTgJobTable), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       now,
			"last_dispatch_error": "服务启动时发现任务调度中断，已重新进入待调度队列",
			"updated_at":          now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "恢复调度中断的TG任务失败")
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		g.Log().Infof(ctx, "已恢复调度中断的TG任务：%d条", affected)
	}
	if err = s.recoverPendingIdleTelegramJobs(ctx, limit); err != nil {
		return err
	}
	if err = s.recoverStaleTelegramDispatchJobs(ctx, limit); err != nil {
		return err
	}
	if err = s.recoverStaleTelegramMediaFallbackJobs(ctx, limit); err != nil {
		return err
	}
	return s.recoverStaleTelegramSendingJobs(ctx, limit)
}

func (s *sSysPublish) runTelegramJobRecovery(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	startup := time.NewTimer(15 * time.Second)
	defer startup.Stop()
	select {
	case <-ctx.Done():
		return
	case <-startup.C:
	}
	s.runTelegramJobRecoveryCycle(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTelegramJobRecoveryCycle(ctx)
		}
	}
}

func (s *sSysPublish) runTelegramJobRecoveryCycle(ctx context.Context) {
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	distributedLock := hglock.NewConfig(2*time.Minute, 100*time.Millisecond).Mutex("youban_publish:telegram:job_recovery")
	if err := distributedLock.TryLock(lockCtx); err != nil {
		if !errors.Is(err, hglock.ErrLockFailed) && ctx.Err() == nil {
			g.Log().Warningf(ctx, "获取TG推送恢复任务锁失败：%+v", err)
		}
		return
	}
	defer func() { _ = distributedLock.Unlock(context.Background()) }()

	if err := s.supersedeExpiredMessagePushPlanJobs(ctx, 1000); err != nil {
		g.Log().Warningf(ctx, "终止过期历史消息计划任务失败：%+v", err)
	}
	if err := s.recoverStalePreAttemptTelegramJobs(ctx, 200); err != nil {
		g.Log().Warningf(ctx, "恢复发送Attempt创建前中断的TG任务失败：%+v", err)
	}
	if err := s.recoverConfirmedTelegramJobs(ctx, 200); err != nil {
		g.Log().Warningf(ctx, "收口已确认TG推送任务失败：%+v", err)
	}
	if err := s.recoverStaleTelegramSendingJobs(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败：%+v", err)
	}
	if err := s.recoverStaleTelegramMediaFallbackJobs(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复协议号媒体降级任务失败：%+v", err)
	}
	if err := s.requeueExpiredTelegramAttempts(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "重新投递过期TG发送Attempt失败：%+v", err)
	}
	if err := s.recoverStaleSendingTelegramAttempts(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复中断的TG发送Attempt失败：%+v", err)
	}
	if err := s.recoverTerminalTelegramAttempts(ctx, 500); err != nil {
		g.Log().Warningf(ctx, "收敛TG发送Attempt状态失败：%+v", err)
	}
	if err := s.recoverFailedJobsMissingVerification(ctx, 500); err != nil {
		g.Log().Warningf(ctx, "恢复缺少验证视频的终态TG任务失败：%+v", err)
	}
	if err := s.recoverExpiredTelegramOutboxDispatches(ctx, 200); err != nil {
		g.Log().Warningf(ctx, "恢复Outbox入队确认中断的TG任务失败：%+v", err)
	}
	if err := s.recoverPendingIdleTelegramJobs(ctx, 500); err != nil {
		g.Log().Warningf(ctx, "恢复待入队TG推送任务失败：%+v", err)
	}
	if err := s.recoverStaleTelegramDispatchJobs(ctx, 200); err != nil {
		g.Log().Warningf(ctx, "恢复调度中断TG推送任务失败：%+v", err)
	}
	if err := s.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "补建资料上架状态失败：%+v", err)
	}
	if err := s.recoverProfilePublishOperationStates(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复资料上架状态失败：%+v", err)
	}
}

// recoverConfirmedTelegramJobs repairs the local terminal projection after all
// Telegram delivery phases were already confirmed. It never sends media again.
func (s *sSysPublish) recoverConfirmedTelegramJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.*").
		Where("j.status", telegramJobStatusSending).
		WhereIn("j.send_phase", []string{telegramSendPhaseVerifyConfirmed, telegramSendPhaseCompletedNoVerify}).
		WhereLTE("j.updated_at", gtime.Now().Add(-telegramConfirmedJobRecoverAfter)).
		Where(telegramActiveChannelConditionForAlias("j")).
		OrderAsc("j.updated_at").OrderAsc("j.id").Limit(limit).Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取已确认未收口TG任务失败")
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if completeErr := s.completeTelegramJobAndWakeChannel(ctx, job); completeErr != nil {
			g.Log().Warningf(ctx, "收口已确认TG任务失败 jobId:%d phase:%s err:%+v", job.Id, job.SendPhase, completeErr)
		}
	}
	return nil
}

// recoverStalePreAttemptTelegramJobs handles the only sending states that are
// safe to retry without reconciliation: cleanup started/completed, but no
// Telegram delivery Attempt was ever created. No external send can have
// happened through the unified delivery path in this state.
func (s *sSysPublish) recoverStalePreAttemptTelegramJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.*").
		Where("j.status", telegramJobStatusSending).
		WhereIn("j.send_phase", telegramPreAttemptPhases()).
		WhereLTE("j.updated_at", now.Add(-telegramPreAttemptRecoverAfter)).
		Where(telegramActiveChannelConditionForAlias("j")).
		Where("NOT EXISTS (SELECT 1 FROM " + publishTgAttemptTable + " attempt WHERE attempt.job_id=j.id)").
		OrderAsc("j.updated_at").OrderAsc("j.id").Limit(limit).Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取发送Attempt创建前中断的TG任务失败")
	}
	for _, job := range jobs {
		jobCtx := telegramPublishJobTraceContext(ctx, job)
		step := beginTelegramPublishStage(jobCtx, "recovery.pre_attempt", g.Map{"reason": "stale_cleanup_without_attempt", "staleAfterMs": telegramPreAttemptRecoverAfter.Milliseconds()})
		transitioned, transitionErr := s.transitionTelegramJob(ctx, telegramJobTransition{
			JobId: job.Id, From: []string{telegramJobStatusSending}, To: telegramJobStatusRetryWait,
			SendPhases: telegramPreAttemptPhases(), UpdatedBefore: now.Add(-telegramPreAttemptRecoverAfter),
			ErrorMessage: "TG任务在发送前中断，已安全重新投递",
			Additional: g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"next_retry_at":       now,
				"last_dispatch_error": "TG任务在发送前中断，已安全重新投递",
			},
		})
		step.End(transitionErr, g.Map{"transitioned": transitioned})
		if transitionErr != nil {
			g.Log().Warningf(ctx, "恢复发送前中断TG任务失败 jobId:%d err:%+v", job.Id, transitionErr)
			continue
		}
		if !transitioned {
			continue
		}
		job.Status = telegramJobStatusRetryWait
		s.appendTelegramJobLog(ctx, job, "recovery", "queued", "发送Attempt尚未创建，已安全重新投递")
		if enqueueErr := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); enqueueErr != nil {
			g.Log().Warningf(ctx, "重新投递发送前中断TG任务失败 jobId:%d err:%+v", job.Id, enqueueErr)
		}
	}
	return nil
}

func telegramPreAttemptPhases() []string {
	return []string{"", telegramSendPhaseCleanupProcessing, telegramSendPhaseCleanupConfirmed}
}

func (s *sSysPublish) recoverFailedJobsMissingVerification(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.*", "(SELECT COUNT(1) FROM "+publishMediaTable+" media WHERE media.profile_id=j.profile_id AND media.purpose='verify' AND media.deleted_at IS NULL) AS verify_media_count").
		Where("j.status", telegramJobStatusFailed).
		Where("j.send_phase", telegramSendPhaseDisplayConfirmed).
		Where(telegramActiveChannelConditionForAlias("j")).
		Where("EXISTS (SELECT 1 FROM " + publishTgMessageTable + " display WHERE display.job_id=j.id AND display.purpose='display' AND display.status='sent' AND display.deleted_at IS NULL)").
		Where("NOT EXISTS (SELECT 1 FROM " + publishTgMessageTable + " message WHERE message.job_id=j.id AND message.purpose='verify' AND message.status='sent' AND message.deleted_at IS NULL)").
		OrderAsc("j.updated_at").OrderAsc("j.id").Limit(limit).Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取缺少验证视频的终态TG任务失败")
	}
	now := gtime.Now()
	for _, job := range jobs {
		allowed, allowedErr := s.canSendTelegramJob(ctx, job)
		if allowedErr != nil {
			g.Log().Warningf(ctx, "校验缺失验证视频TG任务失败 jobId:%d err:%+v", job.Id, allowedErr)
			continue
		}
		if !allowed {
			if supersedeErr := s.supersedeTelegramJobAndCompleteOperation(ctx, job); supersedeErr != nil {
				g.Log().Warningf(ctx, "废弃不可发布的验证视频恢复任务失败 jobId:%d err:%+v", job.Id, supersedeErr)
			}
			continue
		}
		recoveryMessage := telegramFailedDisplayRecoveryMessage(job.VerifyMediaCount)
		result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			Where("status", telegramJobStatusFailed).
			Where("send_phase", telegramSendPhaseDisplayConfirmed).
			Data(g.Map{
				"status":              telegramJobStatusRetryWait,
				"dispatch_status":     tgDispatchStatusIdle,
				"next_retry_at":       now,
				"error_message":       "",
				"last_dispatch_error": recoveryMessage,
				"updated_at":          now,
			}).Update()
		if updateErr != nil {
			return gerror.Wrapf(updateErr, "恢复缺少验证视频的终态TG任务失败 jobId:%d", job.Id)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		job.Status = telegramJobStatusRetryWait
		job.DispatchStatus = tgDispatchStatusIdle
		s.appendTelegramJobLog(ctx, job, "verify_recovery", "queued", recoveryMessage)
		if enqueueErr := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); enqueueErr != nil {
			g.Log().Warningf(ctx, "重新投递缺失验证视频TG任务失败 jobId:%d err:%+v", job.Id, enqueueErr)
		}
	}
	return nil
}

func telegramFailedDisplayRecoveryMessage(verifyMediaCount int) string {
	if verifyMediaCount > 0 {
		return "展示已确认，验证视频已重新进入频道顺序队列"
	}
	return "展示已确认且无验证媒体，已重新进入完成队列"
}

func (s *sSysPublish) recoverExpiredTelegramOutboxDispatches(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	type expiredDispatch struct {
		OutboxId     int64 `json:"outboxId"`
		JobId        int64 `json:"jobId"`
		AttemptCount int   `json:"attemptCount"`
	}
	var rows []expiredDispatch
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishTgOutboxTable+" o", "o.job_id=j.id").
		Fields("o.id AS outbox_id,o.job_id,o.attempt_count").
		Where("o.event_type", telegramOutboxEventPublish).
		Where("o.status", telegramOutboxProcessing).
		WhereLTE("o.lease_expires_at", now).
		WhereIn("j.status", []string{telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusConfirming}).
		Where("j.dispatch_status", tgDispatchStatusQueued).
		Where(telegramActiveChannelConditionForAlias("j")).
		OrderAsc("o.lease_expires_at").OrderAsc("j.id").Limit(limit).Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取Outbox入队确认中断的TG任务失败")
	}
	for _, row := range rows {
		recovered := false
		transactionErr := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			outboxResult, updateErr := tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).
				Where("id", row.OutboxId).
				Where("status", telegramOutboxProcessing).
				WhereLTE("lease_expires_at", now).
				Data(g.Map{
					"status": telegramOutboxPending, "lease_owner": "", "lease_expires_at": nil,
					"available_at": now, "last_error": "Outbox入队确认超时，已自动恢复", "updated_at": now,
				}).Update()
			if updateErr != nil {
				return updateErr
			}
			outboxAffected, _ := outboxResult.RowsAffected()
			if outboxAffected == 0 {
				return nil
			}
			jobResult, updateErr := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("id", row.JobId).
				Where("dispatch_status", tgDispatchStatusQueued).
				Data(g.Map{
					"dispatch_status": tgDispatchStatusIdle,
					"next_retry_at":   now, "last_dispatch_error": "Outbox入队确认超时，已自动恢复",
					"updated_at": now,
				}).Update()
			if updateErr != nil {
				return updateErr
			}
			jobAffected, _ := jobResult.RowsAffected()
			if jobAffected == 0 {
				return gerror.Newf("TG任务已离开待入队状态 jobId:%d", row.JobId)
			}
			recovered = true
			return nil
		})
		if transactionErr != nil {
			g.Log().Warningf(ctx, "原子恢复Outbox入队确认中断任务失败 jobId:%d outboxId:%d err:%+v", row.JobId, row.OutboxId, transactionErr)
			continue
		}
		if !recovered {
			continue
		}
		if dispatchErr := s.dispatchTelegramJobOutbox(ctx, telegramJobOutboxRecord{
			Id: row.OutboxId, JobId: row.JobId, AttemptCount: row.AttemptCount, AvailableAt: now,
		}); dispatchErr != nil {
			g.Log().Warningf(ctx, "重新调度Outbox入队确认中断任务失败 jobId:%d outboxId:%d err:%+v", row.JobId, row.OutboxId, dispatchErr)
		}
	}
	return nil
}

func (s *sSysPublish) recoverStaleTelegramMediaFallbackJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("status", telegramJobStatusSending).
		WhereLike("last_dispatch_error", telegramMediaFallbackMarker+"%").
		WhereNotNull("next_retry_at").
		WhereLTE("next_retry_at", gtime.Now()).
		OrderAsc("next_retry_at").OrderAsc("id").Limit(limit).Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取超时协议号媒体降级任务失败")
	}
	for _, job := range jobs {
		purpose := telegramMediaFallbackPurpose(job.LastDispatchError)
		if purpose != "display" && purpose != "verify" {
			continue
		}
		channel, channelErr := s.messagePushChannelFromJob(ctx, job)
		if channelErr != nil {
			g.Log().Warningf(ctx, "恢复协议号媒体降级任务缺少可用账号 jobId:%d purpose:%s err:%+v", job.Id, purpose, channelErr)
			if deferErr := s.deferTelegramMediaFallbackRecovery(ctx, job); deferErr != nil {
				g.Log().Warningf(ctx, "延后协议号媒体降级恢复失败 jobId:%d purpose:%s err:%+v", job.Id, purpose, deferErr)
			}
			continue
		}
		if channel == nil || channel.TgAccountId <= 0 {
			cause := gerror.Wrap(errTelegramMediaFallbackUnavailable, "频道未绑定可用协议号")
			if failErr := s.failTelegramMediaFallbackJob(ctx, job, cause); failErr != nil {
				g.Log().Warningf(ctx, "终止不可恢复的协议号媒体降级任务失败 jobId:%d purpose:%s err:%+v", job.Id, purpose, failErr)
			}
			continue
		}
		if submitErr := s.submitTelegramMediaFallback(ctx, job, channel.TgAccountId, purpose); submitErr != nil {
			g.Log().Warningf(ctx, "重新提交协议号媒体降级任务失败 jobId:%d purpose:%s err:%+v", job.Id, purpose, submitErr)
			if deferErr := s.deferTelegramMediaFallbackRecovery(ctx, job); deferErr != nil {
				g.Log().Warningf(ctx, "延后协议号媒体降级恢复失败 jobId:%d purpose:%s err:%+v", job.Id, purpose, deferErr)
			}
			continue
		}
		s.appendTelegramJobLog(ctx, job, "account_fallback", "requeued", "协议号媒体降级任务超时，已幂等重新提交")
	}
	return nil
}

func telegramMediaFallbackRecoveryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := telegramMediaFallbackRecoverAfter
	for i := 0; i < attempt && delay < telegramMediaFallbackMaxBackoff; i++ {
		delay *= 2
	}
	if delay > telegramMediaFallbackMaxBackoff {
		return telegramMediaFallbackMaxBackoff
	}
	return delay
}

func (s *sSysPublish) deferTelegramMediaFallbackRecovery(ctx context.Context, job telegramJobRecord) error {
	attempt := job.ReconcileCount + 1
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		Where("status", telegramJobStatusSending).
		WhereLike("last_dispatch_error", telegramMediaFallbackMarker+"%").
		Data(g.Map{
			"reconcile_count": attempt,
			"next_retry_at":   gtime.Now().Add(telegramMediaFallbackRecoveryDelay(attempt)),
			"updated_at":      gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新协议号媒体降级恢复时间失败")
	}
	return nil
}

func telegramMediaFallbackJob(reason string) bool {
	return strings.HasPrefix(strings.TrimSpace(reason), telegramMediaFallbackMarker)
}

func (s *sSysPublish) recoverStaleSendingTelegramAttempts(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var attempts []telegramDeliveryAttempt
	err := g.DB().Model(publishTgAttemptTable+" attempt").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" job", "job.id=attempt.job_id").
		Fields("attempt.*").
		Where("attempt.status", telegramAttemptStatusSending).
		Where("NOT EXISTS (SELECT 1 FROM "+publishTgAttemptMessageTable+" response WHERE response.attempt_id=attempt.id AND response.source='response')").
		Where("attempt.updated_at <= ?", gtime.Now().Add(-telegramSendingAttemptRecoverAfter)).
		WhereIn("job.status", telegramAttemptRecoverableJobStatuses()).
		OrderAsc("attempt.updated_at").OrderAsc("attempt.id").Limit(limit).Scan(&attempts)
	if err != nil {
		return gerror.Wrap(err, "读取中断的TG发送Attempt失败")
	}
	for _, attempt := range attempts {
		deadline := gtime.Now()
		updated := false
		updateErr := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			var currentAttempt telegramDeliveryAttempt
			if err := tx.Model(publishTgAttemptTable).Where("id", attempt.Id).LockUpdate().Scan(&currentAttempt); err != nil {
				return err
			}
			if currentAttempt.Id <= 0 || currentAttempt.Status != telegramAttemptStatusSending {
				return nil
			}
			var job telegramJobRecord
			if err := tx.Model(publishTgJobTable).Where("id", currentAttempt.JobId).LockUpdate().Scan(&job); err != nil {
				return err
			}
			if job.Id <= 0 || !telegramAttemptRecoverableJobStatus(job.Status) {
				return nil
			}
			if _, err := tx.Model(publishTgAttemptTable).Where("id", currentAttempt.Id).
				Where("status", telegramAttemptStatusSending).
				Data(g.Map{
					"status": telegramAttemptStatusWaiting, "webhook_deadline": deadline,
					"error_message": "发送进程中断，等待Webhook或协议号对账", "updated_at": gtime.Now(),
				}).Update(); err != nil {
				return err
			}
			if job.Status != telegramJobStatusConfirming {
				transitioned, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
					JobId: currentAttempt.JobId, From: []string{job.Status}, To: telegramJobStatusConfirming,
					ErrorMessage: "发送进程中断，等待Telegram结果对账",
					Additional:   g.Map{"dispatch_status": tgDispatchStatusProcessing, "next_retry_at": deadline},
				})
				if err != nil {
					return err
				}
				if !transitioned {
					return gerror.New("TG任务状态已变化，取消Attempt恢复")
				}
			}
			updated = true
			return nil
		})
		if updateErr != nil {
			g.Log().Warningf(ctx, "标记中断TG Attempt待对账失败 attemptId:%d err:%+v", attempt.Id, updateErr)
			continue
		}
		if !updated {
			continue
		}
		attempt.Status = telegramAttemptStatusWaiting
		s.projectTelegramDeliveryAttempt(ctx, attempt, "发送进程中断，等待Webhook或协议号对账")
		if _, updateErr = s.enqueueTelegramAttemptTimeout(ctx, attempt.Id, 0); updateErr != nil {
			g.Log().Warningf(ctx, "投递中断TG Attempt对账失败 attemptId:%d err:%+v", attempt.Id, updateErr)
		}
	}
	return nil
}

func telegramAttemptRecoverableJobStatuses() []string {
	return []string{
		telegramJobStatusPending,
		telegramJobStatusSending,
		telegramJobStatusRetryWait,
		telegramJobStatusConfirming,
	}
}

func telegramAttemptRecoverableJobStatus(status string) bool {
	for _, candidate := range telegramAttemptRecoverableJobStatuses() {
		if status == candidate {
			return true
		}
	}
	return false
}

func (s *sSysPublish) requeueExpiredTelegramAttempts(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Fields("id").Where("status", telegramAttemptStatusWaiting).
		WhereLTE("webhook_deadline", gtime.Now().Add(-5*time.Second)).
		OrderAsc("webhook_deadline").OrderAsc("id").Limit(limit).Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取过期TG发送Attempt失败")
	}
	queued := 0
	for _, row := range rows {
		attemptId := row.Id
		enqueued, enqueueErr := s.enqueueTelegramAttemptTimeout(ctx, attemptId, 0)
		if enqueueErr != nil {
			err = enqueueErr
			g.Log().Warningf(ctx, "重新投递过期TG发送Attempt失败 attemptId:%d err:%+v", attemptId, err)
			continue
		}
		if enqueued {
			queued++
		}
	}
	if queued > 0 {
		g.Log().Infof(ctx, "已重新投递过期TG发送Attempt：%d条", queued)
	}
	return nil
}

func (s *sSysPublish) recoverTerminalTelegramAttempts(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishTgAttemptTable+" attempt").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" job", "job.id=attempt.job_id").
		Fields("attempt.id").
		WhereIn("attempt.status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		WhereIn("job.status", []string{"sent", "failed", "superseded"}).
		OrderAsc("attempt.id").Limit(limit).Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取终态TG任务遗留Attempt失败")
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Id)
	}
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		Data(g.Map{"status": telegramAttemptStatusSuperseded, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "收敛终态TG任务遗留Attempt失败")
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		g.Log().Infof(ctx, "已收敛终态TG任务遗留Attempt：%d条", affected)
	}
	return nil
}

func (s *sSysPublish) recoverStaleTelegramDispatchJobs(ctx context.Context, limit int) error {
	startedAt := time.Now()
	scanned := 0
	var observeErr error
	defer func() { observeRecoveryRun(ctx, "stale_dispatch", startedAt, scanned, observeErr) }()
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	deadline := telegramRecoveryTimeText(now.Add(-telegramDispatchJobRecoverAfter))
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Where(telegramActiveChannelCondition()).
		Where(messagePushPlanRecoveryConditionSQL(publishTgJobTable), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
		WhereLTE("dispatched_at", deadline).
		OrderAsc("dispatched_at").OrderAsc("id").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		observeErr = err
		return gerror.Wrap(err, "读取调度中断TG推送任务失败")
	}
	scanned = len(jobs)
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
			Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"next_retry_at":       gtime.Now(),
				"last_dispatch_error": "TG任务调度超时，已自动恢复入队",
				"updated_at":          gtime.Now(),
			}).Update()
		if updateErr != nil {
			g.Log().Warningf(ctx, "重置调度中断TG任务失败 job:%d err:%+v", job.Id, updateErr)
			continue
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		// The stale Asynq task can still hold its uniqueness key after the task
		// itself disappeared. A recovery enqueue must create a fresh task; the
		// database transition in the worker remains the delivery idempotency gate.
		if enqueueErr := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); enqueueErr != nil {
			g.Log().Warningf(ctx, "重新入队调度中断TG任务失败 job:%d err:%+v", job.Id, enqueueErr)
		}
	}
	return nil
}

func (s *sSysPublish) recoverPendingIdleTelegramJobs(ctx context.Context, limit int) error {
	startedAt := time.Now()
	scanned := 0
	var observeErr error
	defer func() { observeRecoveryRun(ctx, "pending_idle", startedAt, scanned, observeErr) }()
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	nowText := telegramRecoveryTimeText(now)
	deadline := telegramRecoveryTimeText(now.Add(-telegramPendingJobRecoverAfter))
	candidateLimit := limit * 10
	if candidateLimit < 100 {
		candidateLimit = 100
	}
	if candidateLimit > 2000 {
		candidateLimit = 2000
	}
	var candidates []telegramJobRecord
	var err error
	if strings.EqualFold(strings.TrimSpace(g.DB().GetConfig().Type), "pgsql") {
		err = g.DB().GetScan(ctx, &candidates, telegramPendingRecoveryPostgresSQL(),
			telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter)), nowText, deadline, candidateLimit)
	} else {
		err = g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
			WhereIn("j.status", []string{"pending", "failed_retry", "unknown"}).
			Where(telegramWebhookDetachedJobSQL("j.")).
			Where("(j.dispatch_status = ? OR j.dispatch_status = '')", tgDispatchStatusIdle).
			Where(telegramPendingRecoveryChannelHeadSQL("j", "earlier")).
			Where(telegramActiveChannelConditionForAlias("j")).
			Where(messagePushPlanRecoveryConditionSQL("j"), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
			Where("(j.next_retry_at IS NULL OR j.next_retry_at <= ?)", nowText).
			WhereLTE("j.updated_at", deadline).
			OrderAsc("j.created_at").OrderAsc("j.id").Limit(candidateLimit).Scan(&candidates)
	}
	if err != nil {
		observeErr = err
		return gerror.Wrap(err, "读取待入队TG推送任务失败")
	}
	jobs := fairPendingTelegramJobs(candidates, limit)
	scanned = len(candidates)
	if len(jobs) > 0 {
		g.Log().Infof(ctx, "恢复待入队TG推送任务：%d条", len(jobs))
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if err = s.rearmTelegramJobOutbox(ctx, job.Id, 0); err != nil {
			g.Log().Warningf(ctx, "重新激活待入队TG任务Outbox失败 job:%d err:%+v", job.Id, err)
			continue
		}
		if err = s.dispatchTelegramJobOutboxByJob(ctx, job.Id); err != nil {
			g.Log().Warningf(ctx, "恢复待入队TG推送任务失败 job:%d err:%+v", job.Id, err)
		}
	}
	return nil
}

func telegramPendingRecoveryPostgresSQL() string {
	channelKey := "CASE WHEN j.channel_id > 0 THEN j.channel_id::text ELSE 'chat:' || REPLACE(COALESCE(j.target_chat_id,''), '-100', '') END"
	return `WITH channel_heads AS (
		SELECT DISTINCT ON (j.tenant_id, ` + channelKey + `)
			j.id,j.task_id,j.operation_no,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.bot_id,
			j.push_mode,j.status,j.target_chat_id,j.collect_event_id,j.collect_source_id,j.collect_source_chat_id,
			j.collect_source_message_id,j.retry_count,j.priority,j.asynq_task_id,j.queue_name,j.dispatch_status,
			j.dispatched_at,j.dispatch_count,j.last_dispatch_error,j.next_retry_at,j.send_phase,j.reconcile_count,
			j.error_message,j.sent_at,j.cycle_enabled,j.cycle_days,j.cycle_publish_time,j.next_cycle_at,j.created_at,j.updated_at
		FROM ` + publishTgJobTable + ` j
		WHERE ` + telegramChannelBlockingJobSQL("j") + `
		ORDER BY j.tenant_id, ` + channelKey + `, j.created_at, j.id
	)
	SELECT h.* FROM channel_heads h
	WHERE h.status IN ('pending','failed_retry','unknown')
		AND COALESCE(h.dispatch_status,'') IN ('','idle')
		AND ` + telegramActiveChannelConditionForAlias("h") + `
		AND ` + messagePushPlanRecoveryConditionSQL("h") + `
		AND (h.next_retry_at IS NULL OR h.next_retry_at<=?)
		AND h.updated_at<=?
	ORDER BY h.created_at,h.id
	LIMIT ?`
}

// telegramPendingRecoveryChannelHeadSQL keeps heavily backlogged channels from
// consuming the global candidate limit with jobs that cannot run yet.
func telegramPendingRecoveryChannelHeadSQL(jobAlias, earlierAlias string) string {
	return `NOT EXISTS (
		SELECT 1 FROM ` + publishTgJobTable + ` ` + earlierAlias + `
		WHERE ` + earlierAlias + `.tenant_id=` + jobAlias + `.tenant_id
			AND (
				(` + jobAlias + `.channel_id>0 AND ` + earlierAlias + `.channel_id=` + jobAlias + `.channel_id) OR
				(` + jobAlias + `.channel_id<=0 AND ` + earlierAlias + `.channel_id<=0 AND
					REPLACE(COALESCE(` + earlierAlias + `.target_chat_id,''),'-100','')=
					REPLACE(COALESCE(` + jobAlias + `.target_chat_id,''),'-100',''))
			)
			AND ` + telegramChannelBlockingJobSQL(earlierAlias) + `
			AND (` + earlierAlias + `.created_at<` + jobAlias + `.created_at OR
				(` + earlierAlias + `.created_at=` + jobAlias + `.created_at AND ` + earlierAlias + `.id<` + jobAlias + `.id))
	)`
}

func fairPendingTelegramJobs(candidates []telegramJobRecord, limit int) []telegramJobRecord {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	jobs := make([]telegramJobRecord, 0, min(limit, len(candidates)))
	seen := make(map[string]struct{}, len(candidates))
	for _, job := range candidates {
		key := fmt.Sprintf("%d:%d:%s", job.TenantId, job.ChannelId, normalizeTelegramChannelChatID(job.TargetChatId))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
		}
	}
	return jobs
}

func telegramRecoveryTimeText(value *gtime.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("Y-m-d H:i:s")
}

func (s *sSysPublish) telegramJobChannelIsActive(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.ChannelId <= 0 || job.TenantId <= 0 {
		return false, nil
	}
	if isMessagePushOperationNo(job.OperationNo) {
		if job.AccountId <= 0 || job.TargetChatId == "" {
			return false, nil
		}
		count, err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Where("id", job.ChannelId).
			Where("tenant_id", job.TenantId).
			Where("tg_account_id", job.AccountId).
			Where("REPLACE(channel_id, '-100', '') = REPLACE(?, '-100', '')", normalizeTelegramChannelChatID(job.TargetChatId)).
			Count()
		if err != nil {
			return false, gerror.Wrap(err, "检查快速推送目标频道失败")
		}
		return count > 0, nil
	}
	count, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", job.ChannelId).
		Where("tenant_id", job.TenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查TG任务目标频道失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) recoverStaleTelegramSendingJobs(ctx context.Context, limit int) error {
	startedAt := time.Now()
	scanned := 0
	var observeErr error
	defer func() { observeRecoveryRun(ctx, "stale_sending", startedAt, scanned, observeErr) }()
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	deadline := telegramRecoveryTimeText(now.Add(-telegramSendingJobRecoverAfter))
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("status", "sending").
		Where(telegramActiveChannelCondition()).
		Where(messagePushPlanRecoveryConditionSQL(publishTgJobTable), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		observeErr = err
		return gerror.Wrap(err, "读取卡住的TG推送任务失败")
	}
	scanned = len(jobs)
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if err = s.requeueStaleTelegramSendingJob(ctx, job); err != nil {
			g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败 job:%d err:%+v", job.Id, err)
		}
	}
	return nil
}

func (s *sSysPublish) supersedeExpiredMessagePushPlanJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := telegramRecoveryTimeText(gtime.Now().Add(-messagePushPlanJobExpireAfter))
	var rows []struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id").
		Where("operation_no LIKE ?", "message_push_plan:%").
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
		WhereLT("created_at", deadline).
		OrderAsc("id").
		Limit(limit).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取过期历史消息计划任务失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	affected, err := s.transitionTelegramJobs(ctx, ids, telegramJobTransition{
		From: []string{
			telegramJobStatusPending, telegramJobStatusSending,
			telegramJobStatusRetryWait, telegramJobStatusConfirming,
		},
		To:           telegramJobStatusSuperseded,
		ErrorMessage: expiredMessagePushPlanJobMessage,
		Additional:   g.Map{"last_dispatch_error": expiredMessagePushPlanJobMessage},
	})
	if err != nil {
		return gerror.Wrap(err, "终止过期历史消息计划任务失败")
	}
	if affected > 0 {
		g.Log().Infof(ctx, "已终止过期历史消息计划任务：%d条", affected)
	}
	return nil
}

func messagePushPlanRecoveryConditionSQL(jobTable string) string {
	return "NOT (" + jobTable + ".operation_no LIKE 'message_push_plan:%' AND " + jobTable + ".created_at < ?)"
}

func telegramActiveChannelCondition() string {
	return telegramActiveChannelConditionForAlias(publishTgJobTable)
}

func telegramActiveChannelConditionForAlias(jobTable string) string {
	publishChannelCondition := "EXISTS (SELECT 1 FROM " + publishChannelTable + " c INNER JOIN " + publishTgAccountTable + " a ON a.id=c.tg_account_id AND a.tenant_id=c.tenant_id AND a.status='authorized' AND NULLIF(BTRIM(a.session_key), '') IS NOT NULL AND a.deleted_at IS NULL WHERE c.id=" + jobTable + ".channel_id AND c.tenant_id=" + jobTable + ".tenant_id AND c.status=1 AND c.deleted_at IS NULL)"
	messagePushChannelCondition := "((" + jobTable + ".operation_no LIKE 'message_push:%' OR " + jobTable + ".operation_no LIKE 'message_push_plan:%') AND EXISTS (SELECT 1 FROM " + publishTgChannelTable + " tc INNER JOIN " + publishTgAccountTable + " a ON a.id=tc.tg_account_id AND a.tenant_id=tc.tenant_id AND a.status='authorized' AND NULLIF(BTRIM(a.session_key), '') IS NOT NULL AND a.deleted_at IS NULL WHERE tc.id=" + jobTable + ".channel_id AND tc.tenant_id=" + jobTable + ".tenant_id AND REPLACE(tc.channel_id, '-100', '') = REPLACE(" + jobTable + ".target_chat_id, '-100', '')))"
	// An expired protocol account must not be continuously re-enqueued. Keep
	// its jobs durable and eligible for a later login-triggered rescan, while
	// excluding them from the active recovery/dispatch loops in the meantime.
	return "(" + publishChannelCondition + " OR " + messagePushChannelCondition + ")"
}

func (s *sSysPublish) requeueStaleTelegramSendingJob(ctx context.Context, job telegramJobRecord) error {
	now := gtime.Now()
	status := "failed_retry"
	message := "TG推送任务长时间处于发送中，已自动重新投递"
	var latestAttempt telegramDeliveryAttempt
	if err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", job.Id).OrderDesc("id").Limit(1).Scan(&latestAttempt); err != nil {
		return gerror.Wrap(err, "读取最新TG发送Attempt失败")
	}
	attemptCount, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", job.Id).Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG发送Attempt失败")
	}
	if attemptCount == 0 {
		message = "TG推送在创建发送Attempt前中断，已安全重新投递"
	}
	reconcileCount := job.ReconcileCount
	if telegramAttemptHasDefinitivePermanentFailure(latestAttempt) {
		decision := telegramJobFailureNextState(gerror.New(latestAttempt.ErrorMessage), job.RetryCount)
		status = decision.Status
		message = decision.Message
	} else if job.SendPhase == telegramSendPhaseDisplaySending || job.SendPhase == telegramSendPhaseVerifySending {
		status = telegramJobStatusConfirming
		message = "服务中断时TG发送结果不确定，已进入频道消息对账"
		reconcileCount = 0
	}
	retryDelay := time.Duration(0)
	if status == telegramJobStatusConfirming {
		retryDelay = telegramUnknownReconcileDelay
	}
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId:         job.Id,
		From:          []string{telegramJobStatusSending},
		To:            status,
		RetryDelay:    retryDelay,
		ErrorMessage:  message,
		UpdatedBefore: now.Add(-telegramSendingJobRecoverAfter),
		Additional:    g.Map{"reconcile_count": reconcileCount},
	})
	if err != nil {
		return gerror.Wrap(err, "重置卡住的TG推送任务失败")
	}
	if !transitioned {
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "publish", status, message)
	projectedStatus := sysin.PublishTaskStatusPending
	if status == telegramJobStatusFailed {
		projectedStatus = sysin.PublishTaskStatusFailed
	}
	if err = s.updateProfilePublishOperationState(ctx, job, projectedStatus); err != nil {
		return err
	}
	if status == telegramJobStatusFailed {
		return nil
	}
	return s.enqueueTelegramJob(ctx, job.Id, 0)
}

func telegramAttemptHasDefinitivePermanentFailure(attempt telegramDeliveryAttempt) bool {
	if attempt.Id <= 0 || attempt.Status != telegramAttemptStatusRetryWait ||
		attempt.ResponseReceived != 0 || strings.TrimSpace(attempt.ErrorMessage) == "" {
		return false
	}
	return telegramJobFailureNextState(gerror.New(attempt.ErrorMessage), 0).Status == telegramJobStatusFailed
}
