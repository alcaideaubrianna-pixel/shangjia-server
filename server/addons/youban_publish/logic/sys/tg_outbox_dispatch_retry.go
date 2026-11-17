package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// deferPendingTelegramOutbox backs off only the failed delivery generation.
// Concurrent completion, a new lease or another retry must never be overwritten.
func (s *sSysPublish) deferPendingTelegramOutbox(ctx context.Context, row telegramJobOutboxRecord, cause error) error {
	if row.Id <= 0 || row.AvailableAt == nil || cause == nil {
		return nil
	}
	now := gtime.Now()
	delay := telegramOutboxRetryDelay(row.AttemptCount)
	result, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("id", row.Id).
		Where("job_id", row.JobId).
		Where("event_type", telegramOutboxEventPublish).
		Where("status", telegramOutboxPending).
		Where("attempt_count", row.AttemptCount).
		Where("available_at", row.AvailableAt.Time.Format("2006-01-02 15:04:05.000000")).
		WhereLTE("available_at", now).
		Data(g.Map{
			"attempt_count": gdb.Raw("attempt_count + 1"),
			"available_at":  now.Add(delay),
			"last_error":    boundPublishSuccessRecordMessage(cause.Error()),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "TG Outbox失败投递退避失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取TG Outbox退避结果失败")
	}
	if affected > 0 {
		g.Log().Warning(ctx, "TG Outbox投递失败，已延期重试", g.Map{
			"event": "outbox_dispatch_deferred", "outboxId": row.Id, "jobId": row.JobId,
			"channelId": row.ChannelId, "tenantId": row.TenantId, "accountId": row.AccountId,
			"attemptCount": row.AttemptCount + 1, "retryDelayMs": delay.Milliseconds(),
			"error": boundPublishSuccessRecordMessage(cause.Error()),
		})
	}
	return nil
}
