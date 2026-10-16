package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// Called under the existing recovery distributed lock. Each transaction and
// scan is bounded; no Telegram calls, profile/media deletion or Redis purge.
func (s *sSysPublish) maintainPermissionPausedBacklog(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Retention must still progress when cancelling a large historical backlog.
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return prunePermissionCancelledJobsTx(ctx, tx, gtime.New(time.Now().AddDate(0, -1, 0)))
	}); err != nil {
		return err
	}
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,channel_id").WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusRetryWait}).
		Where(permissionUntouchedJobSQL()).
		Where("LOWER(TRIM(operation_no)) NOT LIKE ?", "down:%").
		Where("operation_no NOT LIKE ?", "message_push:%").Where("operation_no NOT LIKE ?", "message_push_plan:%").
		Where("EXISTS (SELECT 1 FROM " + channelPermissionCircuitTable + " c WHERE c.tenant_id=" + publishTgJobTable + ".tenant_id AND c.channel_id=" + publishTgJobTable + ".channel_id AND (c.paused=1 OR " + publishTgJobTable + ".created_at<=c.resumed_at))").
		OrderAsc("id").Limit(200).Scan(&jobs)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		job.Status = telegramJobStatusPending
		if _, err := s.postponePermissionPausedJob(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func prunePermissionCancelledJobsTx(ctx context.Context, tx gdb.TX, cutoff *gtime.Time) error {
	var jobs []telegramJobRecord
	if err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Fields("id").
		Where("status", telegramJobStatusSuperseded).Where("last_dispatch_error", channelPermissionCancelledMessage).
		Where("updated_at <= ?", cutoff).Where(permissionUntouchedJobSQL()).
		OrderAsc("updated_at").OrderAsc("id").Limit(200).LockUpdate().Scan(&jobs); err != nil {
		return err
	}
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.Id)
	}
	if len(ids) == 0 {
		return nil
	}
	// These jobs never contacted Telegram. Their transport and UI dispatch logs
	// can be removed atomically, while all source profiles remain untouched.
	for _, table := range []string{publishTgOutboxTable, publishSuccessRecordTable} {
		if _, err := tx.Model(table).Safe().Ctx(ctx).Unscoped().WhereIn("job_id", ids).Delete(); err != nil {
			return err
		}
	}
	_, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().WhereIn("id", ids).Delete()
	return err
}
