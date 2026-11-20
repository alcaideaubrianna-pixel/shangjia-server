package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

type eventHandoffMode string

const (
	eventHandoffDirect        eventHandoffMode = "direct"
	eventHandoffQueued        eventHandoffMode = "queued"
	eventHandoffModeConfig                     = "telegramCollector.eventHandoff.mode"
	eventHandoffTimeoutConfig                  = "telegramCollector.eventHandoff.timeoutSeconds"
)

func parseEventHandoffMode(value string) (eventHandoffMode, error) {
	switch eventHandoffMode(strings.TrimSpace(value)) {
	case eventHandoffDirect:
		return eventHandoffDirect, nil
	case eventHandoffQueued:
		return eventHandoffQueued, nil
	default:
		return "", gerror.New("采集交付模式必须为 direct 或 queued")
	}
}

// History must retain handleDeliveryTask's account slots and queue budget.
func selectEventHandoffMode(mode eventHandoffMode, history bool, priority int) (eventHandoffMode, string) {
	if mode == eventHandoffDirect && (history || priority < sysin.EventPriorityRealtime) {
		return eventHandoffQueued, "history_isolated"
	}
	return mode, ""
}

// Dependencies are passed per call so tests do not replace shared handlers or
// database configuration. Both modes preserve the persisted Delivery identity.
type eventHandoffActions struct {
	save    func(context.Context, *sysin.CollectorDelivery) (int64, error)
	ready   func(context.Context) error
	enqueue func(context.Context, int64) error
	process func(context.Context, int64) error
	observe func(context.Context, int64, error)
}

// Eligibility is based on durable Event attempts and the execution lane, never
// optional tracing timestamps. Replay/recovery Event queues are history lanes.
func firstFreshDeliveryHandoff(attempt int, queue string, delivery *sysin.CollectorDelivery) bool {
	return attempt == 1 && delivery != nil && !delivery.History &&
		(queue == consts.QueueCollectFresh || queue == consts.QueueCollectRealtime) &&
		deliveryPriority(delivery) >= sysin.EventPriorityRealtime
}

// completeCollectorEvent preserves per-event ownership in both write paths.
func (s *sCollector) completeCollectorEvent(ctx context.Context, row *entity.TgCollectorEvent) error {
	mutation := historyEventMutation{Kind: historyEventReady, ID: row.Id, TenantID: row.TenantId, Owner: row.LeaseOwner, Attempt: row.AttemptCount}
	switch {
	case historyEventBatchEnabled(ctx):
		completed, err := s.historyEventWriter(ctx).Do(ctx, mutation)
		if err != nil {
			return err
		}
		if completed.Id != row.Id {
			return gerror.New("采集事件交接租约已变化，未确认本次交接")
		}
		return nil
	case realtimeEventReadyBatchEnabled(ctx):
		completed, err := s.realtimeEventReadyWriter().Do(ctx, mutation)
		if err != nil && ctx.Err() != nil {
			// A caller can time out while the independent batch is committing.
			checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 250*time.Millisecond)
			defer cancel()
			cols := dao.TgCollectorEvent.Columns()
			count, checkErr := dao.TgCollectorEvent.Ctx(checkCtx).WherePri(row.Id).
				Where(cols.TenantId, row.TenantId).Where(cols.AttemptCount, row.AttemptCount).
				Where(cols.Status, sysin.EventStatusReady).Count()
			if checkErr == nil && count == 1 {
				return nil
			}
		}
		if err != nil {
			return err
		}
		if completed.Id != row.Id {
			return gerror.New("采集事件交接租约已变化，未确认本次交接")
		}
		return nil
	}
	cols := dao.TgCollectorEvent.Columns()
	result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(row.Id).
		Where(cols.TenantId, row.TenantId).
		Where(cols.Status, sysin.EventStatusProcessing).
		Where(cols.LeaseOwner, row.LeaseOwner).
		Where(cols.AttemptCount, row.AttemptCount).
		Data(g.Map{cols.Status: sysin.EventStatusReady, cols.ProcessedAt: gtime.Now(),
			cols.LeaseOwner: "", cols.LeaseUntil: nil, cols.NextRunAt: nil,
			cols.ErrorMessage: "", cols.UpdatedAt: gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存采集事件交接状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认采集事件交接状态失败")
	}
	if affected != 1 {
		return gerror.New("采集事件交接租约已变化，未确认本次交接")
	}
	return nil
}

// Before ready, errors belong to Event. After ready, only Delivery may retry.
func (s *sCollector) handoffCollectorEvent(ctx context.Context, row *entity.TgCollectorEvent, delivery *sysin.CollectorDelivery) error {
	if row == nil || row.Id <= 0 || delivery == nil || delivery.EventID != row.Id || delivery.TenantID != row.TenantId {
		return gerror.New("采集事件交付归属无效")
	}
	mode, err := parseEventHandoffMode(g.Cfg().MustGet(ctx, eventHandoffModeConfig, string(eventHandoffDirect)).String())
	if err != nil {
		return err
	}
	seconds := g.Cfg().MustGet(ctx, eventHandoffTimeoutConfig, 10).Int()
	if seconds < 1 || seconds > 30 {
		return gerror.New("采集内联交付超时必须为1到30秒")
	}
	mode, fallbackReason := selectEventHandoffMode(mode, delivery.History, deliveryPriority(delivery))
	if _, sourceRuntime := collectorSourceLaneFromContext(ctx); sourceRuntime {
		mode, fallbackReason = eventHandoffQueued, "source_queue_handoff"
	}
	started := time.Now()
	return executeEventHandoff(ctx, mode, time.Duration(seconds)*time.Second, delivery, eventHandoffActions{
		save: s.saveDelivery,
		ready: func(ctx context.Context) (err error) {
			// Measure only the existing event-ready write and ownership check,
			// not the full handoff or downstream delivery handler.
			ctx, observation := startCollectorStage(ctx, collectorStageEventComplete, collectorObservationMeta{
				TenantID: row.TenantId, SourceID: row.SourceId, EventID: row.Id,
				DeliveryID: delivery.ID, SourceType: collectorObserveSourceType(row.SourceType),
				QueueClass: collectorObserveQueueClassFromPriority(deliveryPriority(delivery)),
			})
			outcome := collectorResultError
			defer func() { observation.Finish(outcome, err) }()
			if err := s.completeCollectorEvent(ctx, row); err != nil {
				return err
			}
			outcome = collectorResultOK
			observation.Finish(outcome, nil)
			logCollectorPipelineStage(ctx, row, "event.ready", started, delivery, nil, g.Map{"handoffMode": mode, "fallbackReason": fallbackReason, "retryOwner": "delivery"})
			return nil
		},
		enqueue: func(ctx context.Context, id int64) error {
			if firstFreshSourceHandoff(ctx, row.AttemptCount, collectorQueueName(ctx), delivery) {
				return s.enqueueDeliveryTaskToQueue(ctx, id, deliveryPriority(delivery), true, consts.QueueDeliveryFresh)
			}
			return s.enqueueDeliveryTask(ctx, id, deliveryPriority(delivery))
		},
		process: func(ctx context.Context, id int64) error { return s.processDeliveryWithEvent(ctx, id, row) },
		observe: func(ctx context.Context, _ int64, processErr error) {
			// A nil return also includes claim.skipped. Completion is reported
			// only by processDelivery's existing lifecycle logs.
			logCollectorPipelineStage(ctx, row, "handoff.returned", started, delivery, processErr,
				g.Map{"handoffMode": mode, "handlerReturnedError": processErr != nil, "retryOwner": "delivery"})
		},
	})
}

func executeEventHandoff(ctx context.Context, mode eventHandoffMode, timeout time.Duration, delivery *sysin.CollectorDelivery, actions eventHandoffActions) error {
	if mode != eventHandoffDirect && mode != eventHandoffQueued {
		return gerror.New("采集交付模式无效")
	}
	if delivery == nil {
		return gerror.New("采集交付不能为空")
	}
	mode, _ = selectEventHandoffMode(mode, delivery.History, deliveryPriority(delivery))
	if timeout <= 0 {
		return gerror.New("采集交付超时无效")
	}
	id, err := actions.save(ctx, delivery)
	if err != nil {
		return err
	}
	if id <= 0 {
		return gerror.New("持久化采集交付ID无效")
	}
	delivery.ID = id
	if mode == eventHandoffQueued {
		// Compatibility: enqueue must succeed before Event becomes ready.
		if err := actions.enqueue(ctx, id); err != nil {
			return err
		}
		return actions.ready(ctx)
	}
	if err := actions.ready(ctx); err != nil {
		return err
	}
	// From here no handler error or panic may consume the Event retry budget.
	// Existing Delivery leases/recovery own unfinished work, including timeouts.
	inlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var processErr error
	defer func() { actions.observe(ctx, id, processErr) }()
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				processErr = fmt.Errorf("采集内联交付 panic: %v", recovered)
			}
		}()
		processErr = actions.process(inlineCtx, id)
	}()
	return nil
}
