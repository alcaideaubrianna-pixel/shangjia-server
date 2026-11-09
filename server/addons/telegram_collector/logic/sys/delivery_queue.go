package sys

import (
	"context"
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
	legacyServer := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{Concurrency: 2, Queues: map[string]int{consts.QueueDeliveryReady: 1, consts.QueueDeliveryUrgent: 1}})
	s.deliveryLegacyServer = legacyServer
	s.deliveryServer = server
	s.deliveryHistoryServer = historyServer

	mux := asynq.NewServeMux()
	mux.HandleFunc(deliveryTaskType, s.handleDeliveryTask)
	g.Log().Infof(runtimeCtx, "启动Telegram Delivery Worker concurrency:%d", collectorDeliveryConcurrency(runtimeCtx))
	// The application owns signal handling and calls Shutdown in StopRuntime.
	// Run waits for an OS signal even after Shutdown and would block runtimeWG.
	for _, worker := range []*asynq.Server{legacyServer, historyServer, server} {
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
		return nil
	}
	return err
}

func (s *sCollector) handleDeliveryTask(ctx context.Context, task *asynq.Task) error {
	deliveryID, err := parseCollectorTaskID(task.Payload(), "交付")
	if err != nil {
		return err
	}
	queue, _ := asynq.GetQueueName(ctx)
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
		counter, _ := collectorMeter.Int64Counter("telegram_collector_delivery_total")
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "failed")))
		return err
	}
	histogram, _ := collectorMeter.Float64Histogram("telegram_collector_delivery_duration_seconds")
	histogram.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(attribute.String("result", "completed")))
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
		return nil
	}
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
	if err = handler.HandleCollectorDelivery(ctx, &delivery); err != nil {
		return s.failDelivery(ctx, deliveryID, err)
	}
	_, err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{
		columns.Status:       sysin.DeliveryStatusCompleted,
		columns.NextRunAt:    nil,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: "",
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	return err
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
	if err := s.resetExpiredDeliveryLeases(ctx, limit); err != nil {
		g.Log().Warningf(ctx, "恢复超时Telegram采集交付失败：%+v", err)
	}
	if err := s.enqueueDueDeliveryTasks(ctx, limit); err != nil {
		g.Log().Warningf(ctx, "重新投递Telegram采集交付失败：%+v", err)
	}
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
	// Keep a bounded recent-history lane ahead of the old backlog. Manual
	// history imports otherwise wait behind every older account delivery even
	// though they are already ready to process. The window is intentionally
	// finite so a continuous stream of new imports cannot starve old work.
	recentHistoryWindow := collectorDeliveryRecentHistoryWindow(ctx)
	recentHistoryCutoff := now.Add(-recentHistoryWindow)
	// Order 只接受完整表达式；不能把 CASE 的参数传给 Order，否则会被 PostgreSQL 当作列名拼入 SQL。
	orderExpression := fmt.Sprintf(
		"%s DESC, CASE WHEN %s = %d AND %s >= '%s' THEN 0 ELSE 1 END, %s ASC",
		columns.Priority, columns.Priority, sysin.EventPriorityNormal, columns.CreatedAt,
		recentHistoryCutoff.Format("2006-01-02 15:04:05.999999"), columns.UpdatedAt,
	)
	rows, err := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(columns.Id, columns.Priority, columns.Status).
		Where(condition,
			sysin.DeliveryStatusPending, staleBefore,
			sysin.DeliveryStatusFailedRetry, now,
		).
		WhereLT(columns.AttemptCount, deliveryMaxAttempts).
		Order(gdb.Raw(orderExpression)).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复Telegram采集交付失败")
	}
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
		unique := priority < sysin.EventPriorityUrgent
		if updateErr := s.enqueueDeliveryTaskWithOptions(ctx, deliveryID, priority, unique); updateErr != nil {
			return gerror.Wrap(updateErr, "投递待恢复Telegram采集交付失败")
		}
	}
	return nil
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
		return 0, gerror.Wrap(err, "读取交付优先级失败")
	}
	err = dao.TgCollectorEvent.Ctx(ctx).Where(ecols.Id+" = (?)", dao.TgCollectorDelivery.Ctx(ctx).Fields(dcols.EventId).Where(dcols.Id, deliveryID)).Scan(&event)
	if err != nil {
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

func deliveryQueueName(ctx context.Context, deliveryID int64, priority int) string {
	if priority >= sysin.EventPriorityUrgent {
		if accountID := deliveryAccountID(ctx, deliveryID); accountID > 0 {
			return consts.DeliveryUrgentShardQueue(accountID)
		}
		return consts.QueueDeliveryUrgentLive
	}
	if priority >= sysin.EventPriorityRealtime {
		if accountID := deliveryAccountID(ctx, deliveryID); accountID > 0 {
			return consts.DeliveryRealtimeShardQueue(accountID)
		}
		return consts.QueueDeliveryRealtime
	}
	accountID := deliveryAccountID(ctx, deliveryID)
	if accountID > 0 {
		return consts.DeliveryHistoryShardQueue(accountID)
	}
	return consts.QueueDeliveryHistory
}

func deliveryHistoryQueues() map[string]int {
	queues := map[string]int{consts.QueueDeliveryHistory: 1}
	for accountID := int64(0); accountID < consts.DeliveryHistoryShardCount; accountID++ {
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryHistoryShardPrefix, accountID)] = 1
	}
	return queues
}

func deliveryRealtimeQueues() map[string]int {
	queues := map[string]int{consts.QueueDeliveryUrgentLive: 8, consts.QueueDeliveryRealtime: 2}
	for accountID := int64(0); accountID < consts.DeliveryHistoryShardCount; accountID++ {
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryUrgentShardPrefix, accountID)] = 2
		queues[fmt.Sprintf("%s%02d", consts.QueueDeliveryRealtimeShardPrefix, accountID)] = 1
	}
	return queues
}

func deliveryAccountID(ctx context.Context, deliveryID int64) int64 {
	if deliveryID <= 0 {
		return 0
	}
	var accountID int64
	_ = dao.TgCollectorDelivery.Ctx(ctx).
		WherePri(deliveryID).
		Fields("e.account_id").
		LeftJoin("hg_tg_collector_event e", "e.id=hg_tg_collector_delivery.event_id").
		Scan(&accountID)
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
