package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	hglock "hotgo/internal/library/hgrds/lock"
	"hotgo/utility/ownedlease"
)

const telegramCleanupPerChannelConcurrency = 4

func (s *sSysPublish) CleanupTelegramJobMessages(ctx context.Context, jobId int64) error {
	return s.cleanupTelegramJobMessages(ctx, jobId, tgQueuePayload{})
}

func (s *sSysPublish) cleanupTelegramJobMessages(ctx context.Context, jobId int64, payload tgQueuePayload) (resultErr error) {
	stageStarted := time.Now()
	ctx, payload, err := hydrateTelegramCleanupPayloadContext(ctx, jobId, payload)
	ctx, observation := beginPublishCleanupObservation(ctx, jobId, payload)
	observation.started = stageStarted
	defer func() { observation.finish(resultErr) }()
	observation.stage("hydrate_payload", stageStarted, err)
	if err != nil {
		return err
	}
	stageStarted = time.Now()
	job, err := s.telegramJobById(ctx, jobId)
	observation.stage("load_job", stageStarted, err)
	if err != nil {
		return err
	}
	observation.job = job
	observation.logStarted()
	logStage := observation.stage
	cleanup := func(cleanupCtx context.Context) error {
		leaseStarted := time.Now()
		err := s.withTelegramCleanupJobLease(cleanupCtx, job.Id, func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			stageStarted := time.Now()
			manualDown := isManualTelegramCleanupUnpublish(payload)
			preserve := false
			if manualDown {
				var policyErr error
				preserve, policyErr = s.channelPreservesHistoryMessages(ctx, job.TenantId, job.ChannelId)
				if policyErr != nil {
					return policyErr
				}
			}
			var err error
			if preserve {
				s.appendTelegramJobLog(ctx, job, "down", "skipped", "资料下架，执行时频道配置缺失或保留旧消息，跳过TG删除")
			} else {
				err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料清理")
			}
			logStage("delete_messages", stageStarted, err)
			if err != nil {
				return err
			}
			stageStarted = time.Now()
			err = settleTelegramCleanupProgress(ctx, s, job, payload)
			logStage("settle_profile_cleanup_progress", stageStarted, err)
			if err != nil {
				return err
			}
			stageStarted = time.Now()
			if !strings.HasPrefix(payload.CleanupOperationNo, "cancel:") && payload.ProgressOperationNo == "" {
				noticePayload := payload
				if manualDown && len(payload.CleanupJobIds) > 0 {
					// The current intent is still processing. Preserve the decision
					// already executed even if channel policy changes before notification.
					noticeIds := make([]int64, 0, len(payload.CleanupJobIds))
					for _, id := range payload.CleanupJobIds {
						if preserve && id == job.Id {
							continue
						}
						noticeIds = append(noticeIds, id)
					}
					noticePayload.CleanupJobIds, err = s.telegramCleanupDeletableJobIds(ctx, job.TenantId, noticeIds, payload.CleanupOperationNo)
					if err != nil {
						return err
					}
					noticePayload.ForceCompletion = len(noticePayload.CleanupJobIds) == 0
				}
				err = s.notifyBotProfileUnpublishCompleted(ctx, job, noticePayload)
				logStage("notify_unpublish_completed", stageStarted, err)
			} else {
				observation.recordStage("notify_unpublish_completed", "skipped", stageStarted, nil)
			}
			if err != nil {
				g.Log().Warningf(ctx, "发送Bot资料下架完成通知失败 profileId:%d jobId:%d err:%s", job.ProfileId, job.Id, publishCleanupSafeText(err.Error()))
				return gerror.Wrap(err, "发送Bot资料下架完成通知失败")
			}
			stageStarted = time.Now()
			var skippedJobIds []int64
			if manualDown && preserve {
				skippedJobIds = []int64{job.Id}
			}
			err = s.finishTelegramCleanupProfile(ctx, job, payload, skippedJobIds...)
			logStage("finish_profile_state", stageStarted, err)
			if err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			stageStarted = time.Now()
			err = s.completeTelegramCleanupIntent(ctx, job.Id, payload)
			logStage("complete_cleanup_intent", stageStarted, err)
			if err == nil {
				observation.outcome = "settled_attempt"
			}
			return err
		})
		logStage("job_cleanup_lease", leaseStarted, err)
		return err
	}
	priority := telegramCleanupExecutionPriority(ctx, job)
	if strings.TrimSpace(payload.CleanupOperationNo) != "" {
		priority = channelPriorityManualDelete
	}
	stageStarted = time.Now()
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		entered := false
		err = s.withTelegramCleanupExecution(ctx, job, func(cleanupCtx context.Context) error {
			entered = true
			logStage("channel_cleanup_slot", stageStarted, nil)
			return cleanup(cleanupCtx)
		}, priority)
		if !entered {
			if publishCleanupErrorType(err) == "retry_after" {
				observation.outcome = "deferred"
				observation.recordStage("channel_cleanup_slot", "deferred", stageStarted, err)
			} else {
				logStage("channel_cleanup_slot", stageStarted, err)
			}
		}
		return err
	}
	lease, ok, err := tryTelegramCleanupChannelSlot(ctx, job.TargetChatId, priority)
	if err != nil {
		logStage("channel_cleanup_slot", stageStarted, err)
		return err
	}
	if !ok {
		data := observation.fields("channel_cleanup_slot", "deferred", stageStarted, nil)
		data["event"], data["priority"] = "requeue", priority
		data["slotLimit"] = telegramCleanupSlotLimit(ctx, priority)
		g.Log().Warning(ctx, "TG资料清理等待频道并发槽位", data)
		observation.recordStage("channel_cleanup_slot", "deferred", stageStarted, nil)
		stageStarted = time.Now()
		err = s.requeueTelegramCleanupJob(ctx, jobId, time.Second, payload)
		logStage("requeue", stageStarted, err)
		if err == nil {
			observation.outcome = "deferred"
		}
		return err
	}
	logStage("channel_cleanup_slot", stageStarted, nil)
	defer s.releaseTelegramChannelLease(ctx, lease)
	return cleanup(ctx)
}

// finishTelegramCleanupProfile only accepts the operation captured when the
// current unpublish/delete request created the cleanup payload. A historical
// publish job operation is never a valid fallback.
func (s *sSysPublish) withTelegramCleanupJobLease(ctx context.Context, jobId int64, fn func(context.Context) error) error {
	lease := ownedlease.New(fmt.Sprintf("telegram:cleanup:job:%d", jobId), "telegram_cleanup_job", telegramStreamExecutionTTL)
	acquired, err := lease.TryAcquire(ctx)
	if err != nil {
		return gerror.Wrap(err, "获取清理任务执行租约失败")
	}
	if !acquired {
		return &tgRetryAfterError{after: telegramStreamExecutionRenewInterval, err: gerror.New("该清理任务正在执行")}
	}
	bounded, stop := telegramStreamExecutionContext(ctx, lease)
	defer func() {
		stop()
		_ = lease.Unlock(context.Background())
	}()
	return fn(bounded)
}

func (s *sSysPublish) finishTelegramCleanupProfile(ctx context.Context, job telegramJobRecord, payload tgQueuePayload, skippedJobIds ...int64) error {
	operationNo := strings.TrimSpace(payload.CleanupOperationNo)
	if strings.HasPrefix(operationNo, "cancel:") || payload.ProgressOperationNo != "" {
		return nil
	} // Cancellation/progress receipt recovery owns its terminal state.
	if operationNo == "" {
		// 没有当前 unpublishing 状态时，重复清理任务已经是幂等完成；
		// 不使用历史 job.OperationNo，也不让遗留任务无限重试。
		return nil
	}
	return s.finishProfilesUnpublishing(ctx, []int64{job.ProfileId}, job.TenantId, operationNo, payload.CleanupJobIds, skippedJobIds)
}

// Only the current cleanup request identifies a manual unpublish. Historical
// job operations must not make delete, cancellation or progress cleanup skip TG.
func isManualTelegramCleanupUnpublish(payload tgQueuePayload) bool {
	if strings.TrimSpace(payload.ProgressOperationNo) != "" {
		return false
	}
	operationNo := strings.ToLower(strings.TrimSpace(payload.CleanupOperationNo))
	return strings.HasPrefix(operationNo, "unpublish:") || isDownTelegramOperationNo(operationNo)
}

// Keep preserved messages sent so a later explicit delete can still find them.
// Only unpublish completion and its notice exclude these jobs.
func (s *sSysPublish) telegramCleanupDeletableJobIds(ctx context.Context, tenantId int64, ids []int64, operationNo ...string) ([]int64, error) {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	var jobs []telegramJobRecord
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Master().
		Fields("j.id,j.channel_id").Where("j.tenant_id", tenantId).WhereIn("j.id", ids)
	if len(operationNo) > 0 && strings.TrimSpace(operationNo[0]) != "" {
		mod = mod.Where("NOT EXISTS (SELECT 1 FROM "+publishTgOutboxTable+" o WHERE o.job_id=j.id AND o.operation_no=? AND o.event_type IN (?,?,?) AND o.status=?)",
			strings.TrimSpace(operationNo[0]), telegramOutboxCleanupPrefix+"manual", telegramOutboxCleanupPrefix+"keyword", telegramOutboxCleanupPrefix+"cycle", telegramOutboxDelivered)
	}
	if err := mod.Scan(&jobs); err != nil {
		return nil, gerror.Wrap(err, "读取下架清理频道失败")
	}
	channelIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		channelIds = append(channelIds, job.ChannelId)
	}
	policies, err := s.channelHistoryMessagePolicies(ctx, tenantId, channelIds)
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		if preserve, configured := policies[job.ChannelId]; configured && !preserve {
			result = append(result, job.Id)
		}
	}
	return result, nil
}

// Cleanup keeps the existing four Redis keys; automatic jobs leave the highest
// slots available for manual requests. No new lock namespace or extra capacity.
func tryTelegramCleanupChannelSlot(ctx context.Context, chatId string, priority channelDispatchPriority) (*hglock.Lock, bool, error) {
	base := telegramChannelLockKey(chatId) + ":cleanup"
	limit := telegramCleanupSlotLimit(ctx, priority)
	for slot := 0; slot < limit; slot++ {
		lock := hglock.NewConfig(telegramChannelLeaseTTL(ctx), time.Second).WithoutWatchDog().Mutex(fmt.Sprintf("%s:%d", base, slot))
		if err := lock.TryLock(ctx); err != nil {
			if gerror.Is(err, hglock.ErrLockFailed) {
				continue
			}
			return nil, false, gerror.Wrap(err, "获取TG频道清理并发槽位失败")
		}
		return lock, true, nil
	}
	return nil, false, nil
}

func (s *sSysPublish) telegramJobById(ctx context.Context, jobId int64) (telegramJobRecord, error) {
	var job telegramJobRecord
	if jobId <= 0 {
		return job, gerror.New("TG任务ID不能为空")
	}
	// 发送任务必须从主库读取：Outbox/队列可能已由主库提交，读副本延迟会把
	// 一个真实存在的任务误判为 sql.ErrNoRows，进而反复重试并阻塞频道队首。
	// 读取 Job 时一次关联已落库的采集事件，避免每个埋点阶段重复查询。
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Master().
		LeftJoin(publishCollectEventTable+" e", "e.id=j.collect_event_id AND e.tenant_id=j.tenant_id").
		Fields("j.*,e.received_at AS original_telegram_at,e.source_type AS collect_source_type").Where("j.id", jobId).Scan(&job); err != nil {
		return job, gerror.Wrap(err, "读取TG任务失败")
	}
	if job.Id <= 0 {
		return job, gerror.New("TG任务不存在")
	}
	return job, nil
}

func (s *sSysPublish) telegramJobActiveMessages(ctx context.Context, job telegramJobRecord, purpose ...string) ([]telegramDeleteMessage, error) {
	var rows []telegramDeleteMessage
	mod := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id,tg_message_id AS message_id").
		Where("job_id", job.Id).
		Where("status", "sent")
	if len(purpose) > 0 && strings.TrimSpace(purpose[0]) != "" {
		mod = mod.Where("purpose", strings.TrimSpace(purpose[0]))
	}
	err := mod.
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG消息记录失败")
	}
	return rows, nil
}

func (s *sSysPublish) telegramJobsWithUndeletableMessages(ctx context.Context, jobIds []int64) (map[int64]bool, error) {
	jobIds = uniqueIds(jobIds)
	result := make(map[int64]bool, len(jobIds))
	if len(jobIds) == 0 {
		return result, nil
	}
	var rows []struct {
		JobId int64 `orm:"job_id"`
	}
	err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("job_id").
		WhereIn("job_id", jobIds).
		Where("status", "undeletable").
		Group("job_id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "批量检查TG历史消息删除状态失败")
	}
	for _, row := range rows {
		result[row.JobId] = true
	}
	return result, nil
}

func (s *sSysPublish) telegramJobsWithActiveMessages(ctx context.Context, tenantId int64, profileIds []int64, cutoffAt string) ([]telegramResubmitJob, error) {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return []telegramResubmitJob{}, nil
	}
	var jobs []telegramResubmitJob
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.*").
		Where("j.tenant_id", tenantId).
		WhereIn("j.profile_id", profileIds).
		Where("EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=j.id AND m.status='sent')")
	if strings.TrimSpace(cutoffAt) != "" {
		mod = mod.WhereLTE("j.created_at", cutoffAt)
	}
	if err := mod.OrderAsc("j.id").Scan(&jobs); err != nil {
		return nil, gerror.Wrap(err, "读取待清理TG历史消息失败")
	}
	return jobs, nil
}

type telegramDeleteMessage struct {
	Id           int64  `json:"id"`
	TargetChatId string `json:"targetChatId"`
	MessageId    int64  `json:"messageId"`
}
