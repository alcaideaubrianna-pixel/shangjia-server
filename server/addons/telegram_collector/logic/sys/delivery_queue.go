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
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/internal/library/collectobserve"
	"hotgo/utility/sourcequeue"
)

var errDeliveryInvalidEvent = errors.New("采集交付原始事件不可用")

type deliveryNotDueError struct {
	at time.Time
}

func (e *deliveryNotDueError) Error() string { return "采集交付尚未到重试时间" }

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
		return
	}
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.deliveryServer != nil {
		return
	}
	runtimeCtx := s.ensureRuntimeContext(ctx)
	q, err := sourcequeue.New(collectorDeliverySourceNamespace)
	if err != nil {
		g.Log().Warning(runtimeCtx, "初始化来源交付队列失败", g.Map{"error": err.Error()})
		return
	}
	historyWorkers, legacyWorkers := deliverySourceHistoryBudgets(collectorDeliveryHistoryConcurrency(runtimeCtx))
	// StopRuntime owns this marker. Legacy history readers share the history
	// budget; no new task is ever routed to Asynq.
	server := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{Concurrency: 1, Queues: deliveryHistoryQueues()})
	s.deliveryServer = server
	if legacyWorkers > 0 {
		mux := asynq.NewServeMux()
		mux.HandleFunc(deliveryTaskType, s.migrateLegacyDeliveryTask)
		if err := server.Start(mux); err != nil {
			g.Log().Warning(runtimeCtx, "启动历史交付兼容Worker失败", g.Map{"error": err.Error()})
		}
	}
	s.startDeliverySourcePools(runtimeCtx, q, collectorDeliveryConcurrency(runtimeCtx), historyWorkers)
	s.runtimeWG.Add(1)
	go func() { defer s.runtimeWG.Done(); s.runDeliveryRecovery(runtimeCtx) }()
	g.Log().Info(runtimeCtx, "Telegram来源交付Worker已启动", g.Map{
		"event": "collect_delivery_worker_started", "queueAdapter": "sourcequeue",
		"realtimeConcurrency": collectorDeliveryConcurrency(runtimeCtx),
		"freshEnabled":        deliveryFreshEnabled(),
		"historyConcurrency":  historyWorkers, "legacyConcurrency": legacyWorkers,
	})
}

func (s *sCollector) enqueueDeliveryTask(ctx context.Context, deliveryID int64, priority int) error {
	return s.enqueueDeliveryTaskWithOptions(ctx, deliveryID, priority, true)
}

// enqueueDeliveryTaskWithOptions preserves the replay API. Source queue IDs
// coalesce until ACK; the existing DB claim still fences business execution.
func (s *sCollector) enqueueDeliveryTaskWithOptions(ctx context.Context, deliveryID int64, priority int, unique bool) error {
	return s.enqueueDeliveryTaskToQueue(ctx, deliveryID, priority, unique, "")
}

// All producers use source-isolated lanes. Only a verified first realtime
// handoff may request fresh placement; recovery never upgrades existing work.
func (s *sCollector) enqueueDeliveryTaskToQueue(ctx context.Context, deliveryID int64, priority int, unique bool, explicitQueue string) (err error) {
	ctx, observation := startCollectorStage(ctx, collectorStageDeliveryEnqueue, collectorObservationMeta{DeliveryID: deliveryID})
	observeResult := collectorResultError
	defer func() { observation.Finish(observeResult, err) }()
	observation.meta.QueueClass = collectorObserveQueueClassFromPriority(priority)
	// History/recovery cannot become fresh merely because the persisted priority is high.
	if explicitQueue == consts.QueueDeliveryFresh && deliveryFreshEnabled() {
		if recovery, _ := ctx.Value(deliverySourceRecoveryKey{}).(bool); !recovery {
			if lane := deliverySourceLane(ctx, priority); lane != sourcequeue.History && priority >= sysin.EventPriorityRealtime {
				ctx = withCollectorSourceLane(ctx, sourcequeue.Fresh)
			}
		}
	}
	// Source queue IDs coalesce until ACK, including replay and recovery.
	err = s.enqueueDeliverySourceTask(ctx, deliveryID, priority)
	if err == nil {
		observeResult = collectorResultOK
	}
	return err
}

func (s *sCollector) handleDeliveryTask(ctx context.Context, task *asynq.Task) (err error) {
	taskReceivedAt := time.Now()
	ctx, deliveryID, decodeErr := collectobserve.DecodeTask(ctx, task.Payload(), taskReceivedAt)
	_, parseObservation := startCollectorStage(ctx, collectorStageDeliveryTaskParse, collectorObservationMeta{})
	defer parseObservation.Finish(collectorResultError, nil)
	err = decodeErr
	if err != nil {
		err = gerror.Wrap(err, "Telegram采集交付任务ID无效")
	}
	parseObservation.meta.DeliveryID = deliveryID
	parseObservation.Finish(collectorResultOK, err)
	if err != nil {
		return err
	}
	ctx, _ = collectorObservationContext(ctx, parseObservation.meta)
	wait := startCollectorQueueWaitFromTiming(ctx, collectorStageDeliveryConsume, parseObservation.meta)
	defer wait.Finish(parseObservation.meta)
	ctx = collectorQueueWaitContext(ctx, wait)
	taskStarted := taskReceivedAt
	prepareParent := ctx
	ctx, prepareObservation := startCollectorStage(ctx, collectorStageDeliveryPrepare, collectorObservationMeta{DeliveryID: deliveryID})
	prepareResult := collectorResultError
	defer func() { prepareObservation.Finish(prepareResult, err) }()
	queue := collectorQueueName(ctx)
	logCollectorDeliveryStage(ctx, "delivery.process.started", deliveryID, 0, queue, nil,
		g.Map{"taskStartedAtUnixMs": taskStarted.UnixMilli()})
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
	retryCount, _ := asynq.GetRetryCount(ctx)
	if deliveryTaskNeedsReclassification(queue, retryCount) {
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
		// Fresh retries must confirm a new enqueue rather than trusting an
		// expired/completed task receipt. DB claims still fence duplicates.
		if queue == consts.QueueDeliveryFresh {
			err = s.enqueueDeliveryTaskWithOptions(ctx, deliveryID, priority, false)
		} else {
			err = s.enqueueDeliveryTask(ctx, deliveryID, priority)
		}
		prepareObservation.Finish(collectorResultAccepted, err)
		return err
	}
	if lane, limited := deliveryAccountLaneForQueue(queue); limited {
		accountID := deliveryAccountID(ctx, deliveryID)
		// Lane permits only partition the existing Worker budget. A failed
		// fresh task is rerouted above before it can acquire fresh permits.
		releaseAccountSlot, acquired := s.deliveryAccountSlots.acquire(ctx, lane, accountID, collectorDeliveryHistoryAccountConcurrency(ctx))
		if !acquired {
			return newDeliveryAccountBusyError()
		}
		defer releaseAccountSlot()
	}
	prepareObservation.Finish(collectorResultOK, nil)
	ctx = prepareParent
	startedAt := time.Now()
	if err = s.processDelivery(ctx, deliveryID, taskStarted); err != nil {
		logCollectorDeliveryStage(ctx, "delivery.process.failed", deliveryID, 0, queue, err,
			g.Map{"taskDurationMs": time.Since(taskStarted).Milliseconds()})
		counter, _ := collectorMeter.Int64Counter("telegram_collector_delivery_total")
		counter.Add(ctx, 1, metric.WithAttributes(append(collectorCollectionAttributes(ctx), attribute.String("result", "failed"), attribute.String("queue", collectorQueueName(ctx)))...))
		return err
	}
	histogram, _ := collectorMeter.Float64Histogram("telegram_collector_delivery_duration_seconds")
	histogram.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(append(collectorCollectionAttributes(ctx), attribute.String("result", "completed"), attribute.String("queue", collectorQueueName(ctx)))...))
	logCollectorDeliveryStage(ctx, "delivery.process.completed", deliveryID, 0, queue, nil,
		g.Map{"taskDurationMs": time.Since(taskStarted).Milliseconds()})
	return nil
}

type deliveryAccountBusyError struct{}

func (e *deliveryAccountBusyError) Error() string { return "采集交付账户并发额度已满" }

func newDeliveryAccountBusyError() error { return &deliveryAccountBusyError{} }

func (s *sCollector) processDelivery(ctx context.Context, deliveryID int64, taskStarts ...time.Time) error {
	return s.processDeliveryWithEvent(ctx, deliveryID, nil, taskStarts...)
}

// Inline Event handoff already owns a validated raw event; queued attempts load it.
func (s *sCollector) processDeliveryWithEvent(ctx context.Context, deliveryID int64, loadedEvent *entity.TgCollectorEvent, taskStarts ...time.Time) (err error) {
	if deliveryID <= 0 {
		return gerror.New("Telegram采集交付任务无效")
	}
	ctx, observation := startCollectorStage(ctx, collectorStageDeliveryConsume, collectorObservationMeta{
		DeliveryID: deliveryID, QueueClass: collectorObserveQueueClassFromClass(""),
	})
	observeResult := collectorResultError
	defer func() { observation.Finish(observeResult, err) }()
	taskStarted := time.Now()
	if len(taskStarts) > 0 {
		taskStarted = taskStarts[0]
	}
	defer func() {
		if wait := collectorQueueWaitFromContext(ctx); wait != nil && wait.stage == collectorStageDeliveryConsume {
			wait.Finish(observation.meta)
		}
	}()
	// Source claims reuse these exact rows below; scope validation happens
	// before taking a business lease or invoking any handler.
	var sourceRow *entity.TgCollectorDelivery
	if scope, ok := ctx.Value(deliverySourceClaimKey{}).(sourcequeue.Scope); ok {
		var row entity.TgCollectorDelivery
		if err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Scan(&row); err != nil {
			return gerror.Wrap(err, "读取来源交付记录失败")
		}
		var event entity.TgCollectorEvent
		if loadedEvent != nil && loadedEvent.Id == row.EventId {
			event = *loadedEvent
		} else if err := dao.TgCollectorEvent.Ctx(ctx).WherePri(row.EventId).Scan(&event); err != nil {
			return gerror.Wrap(err, "读取来源交付事件失败")
		}
		if err := validateDeliverySourceScope(scope, &row, &event); err != nil {
			return err
		}
		sourceRow, loadedEvent = &row, &event
	}
	claimStarted := time.Now()
	logCollectorDeliveryStage(ctx, "delivery.claim.started", deliveryID, 0, collectorQueueName(ctx), nil,
		g.Map{"taskStartedAtUnixMs": taskStarted.UnixMilli(), "taskDurationMs": time.Since(taskStarted).Milliseconds()})
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	executionOwner := guid.S()
	claim := dao.TgCollectorDelivery.Ctx(ctx).
		WherePri(deliveryID).
		WhereIn(columns.Status, []string{sysin.DeliveryStatusPending, sysin.DeliveryStatusFailedRetry})
	if sourceRow != nil {
		claim = claim.Where("("+columns.NextRunAt+" IS NULL OR "+columns.NextRunAt+" <= ?)", now)
	}
	result, err := claim.Data(g.Map{
		columns.Status:       sysin.DeliveryStatusProcessing,
		columns.LeaseOwner:   executionOwner,
		columns.LeaseUntil:   now.Add(deliveryLeaseDuration),
		columns.AttemptCount: gdb.Raw(columns.AttemptCount + "+1"),
		columns.UpdatedAt:    now,
	}).Update()
	claimDuration := time.Since(claimStarted).Milliseconds()
	if err != nil {
		recordCollectorStage(ctx, collectorStageDeliveryClaim, observation.meta, collectorResultError, time.Since(claimStarted), err)
		logCollectorDeliveryStage(ctx, "delivery.claim.failed", deliveryID, 0, collectorQueueName(ctx), err,
			g.Map{"claimDurationMs": claimDuration, "taskDurationMs": time.Since(taskStarted).Milliseconds()})
		return gerror.Wrap(err, "领取Telegram采集交付失败")
	}
	affected, affectedErr := result.RowsAffected()
	if affectedErr != nil {
		recordCollectorStage(ctx, collectorStageDeliveryClaim, observation.meta, collectorResultError, time.Since(claimStarted), affectedErr)
		return gerror.Wrap(affectedErr, "确认Telegram采集交付领取结果失败")
	}
	if affected == 0 {
		recordCollectorStage(ctx, collectorStageDeliveryClaim, observation.meta, collectorResultSkipped, time.Since(claimStarted), nil)
		observeResult = collectorResultSkipped
		logCollectorDeliveryStage(ctx, "delivery.claim.skipped", deliveryID, 0, collectorQueueName(ctx), nil,
			g.Map{"claimDurationMs": claimDuration, "taskDurationMs": time.Since(taskStarted).Milliseconds()})
		if sourceRow != nil {
			var current entity.TgCollectorDelivery
			if err := dao.TgCollectorDelivery.Ctx(ctx).Fields(columns.Status, columns.NextRunAt).
				WherePri(deliveryID).Scan(&current); err != nil {
				return gerror.Wrap(err, "复查Telegram采集交付重试时间失败")
			}
			if (current.Status == sysin.DeliveryStatusPending || current.Status == sysin.DeliveryStatusFailedRetry) &&
				current.NextRunAt != nil {
				return &deliveryNotDueError{at: current.NextRunAt.Time}
			}
		}
		return nil
	}
	recordCollectorStage(ctx, collectorStageDeliveryClaim, observation.meta, collectorResultOK, time.Since(claimStarted), nil)
	logCollectorDeliveryStage(ctx, "delivery.claimed", deliveryID, 0, collectorQueueName(ctx), nil,
		g.Map{"claimDurationMs": claimDuration, "taskDurationMs": time.Since(taskStarted).Milliseconds()})
	readStarted := time.Now()
	var row entity.TgCollectorDelivery
	if sourceRow != nil {
		row = *sourceRow
		row.Status, row.LeaseOwner = sysin.DeliveryStatusProcessing, executionOwner
		row.AttemptCount++
	} else {
		err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Where(columns.Status, sysin.DeliveryStatusProcessing).Where(columns.LeaseOwner, executionOwner).Scan(&row)
	}
	if err != nil {
		recordCollectorStage(ctx, collectorStageDeliveryRead, observation.meta, collectorResultError, time.Since(readStarted), err)
		logCollectorDeliveryStage(ctx, "delivery.load.failed", deliveryID, 0, collectorQueueName(ctx), err,
			g.Map{"readDurationMs": time.Since(readStarted).Milliseconds(), "taskDurationMs": time.Since(taskStarted).Milliseconds()})
		return s.failDelivery(ctx, deliveryID, executionOwner, err)
	}
	if row.LeaseOwner != executionOwner || row.Status != sysin.DeliveryStatusProcessing {
		err = gerror.New("采集执行归属已变化，停止旧执行")
		recordCollectorStage(ctx, collectorStageDeliveryRead, observation.meta, collectorResultError, time.Since(readStarted), err)
		return err
	}
	observation.meta.TenantID, observation.meta.EventID = row.TenantId, row.EventId
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	recordCollectorStage(ctx, collectorStageDeliveryRead, observation.meta, collectorResultOK, time.Since(readStarted), nil)
	readDetails := g.Map{"readDurationMs": time.Since(readStarted).Milliseconds(),
		"taskDurationMs": time.Since(taskStarted).Milliseconds(), "tenantId": row.TenantId, "attemptCount": row.AttemptCount}
	readDetails["ageTimeBasis"] = "db_wall_clock_unverified"
	readDetails["deliveryAgeAvailable"] = false
	if row.CreatedAt != nil {
		readDetails["deliveryCreatedAtRaw"] = row.CreatedAt.Time.Format("2006-01-02 15:04:05.999999")
	}
	logCollectorDeliveryStage(ctx, "delivery.loaded", deliveryID, row.EventId, collectorQueueName(ctx), nil, readDetails)
	if row.EventId <= 0 {
		return s.failDelivery(ctx, deliveryID, executionOwner, gerror.New("Telegram采集交付缺少原始事件"))
	}
	var event entity.TgCollectorEvent
	eventReadStarted := time.Now()
	if loadedEvent != nil && loadedEvent.Id == row.EventId && loadedEvent.TenantId == row.TenantId {
		event = *loadedEvent
	} else {
		err = dao.TgCollectorEvent.Ctx(ctx).WherePri(row.EventId).Scan(&event)
	}
	if err != nil {
		recordCollectorStage(ctx, collectorStageDeliveryEventRead, observation.meta, collectorResultError, time.Since(eventReadStarted), err)
		logCollectorDeliveryStage(ctx, "delivery.event.load.failed", deliveryID, row.EventId, collectorQueueName(ctx), err,
			g.Map{"eventReadDurationMs": time.Since(eventReadStarted).Milliseconds(), "taskDurationMs": time.Since(taskStarted).Milliseconds()})
		return s.failDelivery(ctx, deliveryID, executionOwner, gerror.Wrap(err, "读取Telegram采集原始事件失败"))
	}
	eventReadDuration := time.Since(eventReadStarted).Milliseconds()
	observation.meta.SourceID = event.SourceId
	if event.SourceType == sysin.SourceTypeAccount {
		observation.meta.TgAccountID = event.AccountId
	}
	observation.meta.ChatID, observation.meta.MessageID = event.ChatId, event.MessageId
	observation.meta.SourceType = collectorObserveSourceType(event.SourceType)
	observation.meta.QueueClass = collectorObserveQueueClassFromPriority(event.Priority)
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	if event.Id <= 0 || event.RawUpdate == nil {
		cause := gerror.New("Telegram采集原始事件不存在")
		recordCollectorStage(ctx, collectorStageDeliveryEventRead, observation.meta, collectorResultError, time.Since(eventReadStarted), cause)
		return s.failDelivery(ctx, deliveryID, executionOwner, cause)
	}
	if identityErr := validateCollectorDeliveryEvent(&row, &event, []byte(event.RawUpdate.String())); identityErr != nil {
		logCollectorDeliveryStage(ctx, "delivery.identity_conflict", deliveryID, row.EventId, collectorQueueName(ctx), identityErr,
			g.Map{"tenantId": row.TenantId, "sourceId": event.SourceId, "sourceMessageId": event.MessageId})
		if recoveryErr := s.recoverConflictingDelivery(ctx, &row, &event); recoveryErr != nil {
			return s.failDelivery(ctx, deliveryID, executionOwner, recoveryErr)
		}
		observeResult = collectorResultOK
		return nil
	}
	recordCollectorStage(ctx, collectorStageDeliveryEventRead, observation.meta, collectorResultOK, time.Since(eventReadStarted), nil)
	parseStarted := time.Now()
	ctx = collectobserve.RestorePayloadClock(ctx, []byte(event.RawUpdate.String()), time.Now())
	observation.meta.ReceivedAt = collectobserve.From(ctx).ReceivedAt
	delivery, err := collectorDeliveryFromEvent(&event, []byte(event.RawUpdate.String()))
	if err == nil {
		observation.meta.Mode = collectorModeFromRaw(event.SourceType, []byte(event.RawUpdate.String()))
		observation.meta.AccountID, observation.meta.TgAccountID = delivery.AccountID, delivery.TgAccountID
		ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	}
	recordCollectorStage(ctx, collectorStageDeliveryParse, observation.meta, collectorResultOK, time.Since(parseStarted), err)
	if err != nil {
		return s.failDelivery(ctx, deliveryID, executionOwner, err)
	}
	// History is persisted on the Delivery because raw Bot updates have no lane.
	// Legacy rows with schema-default history but realtime priority stay live.
	if row.Priority < sysin.EventPriorityRealtime {
		delivery.History = true
		observation.meta.Mode = collectorModeHistory
	}
	observation.meta.QueueClass = collectorObserveQueueClassFromPriority(deliveryPriority(&delivery))
	ctx, observation.meta = collectorObservationContext(ctx, observation.meta)
	delivery.ID = deliveryID
	delivery.DeliveryKey = row.DeliveryKey
	handler := collectorservice.CollectorDeliveryHandler()
	if handler == nil {
		return s.failDelivery(ctx, deliveryID, executionOwner, gerror.New("Telegram采集交付处理器尚未注册"))
	}
	queueName := collectorQueueName(ctx)
	handlerStarted := time.Now()
	logCollectorDeliveryStage(ctx, "delivery.handler.started", deliveryID, row.EventId, queueName, nil,
		g.Map{"eventReadDurationMs": eventReadDuration, "taskDurationMs": time.Since(taskStarted).Milliseconds(),
			"handlerStartedAtUnixMs": handlerStarted.UnixMilli(), "tenantId": event.TenantId, "accountId": event.AccountId,
			"sourceId": event.SourceId, "sourceChatId": event.ChatId, "sourceMessageId": event.MessageId, "history": delivery.History})
	handlerCtx, handlerObservation := startCollectorStage(ctx, collectorStageDeliveryHandler, observation.meta)
	defer handlerObservation.Finish(collectorResultError, nil)
	err = handler.HandleCollectorDelivery(handlerCtx, &delivery)
	handlerObservation.Finish(collectorResultOK, err)
	if err != nil {
		logCollectorDeliveryStage(ctx, "delivery.handler.failed", deliveryID, row.EventId, queueName, err,
			g.Map{"handlerDurationMs": time.Since(handlerStarted).Milliseconds(), "taskDurationMs": time.Since(taskStarted).Milliseconds()})
		return s.failDelivery(ctx, deliveryID, executionOwner, err)
	}
	logCollectorDeliveryStage(ctx, "delivery.handler.completed", deliveryID, row.EventId, queueName, nil,
		g.Map{"handlerDurationMs": time.Since(handlerStarted).Milliseconds(), "taskDurationMs": time.Since(taskStarted).Milliseconds()})
	completeStarted := time.Now()
	completeResult, completeErr := dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).
		Where(columns.Status, sysin.DeliveryStatusProcessing).
		Where(columns.LeaseOwner, executionOwner).Data(g.Map{
		columns.Status:       sysin.DeliveryStatusCompleted,
		columns.NextRunAt:    nil,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: "",
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	err = completeErr
	if err == nil {
		var affected int64
		affected, err = completeResult.RowsAffected()
		if err != nil {
			err = gerror.Wrap(err, "确认Telegram采集交付完成结果失败")
		} else if affected != 1 {
			err = gerror.New("采集交付执行归属已变化，未确认本次完成")
		}
	}
	recordCollectorStage(ctx, collectorStageDeliveryComplete, observation.meta, collectorResultOK, time.Since(completeStarted), err)
	if err != nil {
		logCollectorDeliveryStage(ctx, "delivery.completed.persist_failed", deliveryID, row.EventId, collectorQueueName(ctx), err)
		return err
	}
	logCollectorDeliveryStage(ctx, "delivery.completed", deliveryID, row.EventId, collectorQueueName(ctx), nil)
	observeResult = collectorResultOK
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
		"observedAtUnixMs": time.Now().UnixMilli(),
	}
	collectorObservationLogFields(ctx, fields)
	if err != nil {
		fields["errorType"] = fmt.Sprintf("%T", err)
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "Telegram采集交付入队", fields)
		return
	}
	g.Log().Info(ctx, "Telegram采集交付入队", fields)
}

func logCollectorDeliveryStage(ctx context.Context, stage string, deliveryID, eventID int64, queue string, err error, details ...g.Map) {
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
		"observedAtUnixMs": time.Now().UnixMilli(),
	}
	collectorObservationLogFields(ctx, fields)
	for _, detail := range details {
		for key, value := range detail {
			fields[key] = value
		}
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

func (s *sCollector) failDelivery(ctx context.Context, deliveryID int64, executionOwner string, cause error) error {
	if strings.TrimSpace(executionOwner) == "" {
		return gerror.New("采集失败更新必须指定本次执行归属")
	}
	columns := dao.TgCollectorDelivery.Columns()
	var row entity.TgCollectorDelivery
	if err := dao.TgCollectorDelivery.Ctx(ctx).Fields(columns.AttemptCount, columns.LeaseOwner).WherePri(deliveryID).
		Where(columns.Status, sysin.DeliveryStatusProcessing).Where(columns.LeaseOwner, executionOwner).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取Telegram采集交付重试次数失败")
	}
	if row.LeaseOwner != executionOwner {
		return gerror.New("采集执行归属已变化，未写入失败状态")
	}
	status := sysin.DeliveryStatusFailedRetry
	var nextRunAt any = gtime.Now().Add(30 * time.Second)
	if row.AttemptCount >= deliveryMaxAttempts {
		status = sysin.DeliveryStatusDead
		nextRunAt = nil
	}
	result, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).
		Where(columns.Status, sysin.DeliveryStatusProcessing).
		Where(columns.LeaseOwner, executionOwner).Data(g.Map{
		columns.Status:       status,
		columns.NextRunAt:    nextRunAt,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: cause.Error(),
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存采集失败状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认采集失败状态更新结果失败")
	}
	if affected != 1 {
		return gerror.New("采集执行归属已变化，未确认失败或终止状态")
	}
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
	scopes, err := collectorRecoveryScopes(ctx, collectorRecoveryDeliveryLease, 8)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		laneCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.resetExpiredDeliveryLeasesForScope(laneCtx, min(16, limit), scope)
		cancel()
		if err != nil {
			g.Log().Warning(ctx, "来源交付租约恢复失败", g.Map{"scope": scope.key(), "error": err.Error()})
		}
	}
	return nil
}

func (s *sCollector) resetExpiredDeliveryLeasesForScope(ctx context.Context, limit int, scope collectorRecoveryScope) error {
	if limit <= 0 || !scope.valid() {
		return nil
	}

	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	rows, err := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(columns.Id, columns.AttemptCount).
		Where(columns.TenantId, scope.TenantID).Where("account_id", scope.AccountID).Where("source_id", scope.SourceID).
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

func (s *sCollector) enqueueDueDeliveryTasks(ctx context.Context, limit int) (recoveryErr error) {
	ctx = context.WithValue(ctx, deliverySourceRecoveryKey{}, true)
	meta := collectorObservationMeta{QueueClass: collectorQueueMixed}
	ctx, recoveryObservation := startCollectorStage(ctx, collectorStageDeliveryRecovery, meta)
	defer func() { recoveryObservation.Finish(collectorResultOK, recoveryErr) }()
	if limit <= 0 {
		return nil
	}
	scopeCtx, cancelScopes := context.WithTimeout(ctx, time.Second)
	scopes, err := collectorRecoveryScopes(scopeCtx, collectorRecoveryDeliveryEnqueue, 8)
	cancelScopes()
	if err != nil {
		return gerror.Wrap(err, "读取交付恢复来源失败")
	}
	if len(scopes) == 0 {
		return nil
	}
	// One scalar clock read per pass, not per empty scope/pool. Cursor strings
	// retain DB microseconds in the business timezone.
	clockCtx, cancelClock := context.WithTimeout(ctx, time.Second)
	upper, err := newDeliveryRecoveryCursor(clockCtx)
	cancelClock()
	if err != nil {
		return err
	}
	pools := collectorRecoveryBranchOrder([]string{"realtime", "history"})
	// Reserve one slot per selected scope/pool even with a tiny configured
	// batch; earlier lanes cannot spend the later lanes' quota.
	remaining := max(2*len(scopes), min(limit, 200))
	laneQuota := max(1, min(deliveryRecoveryPageSize, remaining/(2*len(scopes))))
	for _, scope := range scopes {
		for _, pool := range pools {
			if err := ctx.Err(); err != nil {
				return errors.Join(recoveryErr, err)
			}
			if remaining <= 0 {
				return recoveryErr
			}
			laneCtx, cancelLane := context.WithTimeout(ctx, time.Second)
			count, err := s.recoverDeliveryBoundedCursorPage(laneCtx, scope, pool, min(laneQuota, remaining), upper)
			cancelLane()
			remaining -= count
			if errors.Is(err, errCollectorRecoveryRedisUnavailable) {
				return errors.Join(recoveryErr, err)
			}
			if err != nil {
				recoveryErr = errors.Join(recoveryErr, err)
				g.Log().Warning(ctx, "来源交付恢复失败", g.Map{"scope": scope.key(), "pool": pool, "error": err.Error()})
			}
		}
	}
	return recoveryErr
}

func collectorDeliveryConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.concurrency", 8).Int()
	if value < 1 {
		return 1
	}
	return value
}

// Partition the existing realtime budget; these configs never add capacity.
// The first server drains old realtime work, the second reserves fresh work.
func deliveryRealtimeServerConfigs(total int) []asynq.Config {
	if total < 1 {
		total = 1
	}
	legacy := deliveryRealtimeQueues()
	if total == 1 {
		legacy[consts.QueueDeliveryFresh] = 8
		return []asynq.Config{{Concurrency: 1, Queues: legacy}}
	}
	backlog := max(1, total/4)
	return []asynq.Config{
		{Concurrency: backlog, Queues: legacy},
		{Concurrency: total - backlog, Queues: map[string]int{consts.QueueDeliveryFresh: 1}},
	}
}

func deliveryTaskNeedsReclassification(queue string, retryCount int) bool {
	return queue == consts.QueueDeliveryReady || (queue == consts.QueueDeliveryFresh && retryCount > 0)
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
	// Explicit history priority must not be promoted by raw Bot type on recovery.
	if storedPriority.Int() < sysin.EventPriorityRealtime {
		return storedPriority.Int(), nil
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
	if value < 0 {
		value = 0
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
