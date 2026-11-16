package sys

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// These are observation stages, not persisted task states.
type collectorObserveStage string
type collectorObserveResult string
type collectorObserveQueueClass string
type collectorObserveSourceType string

const (
	collectorStageIngestBot         collectorObserveStage      = "ingest_bot"
	collectorStageIngestAccount     collectorObserveStage      = "ingest_account"
	collectorStageEventPersist      collectorObserveStage      = "event_persist"
	collectorStageEventEnqueue      collectorObserveStage      = "event_enqueue"
	collectorStageEventConsume      collectorObserveStage      = "event_consume"
	collectorStageEventClaim        collectorObserveStage      = "event_claim"
	collectorStageEventRead         collectorObserveStage      = "event_read"
	collectorStageEventParse        collectorObserveStage      = "event_parse"
	collectorStageEventDispatch     collectorObserveStage      = "event_dispatch"
	collectorStageDeliveryEnqueue   collectorObserveStage      = "delivery_enqueue"
	collectorStageDeliveryConsume   collectorObserveStage      = "delivery_consume"
	collectorStageDeliveryClaim     collectorObserveStage      = "delivery_claim"
	collectorStageDeliveryRead      collectorObserveStage      = "delivery_read"
	collectorStageDeliveryEventRead collectorObserveStage      = "delivery_event_read"
	collectorStageDeliveryParse     collectorObserveStage      = "delivery_parse"
	collectorStageDeliveryComplete  collectorObserveStage      = "delivery_complete"
	collectorStageEventComplete     collectorObserveStage      = "event_complete"
	collectorStageDeliveryPrepare   collectorObserveStage      = "delivery_prepare"
	collectorStageDeliveryHandler   collectorObserveStage      = "delivery_handler"
	collectorStageEventRecovery     collectorObserveStage      = "event_recovery"
	collectorStageDeliveryRecovery  collectorObserveStage      = "delivery_recovery"
	collectorStageEventScan         collectorObserveStage      = "event_recovery_scan"
	collectorStageDeliveryScan      collectorObserveStage      = "delivery_recovery_scan"
	collectorStageEventReenqueue    collectorObserveStage      = "event_recovery_enqueue"
	collectorStageDeliveryReenqueue collectorObserveStage      = "delivery_recovery_enqueue"
	collectorResultAccepted         collectorObserveResult     = "accepted"
	collectorResultOK               collectorObserveResult     = "ok"
	collectorResultError            collectorObserveResult     = "error"
	collectorResultDuplicate        collectorObserveResult     = "duplicate"
	collectorResultSkipped          collectorObserveResult     = "skipped"
	collectorResultCanceled         collectorObserveResult     = "canceled"
	collectorQueueUrgent            collectorObserveQueueClass = "urgent"
	collectorQueueRealtime          collectorObserveQueueClass = "realtime"
	collectorQueueHistory           collectorObserveQueueClass = "history"
	collectorQueueMixed             collectorObserveQueueClass = "mixed"
	collectorSourceBot              collectorObserveSourceType = "bot"
	collectorSourceAccount          collectorObserveSourceType = "account"
)

// Identity fields belong exclusively to spans/logs, never metric labels.
// Leave unavailable values zero; observation must not fetch missing identities.
type collectorObservationMeta struct {
	QueueClass collectorObserveQueueClass
	SourceType collectorObserveSourceType
	TenantID   int64
	AccountID  int64
	SourceID   int64
	EventID    int64
	DeliveryID int64
	ChatID     string
	MessageID  int64
}

type collectorPipelineInstruments struct {
	operations metric.Int64Counter
	duration   metric.Float64Histogram
	candidates metric.Int64Counter
	queueWait  metric.Float64Histogram
	waitSample metric.Int64Counter
}

var (
	collectorPipelineOnce    sync.Once
	collectorPipelineMetrics collectorPipelineInstruments
	collectorPipelineTracer  = otel.Tracer("hotgo/addons/telegram_collector")
)

func newCollectorPipelineInstruments(meter metric.Meter) (collectorPipelineInstruments, error) {
	var instruments collectorPipelineInstruments
	operations, e1 := meter.Int64Counter("telegram_collector_pipeline_operations",
		metric.WithUnit("{operation}"), metric.WithDescription("Collector stage completions, including failed and skipped attempts"))
	duration, e2 := meter.Float64Histogram("telegram_collector_pipeline_duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Collector stage execution duration; not queue residence time"),
		metric.WithExplicitBucketBoundaries(.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120))
	candidates, e3 := meter.Int64Counter("telegram_collector_recovery_candidates",
		metric.WithUnit("{candidate}"), metric.WithDescription("Rows returned by existing recovery page queries; not unique tasks or backlog size"))
	queueWait, e4 := meter.Float64Histogram("telegram_collector_pipeline_delivery_queue_wait_seconds",
		metric.WithUnit("s"), metric.WithDescription("Trusted producer enqueue epoch to consumer receipt; includes schedule/retry delay"),
		metric.WithExplicitBucketBoundaries(.01, .05, .1, .5, 1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 21600, 86400))
	waitSample, e5 := meter.Int64Counter("telegram_collector_delivery_wait_samples",
		metric.WithUnit("{sample}"), metric.WithDescription("Queue wait sample availability: valid, missing or future"))
	instruments.operations, instruments.duration, instruments.candidates = operations, duration, candidates
	instruments.queueWait, instruments.waitSample = queueWait, waitSample
	return instruments, errors.Join(e1, e2, e3, e4, e5)
}

func collectorPipelineInstrumentsFor(ctx context.Context) collectorPipelineInstruments {
	collectorPipelineOnce.Do(func() {
		var err error
		collectorPipelineMetrics, err = newCollectorPipelineInstruments(collectorMeter)
		if err != nil {
			// Instrument registration failure must never fail collection.
			g.Log().Warning(ctx, "初始化采集链路指标失败", g.Map{"event": "collector.observability.init_failed", "errorType": "metric_registration"})
		}
	})
	return collectorPipelineMetrics
}

func collectorBoundedStage(value collectorObserveStage) string {
	switch value {
	case collectorStageIngestBot, collectorStageIngestAccount, collectorStageEventPersist,
		collectorStageEventEnqueue, collectorStageEventConsume, collectorStageEventClaim,
		collectorStageEventRead, collectorStageEventParse, collectorStageEventDispatch,
		collectorStageDeliveryEnqueue, collectorStageDeliveryPrepare, collectorStageDeliveryHandler,
		collectorStageDeliveryConsume, collectorStageDeliveryClaim, collectorStageDeliveryRead,
		collectorStageDeliveryEventRead, collectorStageDeliveryParse, collectorStageDeliveryComplete, collectorStageEventComplete,
		collectorStageEventRecovery, collectorStageDeliveryRecovery, collectorStageEventScan,
		collectorStageDeliveryScan, collectorStageEventReenqueue, collectorStageDeliveryReenqueue:
		return string(value)
	default:
		return "unknown"
	}
}

func collectorBoundedResult(value collectorObserveResult) string {
	switch value {
	case collectorResultAccepted, collectorResultOK, collectorResultError, collectorResultDuplicate, collectorResultSkipped, collectorResultCanceled:
		return string(value)
	default:
		return "unknown"
	}
}

// collectorObserveQueueClassFromClass converts collectorQueueClass(priority)
// or an already resolved lane. It never preserves raw queue names or IDs.
func collectorObserveQueueClassFromClass(value string) collectorObserveQueueClass {
	switch collectorObserveQueueClass(value) {
	case collectorQueueUrgent, collectorQueueRealtime, collectorQueueHistory, collectorQueueMixed:
		return collectorObserveQueueClass(value)
	default:
		return collectorObserveQueueClass("unknown")
	}
}

// collectorObserveQueueClassFromPriority preserves the urgent lane, unlike
// the legacy collectorQueueClass which intentionally groups it into realtime.
// Use persisted event priority before parsing; after parsing delivery, pass
// deliveryPriority(&delivery) so account History routing remains authoritative.
// Never infer a delivery lane from Asynq context in the direct handoff path.
func collectorObserveQueueClassFromPriority(priority int) collectorObserveQueueClass {
	if priority >= sysin.EventPriorityUrgent {
		return collectorQueueUrgent
	}
	if priority >= sysin.EventPriorityRealtime {
		return collectorQueueRealtime
	}
	return collectorQueueHistory
}

func (meta collectorObservationMeta) metricAttributes() []attribute.KeyValue {
	queue, source := "unknown", "unknown"
	switch meta.QueueClass {
	case collectorQueueUrgent, collectorQueueRealtime, collectorQueueHistory, collectorQueueMixed:
		queue = string(meta.QueueClass)
	}
	switch meta.SourceType {
	case collectorSourceBot, collectorSourceAccount:
		source = string(meta.SourceType)
	}
	return []attribute.KeyValue{attribute.String("queue_class", queue), attribute.String("source_type", source)}
}

func (meta collectorObservationMeta) traceAttributes() []attribute.KeyValue {
	attrs := meta.metricAttributes()
	for _, item := range []struct {
		key string
		id  int64
	}{
		{"tenant_id", meta.TenantID}, {"account_id", meta.AccountID}, {"source_id", meta.SourceID},
		{"event_id", meta.EventID}, {"delivery_id", meta.DeliveryID}, {"message_id", meta.MessageID},
	} {
		if item.id != 0 {
			attrs = append(attrs, attribute.String(item.key, strconv.FormatInt(item.id, 10)))
		}
	}
	if meta.ChatID != "" {
		attrs = append(attrs, attribute.String("chat_id", meta.ChatID))
	}
	return attrs
}

type collectorStageObservation struct {
	ctx     context.Context
	span    trace.Span
	started time.Time
	stage   collectorObserveStage
	meta    collectorObservationMeta
	once    sync.Once
}

// startCollectorStage starts one operation. Finish exactly where its outcome is
// known, before wrapping/returning errors. Keep the returned ctx on downstream calls.
func startCollectorStage(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta) (context.Context, *collectorStageObservation) {
	attrs := append(meta.traceAttributes(), attribute.String("stage", collectorBoundedStage(stage)))
	ctx, span := collectorPipelineTracer.Start(ctx, "collector."+collectorBoundedStage(stage), trace.WithAttributes(attrs...))
	return ctx, &collectorStageObservation{ctx: ctx, span: span, started: time.Now(), stage: stage, meta: meta}
}

func endCollectorStage(observation *collectorStageObservation, result collectorObserveResult, err error) {
	observation.Finish(result, err)
}

func collectorObservationResult(result collectorObserveResult, err error) collectorObserveResult {
	if err == nil {
		return result
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return collectorResultCanceled
	}
	return collectorResultError
}

// recordCollectorStage is the metric-only hook for existing structured log
// helpers. Pass that stage's local elapsed duration, not the whole pipeline age.
// Do not also start/end the same stage: that would double count. This function
// neither creates nor ends spans, and performs no logging or external queries.
func recordCollectorStage(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, result collectorObserveResult, elapsed time.Duration, err error) {
	result = collectorObservationResult(result, err)
	instruments := collectorPipelineInstrumentsFor(ctx)
	attrs := append(meta.metricAttributes(),
		attribute.String("stage", collectorBoundedStage(stage)),
		attribute.String("result", collectorBoundedResult(result)))
	options := metric.WithAttributes(attrs...)
	if instruments.operations != nil {
		instruments.operations.Add(ctx, 1, options)
	}
	// Negative elapsed means unavailable; still count the outcome, but don't
	// manufacture a zero or a negative latency sample.
	if elapsed >= 0 && instruments.duration != nil {
		instruments.duration.Record(ctx, elapsed.Seconds(), options)
	}
}

// Finish is idempotent so an explicit outcome and a deferred safety finish
// cannot count the same stage twice. Unknown outcomes remain unknown, not ok.
func (observation *collectorStageObservation) Finish(result collectorObserveResult, err error) {
	if observation == nil {
		return
	}
	observation.once.Do(func() {
		result = collectorObservationResult(result, err)
		elapsed := time.Since(observation.started)
		recordCollectorStage(observation.ctx, observation.stage, observation.meta, result, elapsed, nil)
		observation.span.SetAttributes(observation.meta.traceAttributes()...)
		observation.span.SetAttributes(attribute.String("result", collectorBoundedResult(result)))
		failed := result == collectorResultError || result == collectorResultCanceled
		// Do not record raw error text: Telegram errors may contain private
		// message content or signed URLs. Existing business logs own details.
		if failed {
			observation.span.SetStatus(codes.Error, collectorBoundedResult(result))
		}
		if failed || elapsed >= 100*time.Millisecond {
			fields := g.Map{"event": "collector.stage.completed", "stage": collectorBoundedStage(observation.stage),
				"result": collectorBoundedResult(result), "durationMs": float64(elapsed.Microseconds()) / 1000,
				"tenantId": observation.meta.TenantID, "accountId": observation.meta.AccountID,
				"sourceId": observation.meta.SourceID, "eventId": observation.meta.EventID,
				"deliveryId": observation.meta.DeliveryID, "chatId": observation.meta.ChatID,
				"messageId": observation.meta.MessageID}
			for _, attr := range observation.meta.metricAttributes() {
				fields[string(attr.Key)] = attr.Value.AsString()
			}
			fields["traceID"] = observation.span.SpanContext().TraceID().String()
			if failed {
				g.Log().Warning(observation.ctx, "采集链路阶段失败", fields)
			} else {
				g.Log().Info(observation.ctx, "采集链路阶段耗时", fields)
			}
		}
		observation.span.End()
	})
}

// observeCollectorRecoveryCandidates counts rows already fetched by a recovery
// page. Call once per successful query, including empty pages. No extra query.
func observeCollectorRecoveryCandidates(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, count int) {
	if count <= 0 || (stage != collectorStageEventScan && stage != collectorStageDeliveryScan) {
		return
	}
	instruments := collectorPipelineInstrumentsFor(ctx)
	if instruments.candidates != nil {
		attrs := append(meta.metricAttributes(), attribute.String("stage", collectorBoundedStage(stage)))
		instruments.candidates.Add(ctx, int64(count), metric.WithAttributes(attrs...))
	}
}

func collectorDeliveryWaitSample(producerEnqueuedUnixMS int64, receivedAt time.Time) (time.Duration, string) {
	if producerEnqueuedUnixMS <= 0 || receivedAt.IsZero() {
		return 0, "missing"
	}
	enqueuedAt := time.UnixMilli(producerEnqueuedUnixMS)
	if enqueuedAt.After(receivedAt) {
		return 0, "future"
	}
	return receivedAt.Sub(enqueuedAt), "valid"
}

// observeCollectorDeliveryQueueWait accepts ONLY a producer-generated epoch
// stored in task metadata. Never pass DB CreatedAt/UpdatedAt converted to epoch:
// timestamp-without-time-zone has no trustworthy absolute instant.
// receivedAt is captured immediately on handler entry, before claim/read.
// Missing metadata is expected for legacy tasks; it is not a zero-second wait.
func observeCollectorDeliveryQueueWait(ctx context.Context, meta collectorObservationMeta, producerEnqueuedUnixMS int64, receivedAt time.Time) {
	elapsed, sample := collectorDeliveryWaitSample(producerEnqueuedUnixMS, receivedAt)
	instruments := collectorPipelineInstrumentsFor(ctx)
	attrs := append(meta.metricAttributes(), attribute.String("sample", sample))
	if instruments.waitSample != nil {
		instruments.waitSample.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if sample == "valid" && instruments.queueWait != nil {
		instruments.queueWait.Record(ctx, elapsed.Seconds(), metric.WithAttributes(meta.metricAttributes()...))
	}
}
