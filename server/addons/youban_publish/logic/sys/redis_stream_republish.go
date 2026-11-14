package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
)

const telegramOutboxCleanupPrefix = "telegram_job.cleanup."

// runTelegramCleanupIntents 独立推进删除通知，Redis或删除投递异常不暂停发送Outbox的轮次。
func (s *sSysPublish) runTelegramCleanupIntents(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := s.dispatchTelegramCleanupIntents(bounded, 100); err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "投递频道删除意图失败：%+v", err)
			}
			cancel()
		}
	}
}

// prepareStreamProfileChannelPublish 仅处理仍有消息或尚未终态的旧任务，事务内保存删除意图，发送路径不调用TG删除。
func (s *sSysPublish) prepareStreamProfileChannelPublish(ctx context.Context, current telegramJobRecord) (bool, error) {
	preserve, err := s.channelPreservesHistoryMessages(ctx, current.TenantId, current.ChannelId)
	if err != nil {
		return false, err
	}
	var previous []telegramJobRecord
	err = g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Fields("j.*").
		Where("j.tenant_id", current.TenantId).Where("j.profile_id", current.ProfileId).
		WhereIn("j.target_chat_id", telegramChannelChatIDVariants(current.TargetChatId)).WhereLT("j.id", current.Id).
		Where("(j.status IN ('pending','sending','failed_retry','unknown') OR EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=j.id AND m.status='sent' AND m.deleted_at IS NULL))").
		OrderAsc("j.id").Scan(&previous)
	if err != nil {
		return false, gerror.Wrap(err, "读取频道待替换消息失败")
	}
	if len(previous) == 0 {
		return true, nil
	}
	event := telegramOutboxCleanupPrefix + "keyword"
	if isInteractiveProfilePublishOperation(strings.ToLower(current.OperationNo)) {
		event = telegramOutboxCleanupPrefix + "manual"
	} else if isCycleBatchOperation(current.OperationNo) {
		event = telegramOutboxCleanupPrefix + "cycle"
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, job := range previous {
			if !preserve {
				if _, err := tx.Model(publishTgOutboxTable).Ctx(ctx).InsertIgnore(g.Map{
					"job_id": job.Id, "event_type": event, "dedupe_key": fmt.Sprintf("telegram-cleanup:%d", job.Id),
					"status": telegramOutboxPending, "available_at": gtime.Now(),
				}); err != nil {
					return gerror.Wrap(err, "保存频道历史消息删除意图失败")
				}
			}
			if job.Status != telegramJobStatusSuperseded {
				if _, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{JobId: job.Id, From: []string{job.Status}, To: telegramJobStatusSuperseded}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// dispatchTelegramCleanupIntents 复用现有Outbox和删除队列；投递后保持租约，直到实际删除完成才确认，Redis丢失可重投。
func (s *sSysPublish) dispatchTelegramCleanupIntents(ctx context.Context, limit int) error {
	var rows []struct {
		Id           int64
		JobId        int64
		EventType    string
		AttemptCount int
		TenantId     int64
		ChannelId    int64
	}
	now := gtime.Now()
	if err := g.DB().Model(publishTgOutboxTable+" o").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=o.job_id").
		Fields("o.id,o.job_id,o.event_type,o.attempt_count,j.tenant_id,j.channel_id").
		WhereIn("o.event_type", []string{telegramOutboxCleanupPrefix + "manual", telegramOutboxCleanupPrefix + "keyword", telegramOutboxCleanupPrefix + "cycle"}).
		WhereLTE("o.available_at", now).Where("o.status=? OR (o.status=? AND o.lease_expires_at<=?)", telegramOutboxPending, telegramOutboxProcessing, now).
		OrderAsc("o.available_at").OrderAsc("o.id").Limit(limit).Scan(&rows); err != nil {
		return err
	}
	for _, row := range rows {
		owner := telegramOutboxOwner()
		result, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).Where("id", row.Id).
			Where("status=? OR (status=? AND lease_expires_at<=?)", telegramOutboxPending, telegramOutboxProcessing, now).
			Data(g.Map{"status": telegramOutboxProcessing, "lease_owner": owner, "lease_expires_at": now.Add(telegramOutboxLeaseTTL)}).Update()
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 0 {
			continue
		}
		priority := channelPriorityKeywordDelete
		if row.EventType == telegramOutboxCleanupPrefix+"manual" {
			priority = channelPriorityManualDelete
		} else if row.EventType == telegramOutboxCleanupPrefix+"cycle" {
			priority = channelPriorityCycleDelete
		}
		if indexErr := (tgChannelDeleteIndex{}).addJob(ctx, row.TenantId, row.ChannelId, row.JobId, priority); indexErr != nil {
			g.Log().Warningf(ctx, "写入频道删除队列索引失败 tenantId:%d channelId:%d jobId:%d err:%+v", row.TenantId, row.ChannelId, row.JobId, indexErr)
		}
		client, err := s.telegramQueueClient(ctx)
		if err == nil {
			// 固定task ID在任务执行或重试期间去重，不覆盖发送任务的asynq_task_id/queue_name。
			_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCleanup, []byte(fmt.Sprintf(`{"jobId":%d}`, row.JobId))),
				asynq.Queue(channelDispatchDeleteQueueName(priority)), asynq.TaskID(fmt.Sprintf("tg-cleanup-job-%d", row.JobId)), asynq.MaxRetry(20), asynq.Timeout(2*time.Minute))
		}
		if err != nil && !gerror.Is(err, asynq.ErrTaskIDConflict) {
			if releaseErr := s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, err.Error()); releaseErr != nil {
				return releaseErr
			}
		}
	}
	return nil
}

// completeTelegramCleanupIntent 只在实际删除成功后确认持久化意图，不修改发送任务执行元数据。
func (s *sSysPublish) completeTelegramCleanupIntent(ctx context.Context, jobId int64) error {
	_, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).Where("job_id", jobId).
		WhereIn("event_type", []string{telegramOutboxCleanupPrefix + "manual", telegramOutboxCleanupPrefix + "keyword", telegramOutboxCleanupPrefix + "cycle"}).
		WhereNot("status", telegramOutboxDelivered).Data(g.Map{"status": telegramOutboxDelivered, "delivered_at": gtime.Now(), "lease_owner": "", "lease_expires_at": nil, "last_error": ""}).Update()
	if err != nil {
		return err
	}
	var job struct {
		TenantId  int64 `orm:"tenant_id"`
		ChannelId int64 `orm:"channel_id"`
	}
	if queryErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("tenant_id,channel_id").Where("id", jobId).Scan(&job); queryErr != nil {
		return gerror.Wrap(queryErr, "读取频道删除队列索引任务失败")
	}
	if indexErr := (tgChannelDeleteIndex{}).removeJob(ctx, job.TenantId, job.ChannelId, jobId); indexErr != nil {
		g.Log().Warningf(ctx, "移除频道删除队列索引失败 tenantId:%d channelId:%d jobId:%d err:%+v", job.TenantId, job.ChannelId, jobId, indexErr)
	}
	return err
}
