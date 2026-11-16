package sys

import (
	"context"
	"errors"
	"strings"
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

const (
	deliveryRecoveryPageSize     = 8
	deliveryRecoveryCursorTTL    = 24 * time.Hour
	deliveryRecoveryCreatedField = "recovery_created_at"
)

type deliveryRecoveryCursor struct {
	UntilTime      string `json:"untilTime"`
	StaleBefore    string `json:"staleBefore"`
	AfterPriority  int    `json:"afterPriority"`
	AfterCreatedAt string `json:"afterCreatedAt"`
	AfterID        int64  `json:"afterId"`
}

func newDeliveryRecoveryCursor(ctx context.Context) (deliveryRecoveryCursor, error) {
	// Match the project's naive Asia/Shanghai business timestamps, independent
	// of the database session timezone. This scalar query reads no business table.
	row, err := g.DB().GetOne(ctx, `SELECT
		to_char(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI:SS.US') AS until_time,
		to_char((CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai') - ? * INTERVAL '1 second','YYYY-MM-DD HH24:MI:SS.US') AS stale_before`,
		int64(deliveryRecoveryStaleTime/time.Second))
	if err != nil {
		return deliveryRecoveryCursor{}, gerror.Wrap(err, "读取交付恢复扫描时间上界失败")
	}
	cursor := deliveryRecoveryCursor{UntilTime: row["until_time"].String(), StaleBefore: row["stale_before"].String()}
	if cursor.UntilTime == "" || cursor.StaleBefore == "" {
		return deliveryRecoveryCursor{}, gerror.New("交付恢复扫描时间上界为空")
	}
	return cursor, nil
}

func deliveryRecoveryPage(ctx context.Context, scope collectorRecoveryScope, pool string, cursor deliveryRecoveryCursor, limit int) (gdb.Result, error) {
	if !scope.valid() || (pool != "realtime" && pool != "history") || cursor.UntilTime == "" || cursor.StaleBefore == "" {
		return nil, gerror.New("交付恢复分页范围不完整")
	}
	if limit <= 0 {
		return nil, nil
	}
	limit = min(limit, deliveryRecoveryPageSize)
	cols := dao.TgCollectorDelivery.Columns()
	model := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(cols.Id, cols.Priority, cols.Status, "to_char(created_at,'YYYY-MM-DD HH24:MI:SS.US') AS "+deliveryRecoveryCreatedField).
		Where(cols.TenantId, scope.TenantID).Where("account_id", scope.AccountID).Where("source_id", scope.SourceID).
		Where("scheduler_pool", pool).
		Where(cols.Status+" IN ('pending','failed_retry')").
		Where("((status=? AND updated_at<=?) OR (status=? AND next_run_at IS NOT NULL AND next_run_at<=?))",
			sysin.DeliveryStatusPending, cursor.StaleBefore, sysin.DeliveryStatusFailedRetry, cursor.UntilTime).
		WhereLT(cols.AttemptCount, deliveryMaxAttempts).
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

func walkDeliveryRecoveryPage(rows gdb.Result, cursor deliveryRecoveryCursor, limit int, submit func(gdb.Record) error) (deliveryRecoveryCursor, error) {
	if limit <= 0 {
		return cursor, nil
	}
	cols := dao.TgCollectorDelivery.Columns()
	var resultErr error
	for _, row := range rows {
		id, createdAt := row[cols.Id].Int64(), row[deliveryRecoveryCreatedField].String()
		if id <= 0 || createdAt == "" {
			return cursor, gerror.New("交付恢复候选缺少有效分页键")
		}
		err := submit(row)
		if err != nil {
			if errors.Is(err, errCollectorRecoveryRedisUnavailable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return cursor, errors.Join(resultErr, err)
			}
			// This is only a scan position, never a delivery ACK. Ordinary
			// failures leave the durable row pending and return an error, but may
			// not pin all later rows. The next fixed-window sweep retries them.
			resultErr = errors.Join(resultErr, err)
		}
		cursor.AfterID, cursor.AfterCreatedAt, cursor.AfterPriority = id, createdAt, row[cols.Priority].Int()
	}
	if len(rows) < min(limit, deliveryRecoveryPageSize) {
		// A completed fixed window is revisited on the next pass, so a lost
		// queue task is recoverable without resubmitting the same head forever.
		cursor = deliveryRecoveryCursor{}
	}
	return cursor, resultErr
}

func (s *sCollector) recoverDeliveryCursorPage(ctx context.Context, scope collectorRecoveryScope, pool string, limit int, upper deliveryRecoveryCursor) (int, error) {
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
	rows, err := deliveryRecoveryPage(scanCtx, scope, pool, cursor, limit)
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

// Only identifiable Redis transport failures suspend other lanes. A generic
// context timeout or DB/route error is not evidence that Redis is unavailable.
func classifyDeliveryRecoveryEnqueueError(err error) error {
	if err == nil || errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "redis") {
		for _, marker := range []string{"connection refused", "connection reset by peer", "broken pipe", "i/o timeout", "client is closed"} {
			if strings.Contains(message, marker) {
				return errors.Join(errCollectorRecoveryRedisUnavailable, err)
			}
		}
	}
	if strings.Contains(message, "dial tcp") && strings.Contains(message, "redis") {
		return errors.Join(errCollectorRecoveryRedisUnavailable, err)
	}
	for _, marker := range []string{"redis: connection", "redis: client is closed", "redis is loading", "loading redis", "redis: i/o timeout", "redis: dial", "noauth authentication required", "wrongpass invalid username-password"} {
		if strings.Contains(message, marker) {
			return errors.Join(errCollectorRecoveryRedisUnavailable, err)
		}
	}
	return err
}
