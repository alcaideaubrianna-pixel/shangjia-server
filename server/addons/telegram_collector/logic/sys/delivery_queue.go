package sys

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

var errDeliveryInvalidEvent = errors.New("采集交付原始事件不可用")

const deliveryTaskType = "telegram_collector:delivery"

const (
	deliveryLeaseDuration     = 3 * time.Minute
	deliveryRecoveryInterval  = 30 * time.Second
	deliveryRecoveryStaleTime = 15 * time.Second
	deliveryTaskUniqueTTL     = 5 * time.Minute
	deliveryMaxAttempts       = 5
)

func (s *sCollector) StartDeliveryRuntime(ctx context.Context) {
	if !collectorEnabled(ctx) {
		g.Log().Info(ctx, "Telegram采集插件处于关闭状态，Publish Worker不会消费采集交付")
		return
	}
	s.queueMu.Lock()
	if s.deliveryServer != nil {
		s.queueMu.Unlock()
		return
	}
	runtimeCtx := s.ensureRuntimeContext(ctx)
	server := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{
		Concurrency: collectorDeliveryConcurrency(runtimeCtx),
		Queues:      deliveryRealtimeQueues(),
	})
	if s.queueClient == nil {
		s.queueClient = asynq.NewClient(collectorRedisOption(runtimeCtx))
	}
	historyServer := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{
		Concurrency: collectorDeliveryHistoryConcurrency(runtimeCtx),
		Queues:      deliveryHistoryQueues(),
	})
	s.deliveryServer = server
	s.deliveryHistoryServer = historyServer

	mux := asynq.NewServeMux()
	mux.HandleFunc(deliveryTaskType, s.handleDeliveryTask)
	g.Log().Info(runtimeCtx, "Telegram采集交付Worker已启动", g.Map{
		"log_type":            "telegram_collect_delivery",
		"event":               "collect_delivery_worker_started",
		"workerRole":          "collector-delivery-worker",
		"workerInstanceId":    collectorInstanceID(),
		"realtimeConcurrency": collectorDeliveryConcurrency(runtimeCtx),
		"historyConcurrency":  collectorDeliveryHistoryConcurrency(runtimeCtx),
		"realtimeQueueCount":  len(deliveryRealtimeQueues()),
		"historyQueueCount":   len(deliveryHistoryQueues()),
	})
	// The application owns signal handling and calls Shutdown in StopRuntime.
	// Run waits for an OS signal even after Shutdown and would block runtimeWG.
	for _, worker := range []*asynq.Server{historyServer, server} {
		if err := worker.Start(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Warningf(runtimeCtx, "启动Telegram交付Worker失败：%+v", err)
		}
	}
	s.runtimeWG.Add(1)
	go func() {
		defer s.runtimeWG.Done()
		s.runDeliveryRecovery(runtimeCtx)
	}()
	s.queueMu.Unlock()
}

func (s *sCollector) enqueueDeliveryTask(ctx context.Context, deliveryID int64, priority int) error {
	return s.enqueueDeliveryTaskWithOptions(ctx, deliveryID, priority, true)
}

// enqueueDeliveryTaskWithOptions allows an explicitly requested replay to
// bypass Asynq's old unique task marker. A delivery is still claimed with a
// status transition in processDelivery, so the duplicate task is harmless and
// lets a replay move work from a stale queue to its current priority queue.
func (s *sCollector) enqueueDeliveryTaskWithOptions(ctx context.Context, deliveryID int64, priority int, unique bool) error {
	if deliveryID <= 0 {
		return gerror.New("Telegram采集交付任务ID无效")
	}
	s.queueMu.Lock()
	if s.queueClient == nil {
		s.queueClient = asynq.NewClient(collectorRedisOption(ctx))
	}
	client := s.queueClient
	s.queueMu.Unlock()
	queueName := deliveryQueueName(ctx, deliveryID, priority)
	options := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(deliveryMaxAttempts - 1),
		asynq.Timeout(3 * time.Minute),
	}
	if unique {
		options = append(options, asynq.Unique(deliveryTaskUniqueTTL))
	}
	_, err := client.EnqueueContext(ctx, asynq.NewTask(deliveryTaskType, collectorTaskPayload(deliveryID)), options...)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		logCollectorDeliveryEnqueue(ctx, deliveryID, queueName, "duplicate", err)
		if !unique {
			return gerror.Wrap(err, "恢复Telegram采集交付时任务已存在，未确认本次入队")
		}
		return nil
	}
	if err != nil {
		logCollectorDeliveryEnqueue(ctx, deliveryID, queueName, "failed", err)
		return err
	}
	logCollectorDeliveryEnqueue(ctx, deliveryID, queueName, "enqueued", nil)
	return nil
}

func (s *sCollector) handleDeliveryTask(ctx context.Context, task *asynq.Task) error {
	deliveryID, err := parseCollectorTaskID(task.Payload(), "交付")
	if err != nil {
		return err
	}
	queue := collectorQueueName(ctx)
	logCollectorDeliveryStage(ctx, "delivery.process.started", deliveryID, 0, queue, nil)
	defer func() {
		if recovered := recover(); recovered != nil {
			logCollectorDeliveryStage(ctx, "delivery.process.panic", deliveryID, 0, queue, fmt.Errorf("panic: %v", recovered))
			panic(recovered)
		}
	}()
	if queue == consts.QueueDeliveryHistory {
		timer := time.NewTimer(collectorDeliveryHistoryInterval(ctx))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if queue == consts.QueueDeliveryHistory || strings.HasPrefix(queue, consts.QueueDeliveryHistoryShardPrefix) || strings.HasPrefix(queue, consts.QueueDeliveryRealtimeShardPrefix) || strings.HasPrefix(queue, consts.QueueDeliveryUrgentShardPrefix) {
		accountID := deliveryAccountID(ctx, deliveryID)
		releaseAccountSlot, acquired := s.acquireHistoryAccountSlot(ctx, accountID)
		if !acquired {
			return newDeliveryAccountBusyError()
		}
		defer releaseAccountSlot()
	}
	if queue == consts.QueueDeliveryReady {
		priority, err := s.persistedDeliveryPriority(ctx, deliveryID)
		if err != nil {
			if errors.Is(err, errDeliveryInvalidEvent) {
				if quarantineErr := s.quarantineInvalidDelivery(ctx, deliveryID, err); quarantineErr != nil {
					return quarantineErr
				}
				return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
			}
			return err
		}
		// ACK legacy work only after the classified task is durably enqueued.
		return s.enqueueDeliveryTask(ctx, deliveryID, priority)
	}
	startedAt := time.Now()
	if err = s.processDelivery(ctx, deliveryID); err != nil {
		logCollectorDeliveryStage(ctx, "delivery.process.failed", deliveryID, 0, queue, err)
		counter, _ := collectorMeter.Int64Counter("telegram_collector_delivery_total")
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "failed"), attribute.String("queue", collectorQueueName(ctx))))
		return err
	}
	histogram, _ := collectorMeter.Float64Histogram("telegram_collector_delivery_duration_seconds")
	histogram.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(attribute.String("result", "completed"), attribute.String("queue", collectorQueueName(ctx))))
	logCollectorDeliveryStage(ctx, "delivery.process.completed", deliveryID, 0, queue, nil)
	return nil
}

type deliveryAccountBusyError struct{}

func (e *deliveryAccountBusyError) Error() string { return "历史采集账户并发额度已满" }

func newDeliveryAccountBusyError() error { return &deliveryAccountBusyError{} }

func (s *sCollector) acquireHistoryAccountSlot(ctx context.Context, accountID int64) (func(), bool) {
	if accountID <= 0 {
		return func() {}, true
	}
	limit := collectorDeliveryHistoryAccountConcurrency(ctx)
	s.historyAccountMu.Lock()
	if s.historyAccountSlots == nil {
		s.historyAccountSlots = make(map[int64]chan struct{})
	}
	slots := s.historyAccountSlots[accountID]
	if slots == nil || cap(slots) != limit {
		slots = make(chan struct{}, limit)
		s.historyAccountSlots[accountID] = slots
	}
	s.historyAccountMu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	case <-ctx.Done():
		return func() {}, false
	default:
		return func() {}, false
	}
}

func (s *sCollector) processDelivery(ctx context.Context, deliveryID int64) error {
	if deliveryID <= 0 {
		return gerror.New("Telegram采集交付任务无效")
	}
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	result, err := dao.TgCollectorDelivery.Ctx(ctx).
		WherePri(deliveryID).
		WhereIn(columns.Status, []string{sysin.DeliveryStatusPending, sysin.DeliveryStatusFailedRetry}).
		Data(g.Map{
			columns.Status:       sysin.DeliveryStatusProcessing,
			columns.LeaseOwner:   collectorInstanceID(),
			columns.LeaseUntil:   now.Add(deliveryLeaseDuration),
			columns.AttemptCount: gdb.Raw(columns.AttemptCount + "+1"),
			columns.UpdatedAt:    now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "领取Telegram采集交付失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		logCollectorDeliveryStage(ctx, "delivery.claim.skipped", deliveryID, 0, collectorQueueName(ctx), nil)
		return nil
	}
	logCollectorDeliveryStage(ctx, "delivery.claimed", deliveryID, 0, collectorQueueName(ctx), nil)
	var row entity.TgCollectorDelivery
	if err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Scan(&row); err != nil {
		return s.failDelivery(ctx, deliveryID, err)
	}
	if row.EventId <= 0 {
		return s.failDelivery(ctx, deliveryID, gerror.New("Telegram采集交付缺少原始事件"))
	}
	var event entity.TgCollectorEvent
	if err = dao.TgCollectorEvent.Ctx(ctx).WherePri(row.EventId).Scan(&event); err != nil {
		return s.failDelivery(ctx, deliveryID, gerror.Wrap(err, "读取Telegram采集原始事件失败"))
	}
	if event.Id <= 0 || event.RawUpdate == nil {
		return s.failDelivery(ctx, deliveryID, gerror.New("Telegram采集原始事件不存在"))
	}
	delivery, err := collectorDeliveryFromEvent(&event, []byte(event.RawUpdate.String()))
	if err != nil {
		return s.failDelivery(ctx, deliveryID, err)
	}
	class := "realtime"
	if delivery.History {
		class = "history"
	}
	if row.CreatedAt != nil {
		wait, _ := collectorMeter.Float64Histogram("telegram_collector_delivery_queue_wait_seconds")
		wait.Record(ctx, time.Since(row.CreatedAt.Time).Seconds(), metric.WithAttributes(attribute.String("class", class)))
	}
	delivery.ID = deliveryID
	delivery.DeliveryKey = row.DeliveryKey
	handler := collectorservice.CollectorDeliveryHandler()
	if handler == nil {
		return s.failDelivery(ctx, deliveryID, gerror.New("Telegram采集交付处理器尚未注册"))
	}
	queueName := collectorQueueName(ctx)
	logCollectorDeliveryStage(ctx, "delivery.handler.started", deliveryID, row.EventId, queueName, nil)
	err = handler.HandleCollectorDelivery(ctx, &delivery)
	if err != nil {
		logCollectorDeliveryStage(ctx, "delivery.handler.failed", deliveryID, row.EventId, queueName, err)
		return s.failDelivery(ctx, deliveryID, err)
	}
	logCollectorDeliveryStage(ctx, "delivery.handler.completed", deliveryID, row.EventId, queueName, nil)
	_, err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{
		columns.Status:       sysin.DeliveryStatusCompleted,
		columns.NextRunAt:    nil,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: "",
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	if err != nil {
		logCollectorDeliveryStage(ctx, "delivery.completed.persist_failed", deliveryID, row.EventId, collectorQueueName(ctx), err)
		return err
	}
	logCollectorDeliveryStage(ctx, "delivery.completed", deliveryID, row.EventId, collectorQueueName(ctx), nil)
	return nil
}

func logCollectorSubmitResult(ctx context.Context, delivery *sysin.CollectorDelivery, queue string, result sysin.CollectSubmitResult) {
	if delivery == nil {
		return
	}
	g.Log().Info(ctx, "Telegram采集提交结果", g.Map{
		"log_type":          "telegram_collect_delivery",
		"event":             "collect_submit_result",
		"outcome":           result.Outcome,
		"stage":             result.Stage,
		"reason":            result.Reason,
		"detail":            result.Detail,
		"retryable":         result.Retryable,
		"deliveryId":        delivery.ID,
		"collectEventId":    result.CollectEventID,
		"profileId":         result.ProfileID,
		"dispatchId":        result.DispatchID,
		"jobId":             result.JobID,
		"queue":             queue,
		"ownership":         "collector",
		"telegramExecution": "out_of_scope",
	})
}

func logCollectorDeliveryEnqueue(ctx context.Context, deliveryID int64, queue, status string, err error) {
	fields := g.Map{
		"log_type":         "telegram_collect_delivery",
		"event":            "collect_delivery_enqueue",
		"stage":            "delivery.enqueued",
		"status":           status,
		"deliveryId":       deliveryID,
		"queue":            queue,
		"workerInstanceId": collectorInstanceID(),
		"workerRole":       "collector-worker",
	}
	if err != nil {
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "Telegram采集交付入队", fields)
		return
	}
	g.Log().Info(ctx, "Telegram采集交付入队", fields)
}

func logCollectorDeliveryStage(ctx context.Context, stage string, deliveryID, eventID int64, queue string, err error) {
	fields := g.Map{
		"log_type":         "telegram_collect_delivery",
		"event":            "collect_delivery_stage",
		"stage":            stage,
		"status":           "ok",
		"deliveryId":       deliveryID,
		"collectEventId":   eventID,
		"eventId":          eventID,
		"queue":            queue,
		"workerInstanceId": collectorInstanceID(),
		"workerRole":       "collector-delivery-worker",
	}
	if err != nil {
		fields["status"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "Telegram采集交付链路阶段", fields)
		return
	}
	g.Log().Info(ctx, "Telegram采集交付链路阶段", fields)
}

func (s *sCollector) failDelivery(ctx context.Context, deliveryID int64, cause error) error {
	columns := dao.TgCollectorDelivery.Columns()
	var row entity.TgCollectorDelivery
	if err := dao.TgCollectorDelivery.Ctx(ctx).Fields(columns.AttemptCount).WherePri(deliveryID).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取Telegram采集交付重试次数失败")
	}
	status := sysin.DeliveryStatusFailedRetry
	var nextRunAt any = gtime.Now().Add(30 * time.Second)
	if row.AttemptCount >= deliveryMaxAttempts {
		status = sysin.DeliveryStatusDead
		nextRunAt = nil
	}
	_, _ = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{
		columns.Status:       status,
		columns.NextRunAt:    nextRunAt,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: cause.Error(),
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	if status == sysin.DeliveryStatusDead {
		return fmt.Errorf("%w: Telegram采集交付超过最大重试次数: %v", asynq.SkipRetry, cause)
	}
	return cause
}

func (s *sCollector) runDeliveryRecovery(ctx context.Context) {
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	s.recoverDeliveryTasks(ctx)
	ticker := time.NewTicker(deliveryRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverDeliveryTasks(ctx)
		}
	}
}

func (s *sCollector) recoverDeliveryTasks(ctx context.Context) {
	limit := collectorRecoveryBatchSize(ctx)
	if limit <= 0 {
		return
	}
	fields := g.Map{
		"log_type":   "telegram_collect_delivery",
		"event":      "collect_delivery_recovery",
		"stage":      "recovery.started",
		"batchLimit": limit,
		"workerRole": "collector-delivery-worker",
	}
	g.Log().Debug(ctx, "Telegram采集交付恢复开始", fields)
	if err := s.resetExpiredDeliveryLeases(ctx, limit); err != nil {
		fields["stage"] = "recovery.lease_reset_failed"
		fields["status"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "恢复超时Telegram采集交付失败，下一轮将重试", fields)
	}
	if err := s.enqueueDueDeliveryTasks(ctx, limit); err != nil {
		fields["stage"] = "recovery.enqueue_failed"
		fields["status"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "重新投递Telegram采集交付失败，下一轮将重试", fields)
		return
	}
	fields["stage"] = "recovery.completed"
	fields["status"] = "ok"
	g.Log().Debug(ctx, "Telegram采集交付恢复完成", fields)
}

func (s *sCollector) resetExpiredDeliveryLeases(ctx context.Context, limit int) error {
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	rows, err := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(columns.Id, columns.AttemptCount).
		Where(columns.Status, sysin.DeliveryStatusProcessing).
		WhereNotNull(columns.LeaseUntil).
		WhereLTE(columns.LeaseUntil, now).
		OrderAsc(columns.LeaseUntil).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取超时Telegram采集交付失败")
	}
	for _, row := range rows {
		deliveryID := row[columns.Id].Int64()
		if deliveryID <= 0 {
			continue
		}
		status := sysin.DeliveryStatusFailedRetry
		var nextRunAt any = now
		if row[columns.AttemptCount].Int() >= deliveryMaxAttempts {
			status = sysin.DeliveryStatusDead
			nextRunAt = nil
		}
		_, updateErr := dao.TgCollectorDelivery.Ctx(ctx).
			WherePri(deliveryID).
			Where(columns.Status, sysin.DeliveryStatusProcessing).
			WhereLTE(columns.LeaseUntil, now).
			Data(g.Map{
				columns.Status:       status,
				columns.NextRunAt:    nextRunAt,
				columns.LeaseOwner:   "",
				columns.LeaseUntil:   nil,
				columns.ErrorMessage: "处理租约超时，已自动恢复",
				columns.UpdatedAt:    now,
			}).Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "重置超时Telegram采集交付失败")
		}
	}
	return nil
}

func (s *sCollector) enqueueDueDeliveryTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		return nil
	}
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	staleBefore := now.Add(-deliveryRecoveryStaleTime)
	condition := fmt.Sprintf(
		"(%s=? AND %s<=?) OR (%s=? AND %s IS NOT NULL AND %s<=?)",
		columns.Status,
		columns.UpdatedAt,
		columns.Status,
		columns.NextRunAt,
		columns.NextRunAt,
	)
	// Query each scheduler lane independently. Mixing realtime and history with
	// OR/CASE forces PostgreSQL to scan both pools and prevents the partial lane
	// indexes from being used. A simple deterministic order keeps each query
	// bounded and index-friendly.
	queryRows := func(poolCondition string, batchLimit int, poolArgs ...any) (gdb.Result, error) {
		if batchLimit <= 0 {
			return nil, nil
		}
		return dao.TgCollectorDelivery.Ctx(ctx).
			Fields(columns.Id, columns.Priority, columns.Status).
			Where(condition,
				sysin.DeliveryStatusPending, staleBefore,
				sysin.DeliveryStatusFailedRetry, now,
			).
			WhereLT(columns.AttemptCount, deliveryMaxAttempts).
			Where(poolCondition, poolArgs...).
			Order(columns.Priority + " DESC, " + columns.CreatedAt + " ASC, " + columns.Id + " ASC").
			Limit(batchLimit).
			All()
	}

	// Realtime recovery is isolated from history. A plain global ORDER BY lets
	// one busy source occupy the whole recovery batch and can also starve an
	// old source behind a continuously arriving source. Cap the number of rows
	// selected per source in each pass so every source gets a recovery slot.
	const liveRecoverySourceQuota = 16
	liveRows, err := s.queryFairRealtimeDeliveryRows(ctx, staleBefore, now, limit, liveRecoverySourceQuota)
	if err != nil {
		return gerror.Wrap(err, "读取实时Telegram采集交付失败")
	}
	legacyRows, err := queryRows(
		fmt.Sprintf("scheduler_pool = ? AND %s >= ?", columns.Priority),
		limit-len(liveRows),
		"history", sysin.EventPriorityRealtime,
	)
	if err != nil {
		return gerror.Wrap(err, "读取兼容实时Telegram采集交付失败")
	}
	historyRows, err := queryRows(
		fmt.Sprintf("scheduler_pool = ? AND %s < ?", columns.Priority),
		limit-len(liveRows)-len(legacyRows),
		"history", sysin.EventPriorityRealtime,
	)
	if err != nil {
		return gerror.Wrap(err, "读取历史Telegram采集交付失败")
	}
	rows := append(liveRows, legacyRows...)
	rows = append(rows, historyRows...)

	for _, row := range rows {
		deliveryID := row[columns.Id].Int64()
		status := row[columns.Status].String()
		if deliveryID <= 0 || status == "" {
			continue
		}
		priority, priorityErr := s.persistedDeliveryPriority(ctx, deliveryID)
		if priorityErr != nil {
			if !errors.Is(priorityErr, errDeliveryInvalidEvent) {
				return priorityErr
			}
			if err := s.quarantineInvalidDelivery(ctx, deliveryID, priorityErr); err != nil {
				return err
			}
			g.Log().Warningf(ctx, "隔离无法分类的采集交付，继续恢复后续任务 deliveryId:%d err:%+v", deliveryID, priorityErr)
			continue
		}
		// An urgent task may have an old unique marker in another queue. Allow
		// recovery to create a current-priority copy; processDelivery's
		// conditional claim keeps duplicate callbacks idempotent.
		// Recovery must submit a fresh Asynq task. A stale Unique marker can make
		// EnqueueContext return ErrDuplicateTask even when the old task is no
		// longer present in the live queue, which falsely reports recovery success.
		if updateErr := s.enqueueDeliveryTaskWithOptions(ctx, deliveryID, priority, false); updateErr != nil {
			return gerror.Wrap(updateErr, "投递待恢复Telegram采集交付失败")
		}
	}
	return nil
}

// queryFairRealtimeDeliveryRows bounds recovery work per source. The query is
// deliberately limited to the realtime lane and returns only the columns used
// by enqueueDueDeliveryTasks; it prevents one source from monopolizing the
// global recovery batch while preserving a bounded DB read.
func (s *sCollector) queryFairRealtimeDeliveryRows(ctx context.Context, staleBefore, now *gtime.Time, limit, sourceQuota int) (gdb.Result, error) {
	if limit <= 0 || sourceQuota <= 0 {
		return nil, nil
	}
	sql := fmt.Sprintf(`
SELECT id, priority, status
FROM (
    SELECT id, priority, status, source_id, created_at,
           ROW_NUMBER() OVER (PARTITION BY source_id ORDER BY priority DESC, created_at ASC, id ASC) AS source_rank
    FROM %s
    WHERE ((status = ? AND updated_at <= ?) OR (status = ? AND next_run_at IS NOT NULL AND next_run_at <= ?))
      AND attempt_count < ?
      AND scheduler_pool = ?
) fair
WHERE source_rank <= ?
ORDER BY priority DESC, created_at ASC, id ASC
LIMIT ?`, dao.TgCollectorDelivery.Table())
	return g.DB().GetAll(ctx, sql,
		sysin.DeliveryStatusPending, staleBefore,
		sysin.DeliveryStatusFailedRetry, now,
		deliveryMaxAttempts, "realtime", sourceQuota, limit)
}

func collectorDeliveryConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.concurrency", 8).Int()
	if value < 1 {
		return 1
	}
	return value
}

// Resolve legacy priorities from the immutable event; recovery and first
// delivery use the same classification without rewriting existing jobs.
func (s *sCollector) persistedDeliveryPriority(ctx context.Context, deliveryID int64) (int, error) {
	dcols := dao.TgCollectorDelivery.Columns()
	ecols := dao.TgCollectorEvent.Columns()
	var event entity.TgCollectorEvent
	storedPriority, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Value(dcols.Priority)
	if err != nil {
		if isMissingDeliveryRowError(err) {
			return 0, gerror.Wrapf(errDeliveryInvalidEvent, "Telegram采集交付记录不存在 deliveryId:%d", deliveryID)
		}
		return 0, gerror.Wrap(err, "读取交付优先级失败")
	}
	err = dao.TgCollectorEvent.Ctx(ctx).Where(ecols.Id+" = (?)", dao.TgCollectorDelivery.Ctx(ctx).Fields(dcols.EventId).Where(dcols.Id, deliveryID)).Scan(&event)
	if err != nil {
		if isMissingDeliveryRowError(err) {
			return 0, gerror.Wrapf(errDeliveryInvalidEvent, "Telegram采集交付关联事件不存在 deliveryId:%d", deliveryID)
		}
		return 0, gerror.Wrap(err, "读取交付优先级失败")
	}
	if event.Id <= 0 || event.RawUpdate == nil {
		return 0, fmt.Errorf("%w: 交付缺少原始事件", errDeliveryInvalidEvent)
	}
	delivery, err := collectorDeliveryFromEvent(&event, []byte(event.RawUpdate.String()))
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errDeliveryInvalidEvent, err)
	}
	computed := deliveryPriority(&delivery)
	if storedPriority.Int() > computed {
		return storedPriority.Int(), nil
	}
	return computed, nil
}

func isMissingDeliveryRowError(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func deliveryQueueName(ctx context.Context, deliveryID int64, priority int) string {
	accountID, sourceID, schedulerPool := deliveryRouteScope(ctx, deliveryID)
	// New rows persist scheduler_pool. Legacy rows may still carry the schema
	// default (history); a realtime priority is the compatibility signal until
	// those rows are re-enqueued by the recovery path.
	if schedulerPool == "history" && priority < sysin.EventPriorityRealtime {
		if accountID > 0 {
			return consts.DeliveryHistoryShardQueue(accountID)
		}
		if sourceID > 0 {
			return consts.DeliveryHistorySourceQueue(sourceID)
		}
		return consts.QueueDeliveryHistory
	}
	if priority >= sysin.EventPriorityUrgent {
		if accountID > 0 {
			return consts.DeliveryUrgentShardQueue(accountID)
		}
		if sourceID > 0 {
			return consts.DeliveryLiveSourceQueue(sourceID)
		}
		return consts.QueueDeliveryUrgentLive
	}
	if priority >= sysin.EventPriorityRealtime {
		if accountID > 0 {
			return consts.DeliveryRealtimeShardQueue(accountID)
		}
		if sourceID > 0 {
			return consts.DeliveryLiveSourceQueue(sourceID)
		}
		return consts.QueueDeliveryRealtime
	}
	if accountID > 0 {
		return consts.DeliveryHistoryShardQueue(accountID)
	}
	if sourceID > 0 {
		return consts.DeliveryHistorySourceQueue(sourceID)
	}
	return consts.QueueDeliveryHistory
}

func deliveryHistoryQueues() map[string]int {
	queues := map[string]int{consts.QueueDeliveryHistory: 1}
	for shard := int64(0); shard < consts.DeliveryHistoryShardCount; shard++ {
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryHistoryShardPrefix, shard)] = 1
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryHistorySourceShardPrefix, shard)] = 1
	}
	return queues
}

func deliveryRealtimeQueues() map[string]int {
	queues := map[string]int{consts.QueueDeliveryUrgentLive: 8, consts.QueueDeliveryRealtime: 2}
	for shard := int64(0); shard < consts.DeliveryHistoryShardCount; shard++ {
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryUrgentShardPrefix, shard)] = 2
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryRealtimeShardPrefix, shard)] = 1
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryLiveSourceShardPrefix, shard)] = 2
	}
	return queues
}

func deliveryRouteScope(ctx context.Context, deliveryID int64) (accountID, sourceID int64, schedulerPool string) {
	if deliveryID <= 0 {
		return 0, 0, ""
	}
	row, err := dao.TgCollectorDelivery.Ctx(ctx).
		WherePri(deliveryID).
		Fields("account_id", "source_id", "scheduler_pool").
		One()
	if err != nil {
		return 0, 0, ""
	}
	return row["account_id"].Int64(), row["source_id"].Int64(), row["scheduler_pool"].String()
}

func deliveryAccountID(ctx context.Context, deliveryID int64) int64 {
	accountID, _, _ := deliveryRouteScope(ctx, deliveryID)
	return accountID
}

func collectorDeliveryHistoryInterval(ctx context.Context) time.Duration {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.historyIntervalMilliseconds", 200).Int()
	if value < 100 {
		value = 100
	}
	return time.Duration(value) * time.Millisecond
}

func collectorDeliveryHistoryConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.historyConcurrency", 4).Int()
	if value < 1 {
		value = 1
	}
	if value > 32 {
		value = 32
	}
	return value
}

func collectorDeliveryHistoryAccountConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.accountConcurrency", 2).Int()
	if value < 1 {
		return 1
	}
	if value > 16 {
		return 16
	}
	return value
}

func collectorDeliveryRecentHistoryWindow(ctx context.Context) time.Duration {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.recentHistoryWindowMinutes", 60).Int()
	if value < 1 {
		value = 1
	}
	if value > 24*60 {
		value = 24 * 60
	}
	return time.Duration(value) * time.Minute
}

// Quarantine only a deterministically malformed job by primary key. Keep its
// payload and business state for repair; transient database failures retry.
func (s *sCollector) quarantineInvalidDelivery(ctx context.Context, id int64, cause error) error {
	c := dao.TgCollectorDelivery.Columns()
	_, err := dao.TgCollectorDelivery.Ctx(ctx).Where(c.Id, id).WhereIn(c.Status, []string{sysin.DeliveryStatusPending, sysin.DeliveryStatusFailedRetry}).Data(g.Map{c.Status: sysin.DeliveryStatusQuarantined, c.ErrorMessage: cause.Error()}).Update()
	return err
}
