package sys

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	"hotgo/utility/ownedlease"
)

// telegramStreamChannelBlockingJobSQL 将未知发送保留为本频道屏障，其他频道继续执行，防止迟到发送打乱组内顺序。
func telegramStreamChannelBlockingJobSQL(alias string) string {
	prefix := strings.TrimSpace(alias)
	if prefix != "" {
		prefix += "."
	}
	return "(" + telegramChannelBlockingJobSQL(alias) + " OR " + prefix + "status='unknown')"
}

const (
	telegramStreamExecutionTTL              = 30 * time.Second
	telegramStreamExecutionRenewInterval    = 10 * time.Second
	telegramStreamExecutionRecoveryInterval = 10 * time.Second
)

// telegramPublishScheduleTime 对齐Asynq的秒级延迟分数，禁止提前出队引发同一任务反复检查和再次入队。
func telegramPublishScheduleTime(now time.Time, delay time.Duration) time.Time {
	deadline := now.Add(delay)
	if deadline.Nanosecond() != 0 {
		return deadline.Truncate(time.Second).Add(time.Second)
	}
	return deadline
}

// telegramPublishQueueCheckIntervals 仅为新引擎缩短已有Asynq检查周期，旧引擎保留中间件默认行为。
func telegramPublishQueueCheckIntervals(ctx context.Context) (time.Duration, time.Duration) {
	if globalStreamPushEngineEnabled() || g.Cfg().MustGet(ctx, "youbanPublish.redisStream.grayEnabled", false).Bool() {
		return 200 * time.Millisecond, 100 * time.Millisecond
	}
	return 5 * time.Second, time.Second
}

// newTelegramStreamExecutionLease 复用频道互斥键和有归属的租约，进程退出后最多占用一个短租期。
func newTelegramStreamExecutionLease(job telegramJobRecord) *ownedlease.Lease {
	owner, _ := json.Marshal(telegramPublishLeaseOwner{Kind: "telegram_publish_stream_v2", JobId: job.Id, TenantId: job.TenantId})
	return ownedlease.New(telegramChannelLockKey(job.TargetChatId), string(owner), telegramStreamExecutionTTL)
}

// telegramStreamExecutionContext 在发送预算内续租；续租失败立即取消发送，停止后等待续租协程退出再释放锁。
func telegramStreamExecutionContext(ctx context.Context, lease telegramChannelLease) (context.Context, func()) {
	renewable, ok := lease.(*ownedlease.Lease)
	if !ok {
		return ctx, func() {}
	}
	bounded, stopBudget := context.WithTimeout(ctx, telegramSendBudget)
	execution, cancel := context.WithCancelCause(bounded)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(telegramStreamExecutionRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-execution.Done():
				return
			case <-ticker.C:
				renewCtx, stop := context.WithTimeout(execution, 3*time.Second)
				owned, err := renewable.Renew(renewCtx)
				stop()
				if err != nil || !owned {
					cancel(gerror.Newf("频道执行租约续期失败，停止本次发送：%v", err))
					return
				}
			}
		}
	}()
	return execution, func() { cancel(context.Canceled); stopBudget(); <-done }
}

// runTelegramStreamExecutionRecovery 定期补偿新引擎已失去执行者的确认阶段，单轮有界且按频道独立抢占。
func (s *sSysPublish) runTelegramStreamExecutionRecovery(ctx context.Context) {
	ticker := time.NewTicker(telegramStreamExecutionRecoveryInterval)
	defer ticker.Stop()
	cursor := int64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
			if err := s.recoverTelegramStreamExecutions(bounded, 200); err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "恢复Stream频道执行进度失败：%+v", err)
			}
			for bounded.Err() == nil {
				var err error
				cursor, err = s.recoverMissingTelegramStreamTasks(bounded, cursor, 200)
				if err != nil {
					if bounded.Err() == nil {
						g.Log().Warningf(ctx, "恢复Stream执行队列缺失任务失败：%+v", err)
					}
					break
				}
				if cursor == 0 {
					break
				}
			}
			cancel()
		}
	}
}

// recoverMissingTelegramStreamTasks 用Asynq自身检查能力确认队列任务缺失，再恢复PG通知意图；游标避免正常旧任务挡住后续恢复。
func (s *sSysPublish) recoverMissingTelegramStreamTasks(ctx context.Context, afterID int64, limit int) (int64, error) {
	var jobs []telegramJobRecord
	cutoff := gtime.Now().Add(-telegramStreamExecutionTTL)
	query := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Fields("j.*").
		WhereGT("j.id", afterID).WhereIn("j.status", []string{telegramJobStatusPending, telegramJobStatusRetryWait}).
		Where("(j.dispatch_status IN (?,?) OR (j.dispatch_status=? AND EXISTS (SELECT 1 FROM "+publishTgOutboxTable+" ready WHERE ready.job_id=j.id AND ready.event_type=? AND ready.status=?)))", tgDispatchStatusQueued, tgDispatchStatusProcessing, tgDispatchStatusIdle, telegramOutboxEventPublish, telegramOutboxDelivered).
		WhereLTE("j.updated_at", cutoff).Where("j.next_retry_at IS NULL OR j.next_retry_at<=?", gtime.Now()).
		OrderAsc("j.id").Limit(limit)
	if !globalStreamPushEngineEnabled() {
		query = query.Where("EXISTS (SELECT 1 FROM "+publishAccountSettingTable+" settings WHERE settings.tenant_id=j.tenant_id AND settings.account_id=j.account_id AND settings.push_engine=? AND settings.deleted_at IS NULL)", pushEngineUnifiedStream)
	}
	if err := query.Scan(&jobs); err != nil {
		return afterID, err
	}
	options := telegramQueueRedisOpt(ctx)
	options.DialTimeout = time.Second
	options.ReadTimeout = 2 * time.Second
	options.WriteTimeout = 2 * time.Second
	inspector := asynq.NewInspector(options)
	defer inspector.Close()
	cursor := afterID
	for _, job := range jobs {
		if ctx.Err() != nil {
			return cursor, ctx.Err()
		}
		// Redis丢失就绪事件时，任务仍可能停在idle；只恢复该频道真正队首，避免批量重复通知。
		if job.DispatchStatus == tgDispatchStatusIdle {
			waiting, err := s.telegramChannelHasEarlierActiveJob(ctx, job)
			if err != nil {
				return cursor, err
			}
			if waiting {
				cursor = job.Id
				continue
			}
		}
		missing := job.AsynqTaskId == "" || job.QueueName == ""
		if !missing {
			info, err := inspector.GetTaskInfo(job.QueueName, job.AsynqTaskId)
			if err == nil {
				missing = info.State == asynq.TaskStateArchived || info.State == asynq.TaskStateCompleted
			} else if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
				missing = true
			} else {
				return cursor, err
			}
		}
		if missing {
			result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).
				Where("asynq_task_id", job.AsynqTaskId).WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusRetryWait}).
				WhereIn("dispatch_status", []string{tgDispatchStatusIdle, tgDispatchStatusQueued, tgDispatchStatusProcessing}).WhereLTE("updated_at", cutoff).
				Data(g.Map{"dispatch_status": tgDispatchStatusIdle, "asynq_task_id": "", "updated_at": gtime.Now(), "last_dispatch_error": "就绪通知或执行任务缺失，已按持久化意图恢复"}).Update()
			if err != nil {
				return cursor, err
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return cursor, err
			}
			if changed > 0 {
				if err := s.rearmTelegramJobOutbox(ctx, job.Id, 0); err != nil {
					return cursor, err
				}
				s.appendTelegramJobLog(ctx, job, "execution_recovery", "ready", "Asynq任务已缺失，恢复持久化就绪通知")
			}
		}
		cursor = job.Id
	}
	if len(jobs) < limit {
		return 0, nil
	}
	return cursor, nil
}

// recoverTelegramStreamExecutions 仅扫描安全检查点；未确认的外部发送仍由原Attempt对账处理，禁止盲目重发。
func (s *sSysPublish) recoverTelegramStreamExecutions(ctx context.Context, limit int) error {
	var jobs []telegramJobRecord
	query := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.*").WhereIn("j.status", []string{telegramJobStatusSending, telegramJobStatusConfirming}).
		WhereIn("j.send_phase", []string{telegramSendPhaseDisplayConfirmed, telegramSendPhaseVerifyConfirmed, telegramSendPhaseCompletedNoVerify}).
		WhereLTE("j.updated_at", gtime.Now().Add(-telegramStreamExecutionTTL)).
		Where(telegramActiveChannelConditionForAlias("j")).
		OrderAsc("j.updated_at").OrderAsc("j.id").Limit(limit)
	if !globalStreamPushEngineEnabled() {
		query = query.Where("EXISTS (SELECT 1 FROM "+publishAccountSettingTable+" settings WHERE settings.tenant_id=j.tenant_id AND settings.account_id=j.account_id AND settings.push_engine=? AND settings.deleted_at IS NULL)", pushEngineUnifiedStream)
	}
	if err := query.Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取Stream待续跑检查点失败")
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.recoverTelegramStreamExecution(ctx, job); err != nil {
			g.Log().Warningf(ctx, "恢复Stream发送检查点失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, err)
		}
	}
	return nil
}

// recoverTelegramStreamExecution 先取得频道执行权，再以条件迁移交给已有对账/续跑能力，不创建替代任务。
func (s *sSysPublish) recoverTelegramStreamExecution(ctx context.Context, job telegramJobRecord) error {
	lease := newTelegramStreamExecutionLease(job)
	acquired, err := lease.TryAcquire(ctx)
	if err != nil || !acquired {
		return err
	}
	// 已有确认检查点的unknown只补本地提交，仍复用原对账CAS；未确认发送阶段不进入本扫描。
	if job.Status == telegramJobStatusConfirming {
		s.releaseTelegramChannelLease(ctx, lease)
		if err := s.reconcileUnknownTelegramJob(ctx, job); err != nil {
			return err
		}
		return s.wakeNextTelegramChannelJob(ctx, job)
	}
	// 迁移只对无执行者且检查点未变化的任务生效；提交后先释放执行权，再唤醒续跑。
	changed, transitionErr := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []string{telegramJobStatusSending}, To: telegramJobStatusConfirming,
		SendPhases:    []string{job.SendPhase},
		UpdatedBefore: gtime.Now().Add(-telegramStreamExecutionTTL),
		Additional:    g.Map{"error_message": "执行进程已退出，按已确认检查点恢复", "dispatch_status": tgDispatchStatusProcessing, "next_retry_at": nil},
	})
	s.releaseTelegramChannelLease(ctx, lease)
	if transitionErr != nil || !changed {
		return transitionErr
	}
	s.appendTelegramJobLog(ctx, job, "execution_recovery", "checkpoint", "频道执行租约已释放，复用已确认发送证据继续任务")
	job.Status = telegramJobStatusConfirming
	if err := s.reconcileUnknownTelegramJob(ctx, job); err != nil {
		return err
	}
	return s.wakeNextTelegramChannelJob(ctx, job)
}
