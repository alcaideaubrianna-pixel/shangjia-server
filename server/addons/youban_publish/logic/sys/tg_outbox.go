package sys

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	telegramOutboxEventPublish = "telegram_job.publish"
	telegramOutboxPending      = "pending"
	telegramOutboxProcessing   = "processing"
	telegramOutboxDelivered    = "delivered"
	// 查询允许最多运行 30 秒，但只作用于当前账户分片；超时后该分片退避，
	// 不得占用其他账户的调度资源。租约必须长于查询预算，避免查询尚未结束时租约提前过期。
	telegramOutboxLeaseTTL     = 90 * time.Second
	telegramOutboxQueryTimeout = 30 * time.Second
	telegramOutboxWorkers      = 32
	// 大批量会让频道前序查询退化为 O(N²)，积压时可能触发查询超时。
	// 小批次配合账户分片和失败退避，确保单账户慢查询不会阻塞其他账户。
	telegramOutboxBatchLimitDefault       = 500
	telegramOutboxMaxBatchLimitDefault    = 10000
	telegramStreamOutboxBatchLimitDefault = 500

	telegramOutboxDecisionWait     = "wait"
	telegramOutboxDecisionDispatch = "dispatch"
	telegramOutboxDecisionComplete = "complete"
)

func telegramOutboxBatchLimit(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.queue.outboxBatchLimit", telegramOutboxBatchLimitDefault).Int()
	if value < 100 {
		return 100
	}
	if value > telegramOutboxMaxBatchLimit(ctx) {
		return telegramOutboxMaxBatchLimit(ctx)
	}
	return value
}

func telegramOutboxMaxBatchLimit(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.queue.outboxMaxBatchLimit", telegramOutboxMaxBatchLimitDefault).Int()
	if value < 100 {
		return 100
	}
	if value > 20000 {
		return 20000
	}
	return value
}

// telegramStreamOutboxBatchLimit 控制 Stream 就绪通知单轮批量，便于按
// worker 实例规格逐步放量；上限仍受全局 Outbox 批量保护。
func telegramStreamOutboxBatchLimit(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.queue.streamOutboxBatchLimit", telegramStreamOutboxBatchLimitDefault).Int()
	if value < 100 {
		return 100
	}
	if value > telegramOutboxMaxBatchLimit(ctx) {
		return telegramOutboxMaxBatchLimit(ctx)
	}
	return value
}

type telegramJobOutboxRecord struct {
	Id           int64       `json:"id"`
	JobId        int64       `json:"jobId"`
	AttemptCount int         `json:"attemptCount"`
	AvailableAt  *gtime.Time `json:"availableAt"`
	AccountId    int64       `json:"accountId"`
	TenantId     int64       `json:"tenantId"`
	ChannelId    int64       `json:"channelId"`
	Priority     int         `json:"priority"`
	TargetChatId string      `json:"targetChatId"`
}

// ensureTelegramJobOutboxTx persists the initial queue intent in the same
// transaction as the job. Redis is transport; this row is the durable intent.
func (s *sSysPublish) ensureTelegramJobOutboxTx(ctx context.Context, tx gdb.TX, jobId int64, delay time.Duration) error {
	if jobId <= 0 {
		return nil
	}
	now := gtime.Now()
	availableAt := now.Add(delay)
	_, err := tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).InsertIgnore(g.Map{
		"job_id":       jobId,
		"event_type":   telegramOutboxEventPublish,
		"dedupe_key":   fmt.Sprintf("telegram-job:%d:initial", jobId),
		"status":       telegramOutboxPending,
		"available_at": availableAt,
		"created_at":   now,
		"updated_at":   now,
	})
	if err != nil {
		return gerror.Wrap(err, "保存TG任务Outbox失败")
	}
	// Outbox 是可靠事实记录，频道 ZSET 只是快速队首索引。写入两者后，
	// 即使 Redis 短暂不可用，后续 Outbox 扫描也会重新回填索引。
	var job telegramJobRecord
	if err = tx.Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,channel_id,priority").Where("id", jobId).Scan(&job); err == nil && job.Id > 0 {
		if indexErr := (tgChannelPendingIndex{}).add(ctx, job.TenantId, job.ChannelId, job.Id, job.Priority); indexErr != nil {
			g.Log().Warningf(ctx, "写入频道待投递索引失败，等待Outbox回填 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, indexErr)
		}
	}
	// 事务提交后 Dispatcher 会立即被唤醒；若事务尚未提交，下一轮兜底
	// 扫描仍会再次领取，事件信号本身不承担可靠性。
	s.wakeTelegramOutbox()
	return nil
}

// rearmTelegramJobOutbox restores the durable queue intent after a worker
// returns a previously delivered job to idle. A delivered Outbox row only
// proves transport handoff; it is not terminal while the Job still needs work.
func (s *sSysPublish) rearmTelegramJobOutbox(ctx context.Context, jobId int64, delay time.Duration) error {
	if jobId <= 0 {
		return nil
	}
	clearTelegramReadyStreamDedupe(ctx, jobId)
	now := gtime.Now()
	result, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("event_type", telegramOutboxEventPublish).
		WhereIn("status", []string{telegramOutboxDelivered, telegramOutboxPending}).
		Data(g.Map{
			"status": telegramOutboxPending, "available_at": now.Add(delay),
			"lease_owner": "", "lease_expires_at": nil, "delivered_at": nil,
			"last_error": "", "updated_at": now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重新激活TG任务Outbox失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	// 并发 rearm 可能已经有 processing 行；不能再插入第二条 Outbox。
	exists, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("event_type", telegramOutboxEventPublish).Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG任务Outbox重入状态失败")
	}
	if exists > 0 {
		return nil
	}
	_, err = g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).InsertIgnore(g.Map{
		"job_id": jobId, "event_type": telegramOutboxEventPublish,
		"dedupe_key": fmt.Sprintf("telegram-job:%d:initial", jobId),
		"status":     telegramOutboxPending, "available_at": now.Add(delay),
		"created_at": now, "updated_at": now,
	})
	if err != nil {
		return gerror.Wrap(err, "补建TG任务Outbox失败")
	}
	return nil
}

func (s *sSysPublish) dispatchTelegramJobOutboxByJob(ctx context.Context, jobId int64) error {
	if jobId <= 0 {
		return nil
	}
	record, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("event_type", telegramOutboxEventPublish).
		WhereNot("status", telegramOutboxDelivered).
		Where("available_at <= ?", gtime.Now()).
		OrderAsc("id").Limit(1).One()
	if err != nil {
		return gerror.Wrap(err, "读取TG任务Outbox失败")
	}
	row := telegramJobOutboxRecordFromRecord(record)
	if row.Id <= 0 {
		// A future outbox row is dispatched by the background dispatcher when it
		// becomes due. Only jobs created before the outbox migration may bypass it.
		count, countErr := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
			Where("job_id", jobId).Where("event_type", telegramOutboxEventPublish).
			WhereNot("status", telegramOutboxDelivered).Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "检查TG任务Outbox失败")
		}
		if count > 0 {
			return nil
		}
		return s.enqueueTelegramJobDirect(ctx, jobId, 0)
	}
	return s.dispatchTelegramJobOutbox(ctx, row)
}

func telegramJobOutboxRecordFromRecord(record gdb.Record) telegramJobOutboxRecord {
	if record.IsEmpty() {
		return telegramJobOutboxRecord{}
	}
	return telegramJobOutboxRecord{
		Id:           record["id"].Int64(),
		JobId:        record["job_id"].Int64(),
		AttemptCount: record["attempt_count"].Int(),
		AvailableAt:  record["available_at"].GTime(),
		AccountId:    record["account_id"].Int64(),
		TenantId:     record["tenant_id"].Int64(),
		ChannelId:    record["channel_id"].Int64(),
		Priority:     record["priority"].Int(),
	}
}

func (s *sSysPublish) dispatchTelegramJobOutbox(ctx context.Context, row telegramJobOutboxRecord) (resultErr error) {
	ctx = telegramPublishTraceContext(ctx, row.JobId, g.Map{"outboxId": row.Id, "outboxAttemptCount": row.AttemptCount})
	stage := beginTelegramPublishStage(ctx, "outbox.dispatch")
	defer stage.Finish(&resultErr)
	// 逐条 Outbox 领取是高频热路径，只保留 Debug，避免批量推送时同步格式化大量日志。
	g.Log().Debug(ctx, "TG Outbox 选中任务", g.Map{"event": "outbox_selected", "outboxId": row.Id, "jobId": row.JobId, "outboxAttemptCount": row.AttemptCount, "availableAt": row.AvailableAt})
	// 全量 Stream 模式下，频道队首判断由 Stream 消费后的发送逻辑完成。
	// Outbox 这里只做可靠事件投递，避免每轮对大表执行相关子查询。
	if !globalStreamPushEngineEnabled() {
		step := beginTelegramPublishStage(ctx, "outbox.decision")
		decision, err := s.telegramOutboxJobDispatchDecision(ctx, row.JobId, gtime.Now())
		step.End(err, g.Map{"decision": decision})
		if err != nil {
			return err
		}
		switch decision {
		case telegramOutboxDecisionComplete:
			return s.completeTelegramOutbox(ctx, row.Id)
		case telegramOutboxDecisionWait:
			return nil
		}
	}
	owner := telegramOutboxOwner()
	now := gtime.Now()
	step := beginTelegramPublishStage(ctx, "outbox.lease.acquire")
	result, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", row.Id).
		Where("status = ? OR (status = ? AND lease_expires_at <= ?)", telegramOutboxPending, telegramOutboxProcessing, now).
		Data(g.Map{
			"status": telegramOutboxProcessing, "lease_owner": owner,
			"lease_expires_at": now.Add(telegramOutboxLeaseTTL), "updated_at": now,
		}).Update()
	step.End(err)
	if err != nil {
		return gerror.Wrap(err, "领取TG任务Outbox失败")
	}
	affected, _ := result.RowsAffected()
	beginTelegramPublishStage(ctx, "outbox.lease.result").End(nil, g.Map{"affectedRows": affected, "acquired": affected > 0})
	if affected == 0 {
		g.Log().Info(ctx, "TG Outbox 租约未获取", g.Map{"event": "outbox_lease_missed", "outboxId": row.Id, "jobId": row.JobId})
		return nil
	}
	job, err := s.telegramJobById(ctx, row.JobId)
	if err != nil {
		return s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, err.Error())
	}
	// 所有账户统一以 Asynq 作为发送主路径，不能再由账户灰度字段决定是否
	// 进入可靠队列；Stream 只作为旁路能力，避免 legacy 账户被旧路径卡死。
	if err = s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); err == nil {
		g.Log().Debug(ctx, "TG任务统一进入Asynq发送主路径", g.Map{"event": "asynq_primary_enqueue", "jobId": job.Id, "outboxId": row.Id})
		return s.confirmTelegramStreamOutbox(ctx, row.Id, owner, job.Id, row.AttemptCount)
	}
	// Stream 连续失败达到阈值时，任何账户都必须走 Asynq 兜底；不能再依赖
	// 账户灰度配置，否则旧账户会无限重试而永远没有执行任务。
	if row.AttemptCount >= 3 && telegramOutboxJobStillNeedsDispatch(job.Status, job.DispatchStatus) {
		if fallbackErr := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); fallbackErr == nil {
			g.Log().Warning(ctx, "TG任务Outbox达到重试阈值，直接降级Asynq", g.Map{"event": "stream_fallback_enqueue", "jobId": job.Id, "outboxId": row.Id, "attemptCount": row.AttemptCount})
			return s.confirmTelegramStreamOutbox(ctx, row.Id, owner, job.Id, row.AttemptCount)
		}
	}
	if s.accountStreamPushEngineEnabled(ctx, job.TenantId, job.AccountId) {
		// Asynq 是发送主路径，Stream 仅作为旁路通知/观测通道。
		// Stream 不可用时不能阻塞频道；直接入队仍经过频道租约、队首和幂等校验。
		if err = s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); err == nil {
			g.Log().Debug(ctx, "TG任务绕过Stream进入Asynq主路径", g.Map{"event": "stream_bypass_direct_enqueue", "jobId": job.Id, "outboxId": row.Id})
			return s.confirmTelegramStreamOutbox(ctx, row.Id, owner, job.Id, row.AttemptCount)
		}
		// 手动/AI 运维任务是用户明确触发的交互操作，不能依赖 Stream
		// 作为关键路径。Stream 堵塞时用户点击必须仍能进入 Asynq；
		// 直接入队仍会执行频道前序检查、频道租约和 Job 幂等保护。
		if telegramJobPriorityValue(job) == tgJobPriorityInteractive {
			if err = s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); err != nil {
				return s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, err.Error())
			}
			g.Log().Info(ctx, "手动TG任务绕过Stream直接进入Asynq", g.Map{
				"event": "manual_direct_enqueue", "jobId": job.Id,
				"channelId": job.ChannelId, "accountId": job.AccountId,
			})
			return s.confirmTelegramStreamOutbox(ctx, row.Id, owner, job.Id, row.AttemptCount)
		}
		if err = s.publishTelegramReadyStream(ctx, job); err != nil {
			// 手动上架属于交互优先级。Stream 只是通知传输层，
			// Stream 写入失败不能让用户任务跟循环积压一起等待；
			// 直接走同一频道顺序校验的 Asynq fallback，仍保持幂等。
			if telegramJobPriorityValue(job) == tgJobPriorityInteractive || row.AttemptCount >= 3 {
				if fallbackErr := s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false); fallbackErr == nil {
					g.Log().Warning(ctx, "TG任务Stream失败已降级Asynq", g.Map{"event": "stream_fallback_enqueue", "jobId": job.Id, "outboxId": row.Id, "attemptCount": row.AttemptCount})
					return s.confirmTelegramStreamOutbox(ctx, row.Id, owner, job.Id, row.AttemptCount)
				} else {
					err = gerror.Wrapf(err, "手动TG任务Stream失败且Asynq兜底失败: %v", fallbackErr)
				}
			}
			return s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, err.Error())
		}
		// Stream 只代表就绪通知写入 Redis；必须确认执行任务已进入
		// queued/processing 并写入 task_id/asynq_task_id 后才能确认 Outbox。
		return s.confirmTelegramStreamOutbox(ctx, row.Id, owner, job.Id, row.AttemptCount)
	}
	g.Log().Info(ctx, "TG Outbox 租约已获取", g.Map{"event": "outbox_lease_acquired", "outboxId": row.Id, "jobId": row.JobId, "leaseOwner": owner})
	// available_at is a database wall-clock timestamp. The caller only selects
	// due rows, so do not convert it back to a Go duration: PostgreSQL timestamp
	// values can otherwise acquire an eight-hour offset when locations differ.
	if err = s.enqueueTelegramJobDirect(ctx, row.JobId, 0); err != nil {
		g.Log().Warning(ctx, "TG Outbox Redis 入队失败", g.Map{"event": "outbox_enqueue_failed", "outboxId": row.Id, "jobId": row.JobId, "leaseOwner": owner, "error": err.Error()})
		message := boundPublishSuccessRecordMessage(err.Error())
		_, _ = g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
			Where("id", row.Id).Where("status", telegramOutboxProcessing).Where("lease_owner", owner).
			Data(g.Map{
				"status": telegramOutboxPending, "lease_owner": "", "lease_expires_at": nil,
				"attempt_count": gdb.Raw("attempt_count + 1"), "last_error": message,
				"available_at": gtime.Now().Add(time.Minute), "updated_at": gtime.Now(),
			}).Update()
		return err
	}
	g.Log().Info(ctx, "TG Outbox Redis 入队成功", g.Map{"event": "outbox_enqueued", "outboxId": row.Id, "jobId": row.JobId, "leaseOwner": owner})
	job, err = s.telegramJobById(ctx, row.JobId)
	if err != nil {
		return s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, err.Error())
	}
	if telegramOutboxJobStillNeedsDispatch(job.Status, job.DispatchStatus) {
		beginTelegramPublishStage(ctx, "outbox.deferred").End(nil, g.Map{"reason": "still_needs_dispatch", "jobStatus": job.Status, "dispatchStatus": job.DispatchStatus})
		g.Log().Info(ctx, "TG Outbox 入队后任务仍待发送", g.Map{"event": "outbox_enqueue_deferred", "outboxId": row.Id, "jobId": row.JobId, "jobStatus": job.Status, "dispatchStatus": job.DispatchStatus})
		return s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, "频道发送槽繁忙，等待再次调度")
	}
	_, err = g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", row.Id).Where("status", telegramOutboxProcessing).Where("lease_owner", owner).
		Data(g.Map{
			"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil,
			"delivered_at": gtime.Now(), "last_error": "", "updated_at": gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "确认TG任务Outbox投递失败")
	}
	g.Log().Info(ctx, "TG Outbox 已确认投递", g.Map{"event": "outbox_delivered", "outboxId": row.Id, "jobId": row.JobId, "leaseOwner": owner})
	return nil
}

func (s *sSysPublish) completeTelegramOutbox(ctx context.Context, outboxId int64) error {
	if outboxId <= 0 {
		return nil
	}
	var row telegramJobOutboxRecord
	if err := g.DB().Model(publishTgOutboxTable+" o").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=o.job_id").
		Fields("o.id,o.job_id,j.tenant_id,j.channel_id,j.priority").Where("o.id", outboxId).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取TG任务Outbox失败")
	}
	_, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", outboxId).WhereNot("status", telegramOutboxDelivered).
		Data(g.Map{"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil,
			"delivered_at": gtime.Now(), "last_error": "", "updated_at": gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "收口TG任务Outbox失败")
	}
	if removeErr := (tgChannelPendingIndex{}).remove(ctx, row.TenantId, row.ChannelId, row.JobId); removeErr != nil {
		g.Log().Debug(ctx, "频道待投递索引删除失败，继续使用数据库事实", g.Map{"jobId": row.JobId, "channelId": row.ChannelId, "error": removeErr.Error()})
	}
	return nil
}

func (s *sSysPublish) telegramOutboxJobDispatchDecision(ctx context.Context, jobId int64, now *gtime.Time) (string, error) {
	if jobId <= 0 {
		return telegramOutboxDecisionComplete, nil
	}
	var job telegramJobRecord
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,channel_id,target_chat_id,status,dispatch_status,next_retry_at,send_phase,created_at,operation_no,priority").
		Where("id", jobId).Scan(&job); err != nil {
		return "", gerror.Wrap(err, "读取TG任务Outbox关联任务失败")
	}
	if job.Id <= 0 {
		return telegramOutboxDecisionComplete, nil
	}
	if !telegramOutboxJobStillNeedsDispatch(job.Status, job.DispatchStatus) {
		return telegramOutboxDecisionComplete, nil
	}
	if now == nil {
		now = gtime.Now()
	}
	hasEarlier, err := s.telegramChannelHasEarlierActiveJob(ctx, job)
	if err != nil {
		return "", err
	}
	beginTelegramPublishStage(ctx, "outbox.decision.inputs").End(nil, g.Map{
		"hasEarlier": hasEarlier, "nextRetryAt": job.NextRetryAt, "now": now,
		"createdAt": job.CreatedAt, "jobStatus": job.Status, "dispatchStatus": job.DispatchStatus,
	})
	return telegramOutboxDispatchDecision(job.Status, job.DispatchStatus, job.NextRetryAt, now, hasEarlier), nil
}

func telegramOutboxDispatchDecision(status, dispatchStatus string, nextRetryAt, now *gtime.Time, hasEarlier bool) string {
	if !telegramOutboxJobStillNeedsDispatch(status, dispatchStatus) {
		return telegramOutboxDecisionComplete
	}
	// PostgreSQL timestamp without time zone preserves calendar fields but
	// lib/pq attaches UTC. Compare those fields in the application's clock,
	// otherwise an expired local retry is treated as eight hours in the future.
	retryInFuture := false
	if nextRetryAt != nil && now != nil {
		value := nextRetryAt.Time
		retryTime := time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), now.Time.Location())
		retryInFuture = retryTime.After(now.Time)
	}
	if hasEarlier || retryInFuture {
		return telegramOutboxDecisionWait
	}
	return telegramOutboxDecisionDispatch
}

func telegramOutboxJobStillNeedsDispatch(status, dispatchStatus string) bool {
	status = strings.TrimSpace(status)
	dispatchStatus = strings.TrimSpace(dispatchStatus)
	return (status == "pending" || status == "failed_retry" || status == "unknown") &&
		(dispatchStatus == "" || dispatchStatus == tgDispatchStatusIdle)
}

func (s *sSysPublish) releaseTelegramOutbox(ctx context.Context, outboxId int64, owner string, attemptCount int, message string) error {
	_, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", outboxId).Where("status", telegramOutboxProcessing).Where("lease_owner", owner).
		Data(g.Map{
			"status": telegramOutboxPending, "lease_owner": "", "lease_expires_at": nil,
			"attempt_count": gdb.Raw("attempt_count + 1"), "last_error": boundPublishSuccessRecordMessage(message),
			"available_at": gtime.Now().Add(telegramOutboxRetryDelay(attemptCount)), "updated_at": gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "释放TG任务Outbox租约失败")
	}
	g.Log().Info(ctx, "TG Outbox 已释放等待重试", g.Map{"event": "outbox_released", "outboxId": outboxId, "leaseOwner": owner, "attemptCount": attemptCount + 1, "reason": message})
	return nil
}

func telegramOutboxRetryDelay(attemptCount int) time.Duration {
	if attemptCount < 0 {
		attemptCount = 0
	}
	delay := 3 * time.Second
	for i := 0; i < attemptCount && delay < time.Minute; i++ {
		delay *= 2
	}
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}

func (s *sSysPublish) dispatchDueTelegramJobOutbox(ctx context.Context, limit int) error {
	rows, err := s.dueTelegramJobOutboxRows(ctx, limit, gtime.Now())
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	jobs := make(chan telegramJobOutboxRecord)
	for worker := 0; worker < telegramOutboxWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for row := range jobs {
				if dispatchErr := s.dispatchTelegramJobOutbox(ctx, row); dispatchErr != nil {
					g.Log().Warningf(ctx, "投递TG任务Outbox失败 outboxId:%d jobId:%d err:%+v", row.Id, row.JobId, dispatchErr)
				}
			}
		}()
	}
	for _, row := range rows {
		jobs <- row
	}
	close(jobs)
	wg.Wait()
	if len(rows) > 0 {
		g.Log().Infof(ctx, "TG Outbox dispatcher selected channel heads:%d", len(rows))
	}
	return nil
}

func (s *sSysPublish) dueTelegramJobOutboxRows(ctx context.Context, limit int, now *gtime.Time) ([]telegramJobOutboxRecord, error) {
	queryCtx, cancelQuery := context.WithTimeout(ctx, telegramOutboxQueryTimeout)
	defer cancelQuery()
	// 全量统一使用频道级 Stream 调度；旧账户级队首 SQL 不再作为运行路径。
	return s.dueTelegramStreamOutboxRows(queryCtx, limit, now)
	if strings.EqualFold(strings.TrimSpace(g.DB().GetConfig().Type), "pgsql") {
		batchLimit := limit
		if batchLimit <= 0 {
			batchLimit = telegramOutboxBatchLimit(ctx)
		}
		if batchLimit > telegramOutboxMaxBatchLimit(ctx) {
			batchLimit = telegramOutboxMaxBatchLimit(ctx)
		}
		// 先按账户领取少量就绪账户，再逐账户执行频道队首查询，避免一个
		// 账户的历史积压在全局 NOT EXISTS 查询中耗尽数据库预算。
		var accountRows []struct {
			AccountID int64 `orm:"account_id"`
		}
		if err := g.DB().GetScan(queryCtx, &accountRows, `SELECT j.account_id AS account_id
			FROM `+publishTgOutboxTable+` o JOIN `+publishTgJobTable+` j ON j.id=o.job_id
			WHERE o.event_type=? AND o.available_at<=?
				AND (o.status=? OR (o.status=? AND o.lease_expires_at<=?))
				AND j.account_id>0
			GROUP BY j.account_id
			ORDER BY MIN(o.available_at), j.account_id LIMIT 64`,
			telegramOutboxEventPublish, now, telegramOutboxPending, telegramOutboxProcessing, now); err != nil {
			return nil, gerror.Wrap(err, "读取TG Outbox就绪账户失败")
		}
		accountIDs := make([]int64, 0, len(accountRows))
		for _, row := range accountRows {
			if row.AccountID > 0 {
				accountIDs = append(accountIDs, row.AccountID)
			}
		}
		rows := make([]telegramJobOutboxRecord, 0, batchLimit)
		perAccount := max(1, min(100, batchLimit/max(1, len(accountIDs))))
		for _, accountID := range accountIDs {
			accountQuery := telegramDueOutboxPostgresSQLForAccount()
			var accountRows []telegramJobOutboxRecord
			if err := g.DB().GetScan(queryCtx, &accountRows, accountQuery,
				telegramOutboxEventPublish, now, telegramOutboxPending, telegramOutboxProcessing, now,
				accountID, now, perAccount,
			); err != nil {
				return nil, gerror.Wrapf(err, "读取账户TG任务Outbox失败 accountId:%d", accountID)
			}
			rows = append(rows, accountRows...)
		}
		// 兼容尚未迁移的旧任务。正常账户路径明确排除 account_id=0，
		// 但不能因此静默丢弃历史任务；只有确实存在到期旧任务时才执行
		// 这条额外查询，避免无效 legacy 扫描拖慢正常账户调度。
		if len(rows) < batchLimit {
			legacyExists, err := g.DB().GetValue(queryCtx, `SELECT EXISTS(
				SELECT 1 FROM `+publishTgOutboxTable+` o
				JOIN `+publishTgJobTable+` j ON j.id=o.job_id
				WHERE o.event_type=? AND o.available_at<=?
				  AND (o.status=? OR (o.status=? AND o.lease_expires_at<=?))
				  AND j.account_id<=0
			)`, telegramOutboxEventPublish, now, telegramOutboxPending, telegramOutboxProcessing, now)
			if err != nil {
				return nil, gerror.Wrap(err, "检查旧账户TG任务Outbox失败")
			}
			if legacyExists.Bool() {
				legacyLimit := batchLimit - len(rows)
				if legacyLimit > 100 {
					legacyLimit = 100
				}
				var legacyRows []telegramJobOutboxRecord
				if err := g.DB().GetScan(queryCtx, &legacyRows, telegramDueOutboxPostgresSQLForAccount(),
					telegramOutboxEventPublish, now, telegramOutboxPending, telegramOutboxProcessing, now,
					int64(0), now, legacyLimit); err != nil {
					return nil, gerror.Wrap(err, "读取旧账户TG任务Outbox失败")
				}
				rows = append(rows, legacyRows...)
			}
		}
		if len(rows) > batchLimit {
			rows = rows[:batchLimit]
		}
		if limit > 0 && len(rows) > limit {
			rows = rows[:limit]
		}
		return rows, nil
	}

	candidateLimit := limit * 10
	if candidateLimit <= 0 {
		candidateLimit = 10000
	}
	if candidateLimit > 2000 {
		candidateLimit = 2000
	}
	var candidates []telegramJobOutboxRecord
	err := g.DB().Model(publishTgOutboxTable+" o").Safe().Ctx(queryCtx).
		LeftJoin(publishTgJobTable+" j", "j.id=o.job_id").
		Fields("o.id,o.job_id,o.attempt_count,o.available_at,j.tenant_id,j.channel_id,j.target_chat_id").
		Where("o.event_type", telegramOutboxEventPublish).
		Where("o.available_at <= ?", now).
		Where("o.status = ? OR (o.status = ? AND o.lease_expires_at <= ?)", telegramOutboxPending, telegramOutboxProcessing, now).
		OrderAsc("o.available_at").OrderAsc("o.id").Limit(candidateLimit).Scan(&candidates)
	if err != nil {
		return nil, gerror.Wrap(err, "读取待投递TG任务Outbox失败")
	}
	fairRows := fairTelegramOutboxRows(candidates, candidateLimit)
	rows := make([]telegramJobOutboxRecord, 0, min(limit, len(fairRows)))
	for _, row := range fairRows {
		decision, checkErr := s.telegramOutboxJobDispatchDecision(queryCtx, row.JobId, now)
		if checkErr != nil {
			return nil, checkErr
		}
		if decision != telegramOutboxDecisionDispatch {
			continue
		}
		rows = append(rows, row)
		if len(rows) >= limit {
			break
		}
	}
	return rows, nil
}

// telegramDueOutboxPostgresSQLForAccount 将旧的频道队首 SQL 限定到单个账户，
// 让账户之间共享公平配额而不是共享一次全局候选扫描。
func telegramDueOutboxPostgresSQLForAccount() string {
	query := telegramDueOutboxPostgresSQL()
	needle := "\n\t\tJOIN " + publishTgJobTable + " j ON j.id=o.job_id\n\t), active_heads AS ("
	replacement := "\n\t\tJOIN " + publishTgJobTable + " j ON j.id=o.job_id\n\t\tWHERE j.account_id=?\n\t), active_heads AS ("
	return strings.Replace(query, needle, replacement, 1)
}

// dueTelegramStreamOutboxRows 读取全量 Stream 模式下的待通知 Outbox。
// 频道顺序由发送阶段保证，这里只按到期时间取数，避免扫描频道队首排序 SQL。
func (s *sSysPublish) dueTelegramStreamOutboxRows(ctx context.Context, limit int, now *gtime.Time) ([]telegramJobOutboxRecord, error) {
	if limit <= 0 {
		limit = telegramOutboxBatchLimit(ctx)
	}
	// Stream dispatcher 只做可靠通知，不执行频道发送；适当增大批次可以
	// 一轮排空更多独立频道，减少秒级轮询等待。频道 lease 仍在发送阶段保证串行。
	if limit > telegramStreamOutboxBatchLimit(ctx) {
		limit = telegramStreamOutboxBatchLimit(ctx)
	}
	if limit > telegramOutboxMaxBatchLimit(ctx) {
		limit = telegramOutboxMaxBatchLimit(ctx)
	}
	// Stream dispatcher 只负责可靠通知，频道顺序由发送阶段 lease 保证。
	// 不在这里执行全局 DISTINCT ON 频道头部聚合：历史积压会迫使数据库
	// 对整个候选集排序，直接拖慢实时通知。先按到期时间取有界候选，
	// 再在内存中做账户/频道配额控制。
	var candidates []telegramJobOutboxRecord
	query := `SELECT o.id,o.job_id,o.attempt_count,o.available_at,j.account_id,j.tenant_id,j.channel_id,j.priority
		FROM ` + publishTgOutboxTable + ` o
		JOIN ` + publishTgJobTable + ` j ON j.id=o.job_id
		WHERE j.tenant_id>0 AND j.channel_id>0
			AND o.event_type=? AND o.available_at<=?
			AND (o.status=? OR (o.status=? AND o.lease_expires_at<=?))
			AND j.status IN ('pending','failed_retry','unknown')
			AND COALESCE(j.dispatch_status,'') IN ('','idle')
			AND NOT (j.status='pending' AND COALESCE(j.dispatch_status,'')='idle' AND COALESCE(j.asynq_task_id,'')='' AND j.updated_at < CURRENT_TIMESTAMP - (? * INTERVAL '1 minute'))
			AND (j.next_retry_at IS NULL OR j.next_retry_at<=? OR (j.status='pending' AND COALESCE(j.dispatch_status,'')='idle'))
		ORDER BY o.available_at,o.id LIMIT ?`
	if err := g.DB().GetScan(ctx, &candidates, query, telegramOutboxEventPublish, now, telegramOutboxPending, telegramOutboxProcessing, telegramChannelHeadTimeoutMinutes(), now, limit*2); err != nil {
		return nil, gerror.Wrap(err, "读取Stream频道队首失败")
	}
	var rows []telegramJobOutboxRecord
	// 将本轮发现的队首同步到频道索引；Redis 失败只记录日志，数据库仍是可靠事实源。
	for _, candidate := range candidates {
		if err := (tgChannelPendingIndex{}).add(ctx, candidate.TenantId, candidate.ChannelId, candidate.JobId, candidate.Priority); err != nil {
			g.Log().Debug(ctx, "频道待投递索引写入失败，继续使用数据库兜底", g.Map{"jobId": candidate.JobId, "channelId": candidate.ChannelId, "error": err.Error()})
		}
	}
	// 每个账户本轮最多占用固定频道配额，避免大账户耗尽 dispatcher。
	accountQuota := max(1, limit/8)
	// 配额按租户和账户联合分片；account_id=0 是 BOT 采集的合法归属，
	// 不能让不同租户的 BOT 任务共享同一个限额。
	type accountShard struct {
		tenantID  int64
		accountID int64
	}
	accountSelected := make(map[accountShard]int)
	type channelShard struct {
		tenantID  int64
		channelID int64
	}
	channelSelected := make(map[channelShard]struct{})
	for _, candidate := range candidates {
		if candidate.ChannelId > 0 {
			shard := channelShard{tenantID: candidate.TenantId, channelID: candidate.ChannelId}
			if _, exists := channelSelected[shard]; exists {
				continue
			}
			channelSelected[shard] = struct{}{}
		}
		shard := accountShard{tenantID: candidate.TenantId, accountID: candidate.AccountId}
		if accountSelected[shard] >= accountQuota {
			continue
		}
		accountSelected[shard]++
		rows = append(rows, candidate)
		if len(rows) >= limit {
			break
		}
	}
	return rows, nil
}

func telegramDueOutboxPostgresSQL() string {
	channelKey := telegramOutboxChannelKeySQL("j")
	earlierChannelKey := telegramOutboxChannelKeySQL("earlier")
	return `WITH due_outboxes AS MATERIALIZED (
		SELECT id,job_id,attempt_count,available_at
		FROM ` + publishTgOutboxTable + `
		WHERE event_type=? AND available_at<=?
			AND (status=? OR (status=? AND lease_expires_at<=?))
		ORDER BY available_at,id
		LIMIT 2000
		FOR UPDATE SKIP LOCKED
	), due_jobs AS MATERIALIZED (
		SELECT o.id,o.job_id,o.attempt_count,o.available_at,
			j.tenant_id,j.account_id,j.channel_id,j.target_chat_id,j.status,j.dispatch_status,j.next_retry_at,
			j.created_at,j.operation_no,j.send_phase,j.priority,
			` + telegramChannelScheduleRankSQL("j") + ` AS schedule_rank
		FROM due_outboxes o
		JOIN ` + publishTgJobTable + ` j ON j.id=o.job_id
	), active_heads AS (
		SELECT j.id,j.job_id,j.attempt_count,j.available_at,j.tenant_id,j.account_id,j.channel_id,j.target_chat_id,
			j.status,j.dispatch_status,j.next_retry_at,j.created_at,j.operation_no,j.send_phase,j.priority,j.schedule_rank
		FROM due_jobs j
		WHERE j.status IN ('pending','failed_retry','unknown')
			AND COALESCE(j.dispatch_status,'') IN ('','idle')
			AND (j.next_retry_at IS NULL OR j.next_retry_at<=? OR (j.status='pending' AND COALESCE(j.dispatch_status,'')='idle'))
			AND NOT EXISTS (
				SELECT 1 FROM ` + publishTgJobTable + ` earlier
				WHERE earlier.tenant_id=j.tenant_id
					AND ` + earlierChannelKey + `=` + channelKey + `
					AND ` + telegramChannelBlockingJobStateSQL("earlier") + `
					AND ` + telegramChannelSchedulePrecedesSQL("earlier", "j") + `
			)
	), terminal_outboxes AS (
		SELECT id,job_id,attempt_count,available_at,tenant_id,account_id,channel_id,target_chat_id,999 AS schedule_rank,0 AS effective_priority
		FROM due_jobs
		WHERE status IN ('sent','failed','superseded')
	), account_heads AS (
		SELECT h.*, ROW_NUMBER() OVER (PARTITION BY h.tenant_id,h.account_id ORDER BY h.schedule_rank,h.effective_priority,h.available_at,h.id) AS account_rank
		FROM (
			SELECT id,job_id,attempt_count,available_at,tenant_id,account_id,channel_id,target_chat_id,schedule_rank,` + telegramJobEffectivePrioritySQL("") + ` AS effective_priority
			FROM active_heads
			UNION ALL
			SELECT id,job_id,attempt_count,available_at,tenant_id,account_id,channel_id,target_chat_id,schedule_rank,effective_priority
			FROM terminal_outboxes
		) h
		WHERE h.account_id > 0
	), unscoped_heads AS (
		SELECT h.*, ROW_NUMBER() OVER (ORDER BY h.schedule_rank,h.effective_priority,h.available_at,h.id) AS account_rank
		FROM (
			SELECT id,job_id,attempt_count,available_at,tenant_id,account_id,channel_id,target_chat_id,schedule_rank,` + telegramJobEffectivePrioritySQL("") + ` AS effective_priority
			FROM active_heads
			WHERE account_id <= 0
			UNION ALL
			SELECT id,job_id,attempt_count,available_at,tenant_id,account_id,channel_id,target_chat_id,schedule_rank,effective_priority
			FROM terminal_outboxes
			WHERE account_id <= 0
		) h
	)
	SELECT id,job_id,attempt_count,available_at,tenant_id,channel_id,target_chat_id,
		schedule_rank,effective_priority
	FROM account_heads WHERE account_rank <= 100
	UNION ALL
	SELECT id,job_id,attempt_count,available_at,tenant_id,channel_id,target_chat_id,schedule_rank,effective_priority
	FROM unscoped_heads WHERE account_rank <= 100
	ORDER BY schedule_rank,effective_priority,available_at,id
	LIMIT ?`
}

func telegramOutboxChannelKeySQL(alias string) string {
	return "CASE WHEN NULLIF(REPLACE(COALESCE(" + alias + ".target_chat_id,''), '-100', ''),'') IS NOT NULL THEN 'chat:' || REPLACE(COALESCE(" + alias + ".target_chat_id,''), '-100', '') WHEN " + alias + ".channel_id > 0 THEN " + alias + ".channel_id::text ELSE 'chat:' || COALESCE(" + alias + ".target_chat_id,'') END"
}

func fairTelegramOutboxRows(candidates []telegramJobOutboxRecord, limit int) []telegramJobOutboxRecord {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	rows := make([]telegramJobOutboxRecord, 0, min(limit, len(candidates)))
	seen := make(map[string]struct{}, len(candidates))
	for _, row := range candidates {
		key := fmt.Sprintf("%d:%d:%s", row.TenantId, row.ChannelId, normalizeTelegramChannelChatID(row.TargetChatId))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, row)
		if len(rows) >= limit {
			break
		}
	}
	return rows
}
