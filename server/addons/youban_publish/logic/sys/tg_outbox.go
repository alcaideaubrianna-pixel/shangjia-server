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
	Id          int64       `json:"id"`
	JobId       int64       `json:"jobId"`
	AvailableAt *gtime.Time `json:"availableAt"`
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
	var row telegramJobOutboxRecord
	if err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("event_type", telegramOutboxEventPublish).
		WhereNot("status", telegramOutboxDelivered).
		OrderAsc("id").Limit(1).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取TG任务Outbox失败")
	}
	if row.Id <= 0 {
		// Compatibility for jobs created by older code paths during migration.
		return s.enqueueTelegramJobDirect(ctx, jobId, 0)
	}
	return s.dispatchTelegramJobOutbox(ctx, row)
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
	delay := time.Duration(0)
	if row.AvailableAt != nil && row.AvailableAt.After(now) {
		delay = row.AvailableAt.Sub(now)
	}
	if err = s.enqueueTelegramJobDirect(ctx, row.JobId, delay); err != nil {
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
		return s.releaseTelegramOutbox(ctx, row.Id, owner, err.Error())
	}
	if telegramOutboxJobStillNeedsDispatch(job.Status, job.DispatchStatus) {
		return s.releaseTelegramOutbox(ctx, row.Id, owner, "频道发送槽繁忙，等待再次调度")
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

func (s *sSysPublish) releaseTelegramOutbox(ctx context.Context, outboxId int64, owner, message string) error {
	_, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", outboxId).Where("status", telegramOutboxProcessing).Where("lease_owner", owner).
		Data(g.Map{
			"status": telegramOutboxPending, "lease_owner": "", "lease_expires_at": nil,
			"attempt_count": gdb.Raw("attempt_count + 1"), "last_error": boundPublishSuccessRecordMessage(message),
			"available_at": gtime.Now().Add(3 * time.Second), "updated_at": gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "释放TG任务Outbox租约失败")
	}
	return nil
}

func (s *sSysPublish) dispatchDueTelegramJobOutbox(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	var rows []telegramJobOutboxRecord
	err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("event_type", telegramOutboxEventPublish).
		Where("available_at <= ?", now).
		Where("status = ? OR (status = ? AND lease_expires_at <= ?)", telegramOutboxPending, telegramOutboxProcessing, now).
		OrderAsc("available_at").OrderAsc("id").Limit(limit).Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取待投递TG任务Outbox失败")
	}
	for _, row := range rows {
		if err = s.dispatchTelegramJobOutbox(ctx, row); err != nil {
			g.Log().Warningf(ctx, "投递TG任务Outbox失败 outboxId:%d jobId:%d err:%+v", row.Id, row.JobId, err)
		}
	}
	return nil
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

func telegramOutboxOwner() string {
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = "worker"
	}
	return hostname + ":" + guid.S()
}
