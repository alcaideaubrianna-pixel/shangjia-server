// Bounded delivery recovery evaluates due-time predicates after candidate paging.
package sys

import (
	"context"
	"errors"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

// Each query reads at most one scope/pool candidate page using the recovery
// partial index. The SQL creation fence keeps the snapshot finite as new rows
// arrive; only due-time/attempt eligibility is evaluated after LIMIT. Ineligible
// candidates still consume quota and advance the cursor.
func deliveryRecoveryBoundedCandidatePage(ctx context.Context, scope collectorRecoveryScope, pool string, cursor deliveryRecoveryCursor, limit int) (gdb.Result, error) {
	if !scope.valid() || (pool != "realtime" && pool != "history") || cursor.UntilTime == "" || cursor.StaleBefore == "" {
		return nil, gerror.New("交付恢复分页范围不完整")
	}
	if limit <= 0 {
		return nil, nil
	}
	limit = min(limit, deliveryRecoveryPageSize)
	cols := dao.TgCollectorDelivery.Columns()
	model := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(cols.Id, cols.Priority, cols.Status, "to_char(created_at,'YYYY-MM-DD HH24:MI:SS.US') AS "+deliveryRecoveryCreatedField,
			cols.AttemptCount,
			"to_char("+cols.UpdatedAt+",'YYYY-MM-DD HH24:MI:SS.US') AS recovery_updated_at",
			"to_char("+cols.NextRunAt+",'YYYY-MM-DD HH24:MI:SS.US') AS recovery_next_run_at").
		Where(cols.TenantId, scope.TenantID).Where("account_id", scope.AccountID).Where("source_id", scope.SourceID).
		Where("scheduler_pool", pool).
		Where(cols.Status+" IN ('pending','failed_retry')").
		WhereLTE(cols.CreatedAt, cursor.UntilTime)

	order := cols.Priority + " DESC, " + cols.CreatedAt + " ASC, " + cols.Id + " ASC"
	if cursor.AfterID <= 0 {
		return model.Order(order).Limit(limit).All()
	}
	// Seek the current priority first, then lower priorities. A mixed-direction
	// OR keyset can leave PostgreSQL filtering every earlier row in the index.
	// Both queries below have a directly indexable range and return at most one
	// page in total, without assuming a fixed set of legacy priority values.
	rows, err := model.Clone().
		Where(cols.Priority, cursor.AfterPriority).
		Where("("+cols.CreatedAt+","+cols.Id+") > (?::timestamp,?)", cursor.AfterCreatedAt, cursor.AfterID).
		Order(order).Limit(limit).All()
	if err != nil || len(rows) == limit {
		return rows, err
	}
	lower, err := model.Clone().WhereLT(cols.Priority, cursor.AfterPriority).
		Order(order).Limit(limit - len(rows)).All()
	if err != nil {
		return nil, err
	}
	return append(rows, lower...), nil
}

// deliveryRecoveryCandidateEligible evaluates due conditions after the bounded
// index page has been read. Fixed-width timestamps preserve database microseconds
// and the existing naive business timezone; NULL clocks remain ineligible.
func deliveryRecoveryCandidateEligible(row gdb.Record, cursor deliveryRecoveryCursor) bool {
	cols := dao.TgCollectorDelivery.Columns()
	created := row[deliveryRecoveryCreatedField].String()
	if created == "" || created > cursor.UntilTime || row[cols.AttemptCount].Int() >= deliveryMaxAttempts {
		return false
	}
	switch row[cols.Status].String() {
	case sysin.DeliveryStatusPending:
		updated := row["recovery_updated_at"].String()
		return updated != "" && updated <= cursor.StaleBefore
	case sysin.DeliveryStatusFailedRetry:
		next := row["recovery_next_run_at"].String()
		return next != "" && next <= cursor.UntilTime
	default:
		return false
	}
}

func (s *sCollector) recoverDeliveryBoundedCursorPage(ctx context.Context, scope collectorRecoveryScope, pool string, limit int, upper deliveryRecoveryCursor) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	pageStarted := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lease := lock.NewConfig(10*time.Second, time.Second).WithoutWatchDog().Mutex(consts.DeliveryRecoveryLockKey(scope.key(), pool))
	if err := lease.TryLock(ctx); err != nil {
		if errors.Is(err, lock.ErrLockFailed) {
			return 0, nil
		}
		return 0, errors.Join(errCollectorRecoveryRedisUnavailable, err)
	}
	defer func() {
		unlockCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
		defer stop()
		_ = lease.Unlock(unlockCtx)
	}()
	key := consts.DeliveryRecoveryCursorKey(scope.key(), pool)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return 0, errors.Join(errCollectorRecoveryRedisUnavailable, err)
	}
	var cursor deliveryRecoveryCursor
	if value != nil && !value.IsNil() {
		if err := value.Scan(&cursor); err != nil {
			return 0, gerror.Wrap(err, "读取交付恢复游标失败")
		}
	}
	if cursor.UntilTime == "" {
		cursor = upper
		if cursor.UntilTime == "" || cursor.StaleBefore == "" {
			return 0, gerror.New("交付恢复缺少本轮时间上界")
		}
	}
	limit = min(limit, deliveryRecoveryPageSize)
	meta := collectorObservationMeta{QueueClass: collectorObserveQueueClassFromClass(pool),
		TenantID: scope.TenantID, AccountID: scope.AccountID, SourceID: scope.SourceID}
	scanCtx, observation := startCollectorStage(ctx, collectorStageDeliveryScan, meta)
	rows, err := deliveryRecoveryBoundedCandidatePage(scanCtx, scope, pool, cursor, limit)
	observation.Finish(collectorResultOK, err)
	if err != nil {
		return 0, err
	}
	observeCollectorRecoveryCandidates(scanCtx, collectorStageDeliveryScan, meta, len(rows))
	quarantine := func(row gdb.Record, cause error) error {
		c := dao.TgCollectorDelivery.Columns()
		result, err := dao.TgCollectorDelivery.Ctx(ctx).Where(c.Id, row[c.Id].Int64()).
			Where(c.TenantId, scope.TenantID).Where("account_id", scope.AccountID).Where("source_id", scope.SourceID).
			WhereIn(c.Status, []string{sysin.DeliveryStatusPending, sysin.DeliveryStatusFailedRetry}).
			Data(g.Map{c.Status: sysin.DeliveryStatusQuarantined, c.ErrorMessage: "恢复交接失败，已隔离待修复：" + cause.Error()}).Update()
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return gerror.New("交付状态已变化，未确认隔离，保留恢复游标")
		}
		g.Log().Warning(ctx, "交付恢复坏行已隔离，继续后续候选", g.Map{"deliveryId": row[c.Id].Int64(), "scope": scope.key(), "error": cause.Error()})
		return nil
	}
	cursor, submitErr := walkDeliveryRecoveryPage(rows, cursor, limit, func(row gdb.Record) error {
		// The index page includes ineligible rows deliberately: advancing past
		// them bounds scan work and prevents a future retry from pinning a lane.
		if !deliveryRecoveryCandidateEligible(row, cursor) {
			return nil
		}
		deliveryID := row[dao.TgCollectorDelivery.Columns().Id].Int64()
		priority, err := s.persistedDeliveryPriority(ctx, deliveryID)
		if err != nil {
			if !errors.Is(err, errDeliveryInvalidEvent) {
				g.Log().Warning(ctx, "交付恢复读取失败，保留原状态下轮重试", g.Map{"deliveryId": deliveryID, "scope": scope.key(), "error": err.Error()})
				return err
			}
			// Only a verified invalid event is eligible for quarantine.
			// A failed quarantine leaves business state intact for the next sweep.
			return quarantine(row, err)
		}
		enqueueMeta := meta
		enqueueMeta.DeliveryID = deliveryID
		enqueueMeta.QueueClass = collectorObserveQueueClassFromPriority(priority)
		enqueueCtx, enqueueObservation := startCollectorStage(ctx, collectorStageDeliveryReenqueue, enqueueMeta)
		err = s.enqueueDeliveryTaskWithOptions(enqueueCtx, deliveryID, priority, false)
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			enqueueObservation.Finish(collectorResultError, err)
		} else {
			enqueueObservation.Finish(collectorResultOK, err)
		}
		if err != nil {
			g.Log().Warning(ctx, "交付恢复未确认入队，扫描继续但不确认业务完成", g.Map{"deliveryId": deliveryID, "scope": scope.key(), "error": err.Error()})
		}
		return classifyDeliveryRecoveryEnqueueError(err)
	})
	// Preserve partial progress even after the lane deadline, but fence the
	// write against lease expiry/takeover before publishing a shared cursor.
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
	defer stop()
	owned, refreshErr := lease.Refresh(saveCtx)
	if refreshErr != nil {
		return len(rows), errors.Join(submitErr, errCollectorRecoveryRedisUnavailable, refreshErr)
	}
	if !owned {
		return len(rows), errors.Join(submitErr, gerror.New("交付恢复游标租约已失效"))
	}
	if saveErr := cache.Instance().Set(saveCtx, key, cursor, deliveryRecoveryCursorTTL); saveErr != nil {
		return len(rows), errors.Join(submitErr, errCollectorRecoveryRedisUnavailable, saveErr)
	}
	logCollectorDeliveryStage(saveCtx, "delivery.recovery.page", 0, 0, "", submitErr, g.Map{
		"tenantId": scope.TenantID, "accountId": scope.AccountID, "sourceId": scope.SourceID,
		"scope": scope.key(), "recoveryPool": pool, "candidateCount": len(rows),
		"cursorAfterId": cursor.AfterID, "cursorAfterPriority": cursor.AfterPriority,
		"cursorAfterCreatedAt": cursor.AfterCreatedAt, "scanUntilTime": cursor.UntilTime,
		"windowCompleted": cursor.UntilTime == "", "durationMs": time.Since(pageStarted).Milliseconds(),
	})
	return len(rows), submitErr
}
