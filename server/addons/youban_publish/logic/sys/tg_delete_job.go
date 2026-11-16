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
	started := time.Now()
	payload, err := hydrateTelegramCleanupPayload(ctx, jobId, payload)
	if err != nil {
		return err
	}
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	base := g.Map{
		"log_type":           "telegram_cleanup",
		"event":              "started",
		"jobId":              job.Id,
		"profileId":          job.ProfileId,
		"tenantId":           job.TenantId,
		"accountId":          job.AccountId,
		"channelId":          job.ChannelId,
		"operationNo":        job.OperationNo,
		"cleanupOperationNo": strings.TrimSpace(payload.CleanupOperationNo),
		"targetChatId":       job.TargetChatId,
	}
	g.Log().Info(ctx, "TG资料清理链路", base)
	defer func() {
		result := g.Map{
			"log_type":           "telegram_cleanup",
			"event":              "finished",
			"jobId":              job.Id,
			"profileId":          job.ProfileId,
			"tenantId":           job.TenantId,
			"accountId":          job.AccountId,
			"channelId":          job.ChannelId,
			"operationNo":        job.OperationNo,
			"cleanupOperationNo": strings.TrimSpace(payload.CleanupOperationNo),
			"durationMs":         time.Since(started).Milliseconds(),
			"returnedError":      resultErr != nil,
		}
		if resultErr != nil {
			result["error"] = resultErr.Error()
			g.Log().Warning(ctx, "TG资料清理链路", result)
		} else {
			g.Log().Info(ctx, "TG资料清理链路", result)
		}
	}()
	logStage := func(stage string, stageStarted time.Time, stageErr error) {
		data := g.Map{
			"log_type":           "telegram_cleanup_stage",
			"event":              "finished",
			"stage":              stage,
			"jobId":              job.Id,
			"profileId":          job.ProfileId,
			"channelId":          job.ChannelId,
			"operationNo":        job.OperationNo,
			"cleanupOperationNo": strings.TrimSpace(payload.CleanupOperationNo),
			"durationMs":         time.Since(stageStarted).Milliseconds(),
			"returnedError":      stageErr != nil,
		}
		if stageErr != nil {
			data["error"] = stageErr.Error()
			g.Log().Warning(ctx, "TG资料清理阶段", data)
		} else {
			g.Log().Info(ctx, "TG资料清理阶段", data)
		}
	}
	cleanup := func(cleanupCtx context.Context) error {
		return s.withTelegramCleanupJobLease(cleanupCtx, job.Id, func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			stageStarted := time.Now()
			err := s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料清理")
			logStage("delete_messages", stageStarted, err)
			if err != nil {
				return err
			}
			stageStarted = time.Now()
			err = s.notifyBotProfileUnpublishCompleted(ctx, job, payload)
			logStage("notify_unpublish_completed", stageStarted, err)
			if err != nil {
				g.Log().Warningf(ctx, "发送Bot资料下架完成通知失败 profileId:%d jobId:%d err:%+v", job.ProfileId, job.Id, err)
				return gerror.Wrap(err, "发送Bot资料下架完成通知失败")
			}
			stageStarted = time.Now()
			err = s.finishTelegramCleanupProfile(ctx, job, payload)
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
			return err
		})
	}
	priority := telegramCleanupExecutionPriority(ctx, job)
	if strings.TrimSpace(payload.CleanupOperationNo) != "" {
		priority = channelPriorityManualDelete
	}
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		return s.withTelegramCleanupExecution(ctx, job, cleanup, priority)
	}
	lease, ok, err := tryTelegramCleanupChannelSlot(ctx, job.TargetChatId, priority)
	if err != nil {
		return err
	}
	if !ok {
		g.Log().Warning(ctx, "TG资料清理等待频道并发槽位", g.Map{
			"log_type": "telegram_cleanup_stage", "event": "requeue",
			"stage": "channel_cleanup_slot", "jobId": job.Id, "profileId": job.ProfileId,
			"channelId": job.ChannelId, "targetChatId": job.TargetChatId,
			"priority": priority, "slotLimit": telegramCleanupSlotLimit(ctx, priority),
		})
		return s.requeueTelegramCleanupJob(ctx, jobId, time.Second, payload)
	}
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

func (s *sSysPublish) finishTelegramCleanupProfile(ctx context.Context, job telegramJobRecord, payload tgQueuePayload) error {
	operationNo := strings.TrimSpace(payload.CleanupOperationNo)
	if operationNo == "" {
		// 没有当前 unpublishing 状态时，重复清理任务已经是幂等完成；
		// 不使用历史 job.OperationNo，也不让遗留任务无限重试。
		return nil
	}
	return s.finishProfilesUnpublishing(ctx, []int64{job.ProfileId}, job.TenantId, operationNo)
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
