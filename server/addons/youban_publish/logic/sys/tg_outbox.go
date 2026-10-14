package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
)

const (
	telegramOutboxEventPublish = "telegram_job.publish"
	telegramOutboxPending      = "pending"
	telegramOutboxProcessing   = "processing"
	telegramOutboxDelivered    = "delivered"
	telegramOutboxLeaseTTL     = 30 * time.Second
)

type telegramJobOutboxRecord struct {
	Id           int64       `json:"id"`
	JobId        int64       `json:"jobId"`
	AttemptCount int         `json:"attemptCount"`
	AvailableAt  *gtime.Time `json:"availableAt"`
	TenantId     int64       `json:"tenantId"`
	ChannelId    int64       `json:"channelId"`
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
	}
}

func (s *sSysPublish) dispatchTelegramJobOutbox(ctx context.Context, row telegramJobOutboxRecord) error {
	owner := telegramOutboxOwner()
	now := gtime.Now()
	result, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", row.Id).
		Where("status = ? OR (status = ? AND lease_expires_at <= ?)", telegramOutboxPending, telegramOutboxProcessing, now).
		Data(g.Map{
			"status": telegramOutboxProcessing, "lease_owner": owner,
			"lease_expires_at": now.Add(telegramOutboxLeaseTTL), "updated_at": now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "领取TG任务Outbox失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	// available_at is a database wall-clock timestamp. The caller only selects
	// due rows, so do not convert it back to a Go duration: PostgreSQL timestamp
	// values can otherwise acquire an eight-hour offset when locations differ.
	if err = s.enqueueTelegramJobDirect(ctx, row.JobId, 0); err != nil {
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
	job, err := s.telegramJobById(ctx, row.JobId)
	if err != nil {
		return s.releaseTelegramOutbox(ctx, row.Id, owner, row.AttemptCount, err.Error())
	}
	if telegramOutboxJobStillNeedsDispatch(job.Status, job.DispatchStatus) {
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
	return nil
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
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	candidateLimit := limit * 10
	if candidateLimit > 2000 {
		candidateLimit = 2000
	}
	var candidates []telegramJobOutboxRecord
	err := g.DB().Model(publishTgOutboxTable+" o").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=o.job_id").
		Fields("o.id,o.job_id,o.attempt_count,o.available_at,j.tenant_id,j.channel_id,j.target_chat_id").
		Where("o.event_type", telegramOutboxEventPublish).
		Where("o.available_at <= ?", now).
		Where("o.status = ? OR (o.status = ? AND o.lease_expires_at <= ?)", telegramOutboxPending, telegramOutboxProcessing, now).
		OrderAsc("o.available_at").OrderAsc("o.id").Limit(candidateLimit).Scan(&candidates)
	if err != nil {
		return gerror.Wrap(err, "读取待投递TG任务Outbox失败")
	}
	rows := fairTelegramOutboxRows(candidates, limit)
	for _, row := range rows {
		if err = s.dispatchTelegramJobOutbox(ctx, row); err != nil {
			g.Log().Warningf(ctx, "投递TG任务Outbox失败 outboxId:%d jobId:%d err:%+v", row.Id, row.JobId, err)
		}
	}
	return nil
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

func (s *sSysPublish) runTelegramOutboxDispatcher(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := s.dispatchDueTelegramJobOutbox(ctx, 100); err != nil {
			g.Log().Warningf(ctx, "调度TG任务Outbox失败：%+v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RecoverTelegramDispatch runs one bounded scheduler pass. It is used by
// operational repair tooling and never sends Telegram messages itself.
func (s *sSysPublish) RecoverTelegramDispatch(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	if err := s.dispatchDueTelegramJobOutbox(ctx, limit); err != nil {
		return err
	}
	return s.recoverPendingIdleTelegramJobs(ctx, limit)
}

// RecoverTelegramJobDispatch schedules one existing job through its Outbox,
// preserving available_at and the normal idempotent queue path.
func (s *sSysPublish) RecoverTelegramJobDispatch(ctx context.Context, jobId int64) error {
	return s.dispatchTelegramJobOutboxByJob(ctx, jobId)
}

func telegramOutboxOwner() string {
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = "worker"
	}
	return hostname + ":" + guid.S()
}
