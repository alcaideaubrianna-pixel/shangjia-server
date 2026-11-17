package sys

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"

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
type collectorObserveMode string

const (
	collectorModeRealtime collectorObserveMode = "realtime"
	collectorModeHistory  collectorObserveMode = "history"
	collectorModeUnknown  collectorObserveMode = "unknown"
)

const (
	collectorStageIngestBot         collectorObserveStage      = "ingest_bot"
	collectorStageIngestAccount     collectorObserveStage      = "ingest_account"
	collectorStageEventPersist      collectorObserveStage      = "event_persist"
	collectorStageEventEnqueue      collectorObserveStage      = "event_enqueue"
	collectorStageEventConsume      collectorObserveStage      = "event_consume"
	collectorStageEventTaskParse    collectorObserveStage      = "event_task_parse"
	collectorStageEventClaim        collectorObserveStage      = "event_claim"
	collectorStageEventRead         collectorObserveStage      = "event_read"
	collectorStageEventParse        collectorObserveStage      = "event_parse"
	collectorStageEventDispatch     collectorObserveStage      = "event_dispatch"
	collectorStageDeliveryEnqueue   collectorObserveStage      = "delivery_enqueue"
	collectorStageDeliveryConsume   collectorObserveStage      = "delivery_consume"
	collectorStageDeliveryTaskParse collectorObserveStage      = "delivery_task_parse"
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
	collectorResultTimeout          collectorObserveResult     = "timeout"
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
	QueueClass  collectorObserveQueueClass
	SourceType  collectorObserveSourceType
	Mode        collectorObserveMode
	TenantID    int64
	AccountID   int64
	BotID       int64
	TgAccountID int64
	SourceID    int64
	EventID     int64
	DeliveryID  int64
	ChatID      string
	MessageID   int64
	ReceivedAt  time.Time
}

type collectorPipelineInstruments struct {
	operations      metric.Int64Counter
	duration        metric.Float64Histogram
	candidates      metric.Int64Counter
	queueWait       metric.Float64Histogram
	waitSample      metric.Int64Counter
	eventQueueWait  metric.Float64Histogram
	eventWaitSample metric.Int64Counter
	started         metric.Int64Counter
	active          metric.Int64UpDownCounter
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
	eventQueueWait, e6 := meter.Float64Histogram("telegram_collector_pipeline_event_queue_wait_seconds",
		metric.WithUnit("s"), metric.WithDescription("Trusted event enqueue epoch to consumer receipt; never DB or Telegram time"),
		metric.WithExplicitBucketBoundaries(.01, .05, .1, .5, 1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 21600, 86400))
	eventWaitSample, e7 := meter.Int64Counter("telegram_collector_event_wait_samples",
		metric.WithUnit("{sample}"), metric.WithDescription("Event queue wait sample availability: valid, missing or future"))
	started, e8 := meter.Int64Counter("xiaohuiji.collect.transport.stage.started",
		metric.WithUnit("{attempt}"), metric.WithDescription("Started collector transport stage attempts; dimensions reflect start-time metadata"))
	active, e9 := meter.Int64UpDownCounter("xiaohuiji.collect.transport.stage.active",
		metric.WithUnit("{attempt}"), metric.WithDescription("Collector transport stage attempts started but not finished in this process; nested stages must not be summed"))
	instruments.operations, instruments.duration, instruments.candidates = operations, duration, candidates
	instruments.queueWait, instruments.waitSample = queueWait, waitSample
	instruments.eventQueueWait, instruments.eventWaitSample = eventQueueWait, eventWaitSample
	instruments.started, instruments.active = started, active
	return instruments, errors.Join(e1, e2, e3, e4, e5, e6, e7, e8, e9)
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
		collectorStageEventTaskParse, collectorStageDeliveryTaskParse,
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
	case collectorResultAccepted, collectorResultOK, collectorResultError, collectorResultDuplicate, collectorResultSkipped, collectorResultCanceled, collectorResultTimeout:
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
	return []attribute.KeyValue{
		attribute.String("queue_class", queue), attribute.String("source_type", source),
		attribute.String("collection_mode", collectorBoundedMode(meta.Mode)),
	}
}

func collectorBoundedMode(mode collectorObserveMode) string {
	switch mode {
	case collectorModeRealtime, collectorModeHistory:
		return string(mode)
	default:
		return string(collectorModeUnknown)
	}
}

func collectorModeFromHistory(history bool) collectorObserveMode {
	if history {
		return collectorModeHistory
	}
	return collectorModeRealtime
}

// Only explicit protocol fields determine mode. The legacy business parser may
// infer history using DB wall time; observation must not reuse that inference.
func collectorModeFromRaw(sourceType string, raw []byte) collectorObserveMode {
	if sourceType == sysin.SourceTypeBot {
		// Bot raw events are produced by the realtime Update ingress.
		return collectorModeRealtime
	}
	if sourceType != sysin.SourceTypeAccount {
		return collectorModeUnknown
	}
	var flags struct {
		History *bool `json:"history"`
	}
	if json.Unmarshal(raw, &flags) != nil || flags.History == nil {
		return collectorModeUnknown
	}
	return collectorModeFromHistory(*flags.History)
}

// Synchronize a value-copy of shared metadata before downstream stages/direct
// publish. Raw event AccountId for account transport is TgAccountID, not owner.
// ReceivedAt is supplied only at local ingress, never from persisted rows/raw.
// Collector EventID maps exclusively to UpstreamEventID. Shared EventID is the
// publish collect_event primary key and must never be read or overwritten here.
func collectorObservationContext(ctx context.Context, meta collectorObservationMeta) (context.Context, collectorObservationMeta) {
	ctx = collectobserve.With(ctx, collectobserve.Metadata{
		TenantID: meta.TenantID, AccountID: meta.AccountID, SourceID: meta.SourceID,
		BotID: meta.BotID, TgAccountID: meta.TgAccountID, UpstreamEventID: meta.EventID,
		DeliveryID: meta.DeliveryID, ChatID: meta.ChatID,
		SourceType: string(meta.SourceType), Mode: string(meta.Mode), ReceivedAt: meta.ReceivedAt,
	})
	shared := collectobserve.From(ctx)
	meta.TenantID, meta.AccountID, meta.SourceID = shared.TenantID, shared.AccountID, shared.SourceID
	meta.BotID, meta.TgAccountID = shared.BotID, shared.TgAccountID
	meta.EventID, meta.DeliveryID, meta.ChatID = shared.UpstreamEventID, shared.DeliveryID, shared.ChatID
	meta.SourceType, meta.Mode = collectorObserveSourceType(shared.SourceType), collectorObserveMode(shared.Mode)
	meta.ReceivedAt = shared.ReceivedAt
	return ctx, meta
}

func collectorIngressReceivedAt(ctx context.Context) time.Time {
	if receivedAt := collectobserve.From(ctx).ReceivedAt; !receivedAt.IsZero() {
		return receivedAt
	}
	return time.Now()
}

func (meta collectorObservationMeta) traceAttributes() []attribute.KeyValue {
	attrs := meta.metricAttributes()
	for _, item := range []struct {
		key string
		id  int64
	}{
		{"tenant_id", meta.TenantID}, {"account_id", meta.AccountID}, {"source_id", meta.SourceID},
		{"event_id", meta.EventID}, {"delivery_id", meta.DeliveryID}, {"message_id", meta.MessageID},
		{"bot_id", meta.BotID}, {"tg_account_id", meta.TgAccountID},
	} {
		if item.id != 0 {
			attrs = append(attrs, attribute.String(item.key, strconv.FormatInt(item.id, 10)))
		}
	}
	if meta.ChatID != "" {
		attrs = append(attrs, attribute.String("chat_id", meta.ChatID))
	}
	if !meta.ReceivedAt.IsZero() {
		attrs = append(attrs, attribute.Int64("received_at_unix_ms", meta.ReceivedAt.UnixMilli()))
	}
	return attrs
}

type collectorStageObservation struct {
	ctx         context.Context
	span        trace.Span
	started     time.Time
	stage       collectorObserveStage
	meta        collectorObservationMeta
	once        sync.Once
	instruments *collectorPipelineInstruments
	activeAttrs []attribute.KeyValue
}

// startCollectorStage starts one operation. Finish exactly where its outcome is
// known, before wrapping/returning errors. Keep the returned ctx on downstream calls.
func startCollectorStage(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta) (context.Context, *collectorStageObservation) {
	return startCollectorStageWithInstruments(ctx, stage, meta, collectorPipelineInstrumentsFor(ctx), collectorPipelineTracer)
}

// Explicit dependencies keep ManualReader tests isolated from the global SDK.
// Production always supplies collectorMeter's instruments and the global tracer.
func startCollectorStageWithInstruments(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, instruments collectorPipelineInstruments, tracer trace.Tracer) (context.Context, *collectorStageObservation) {
	startedAt := time.Now()
	ctx, meta = collectorObservationContext(ctx, meta)
	attrs := append(meta.traceAttributes(), attribute.String("stage", collectorBoundedStage(stage)))
	ctx, span := tracer.Start(ctx, "collector."+collectorBoundedStage(stage), trace.WithAttributes(attrs...))
	activeAttrs := append(meta.metricAttributes(), attribute.String("stage", collectorBoundedStage(stage)))
	options := metric.WithAttributes(activeAttrs...)
	if instruments.started != nil {
		instruments.started.Add(ctx, 1, options)
	}
	if instruments.active != nil {
		instruments.active.Add(ctx, 1, options)
	}
	return ctx, &collectorStageObservation{
		ctx: ctx, span: span, started: startedAt, stage: stage, meta: meta,
		instruments: &instruments, activeAttrs: activeAttrs,
	}
}

func endCollectorStage(observation *collectorStageObservation, result collectorObserveResult, err error) {
	observation.Finish(result, err)
}

func collectorObservationResult(result collectorObserveResult, err error) collectorObserveResult {
	if err == nil {
		return result
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return collectorResultTimeout
	}
	if errors.Is(err, context.Canceled) {
		return collectorResultCanceled
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return collectorResultTimeout
	}
	return collectorResultError
}

// Error text, Go concrete type names and remote error codes are never labels.
func collectorObservationErrorType(result collectorObserveResult, err error) string {
	result = collectorObservationResult(result, err)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case result == collectorResultTimeout:
		return "timeout"
	case result == collectorResultCanceled:
		return "canceled"
	case err != nil || result == collectorResultError:
		return "error"
	case collectorBoundedResult(result) == "unknown":
		return "unknown"
	default:
		return "none"
	}
}

// recordCollectorStage is the metric-only hook for existing structured log
// helpers. Pass that stage's local elapsed duration, not the whole pipeline age.
// Do not also start/end the same stage: that would double count. This function
// neither creates nor ends spans, and performs no logging or external queries.
func recordCollectorStage(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, result collectorObserveResult, elapsed time.Duration, err error) {
	collectorPipelineInstrumentsFor(ctx).recordStage(ctx, stage, meta, result, elapsed, err)
}

func (instruments collectorPipelineInstruments) recordStage(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, result collectorObserveResult, elapsed time.Duration, err error) {
	ctx, meta = collectorObservationContext(ctx, meta)
	result = collectorObservationResult(result, err)
	attrs := append(meta.metricAttributes(),
		attribute.String("stage", collectorBoundedStage(stage)),
		attribute.String("result", collectorBoundedResult(result)),
		attribute.String("error_type", collectorObservationErrorType(result, err)))
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
		instruments := observation.instruments
		if instruments == nil {
			fallback := collectorPipelineInstrumentsFor(observation.ctx)
			instruments = &fallback
		}
		instruments.recordStage(observation.ctx, observation.stage, observation.meta, result, elapsed, err)
		if observation.instruments != nil && instruments.active != nil {
			// Late identity enrichment must not change the series decremented.
			instruments.active.Add(observation.ctx, -1, metric.WithAttributes(observation.activeAttrs...))
		}
		defer observation.span.End()
		observation.span.SetAttributes(observation.meta.traceAttributes()...)
		observation.span.SetAttributes(attribute.String("result", collectorBoundedResult(result)))
		failed := result == collectorResultError || result == collectorResultCanceled || result == collectorResultTimeout
		// Do not record raw error text: Telegram errors may contain private
		// message content or signed URLs. Existing business logs own details.
		if failed {
			observation.span.SetStatus(codes.Error, collectorBoundedResult(result))
			observation.span.SetAttributes(attribute.String("error.type", collectorObservationErrorType(result, err)))
		}
		if failed || elapsed >= 100*time.Millisecond {
			fields := g.Map{"event": "collector.stage.completed", "stage": collectorBoundedStage(observation.stage),
				"result": collectorBoundedResult(result), "durationMs": float64(elapsed.Microseconds()) / 1000,
				"tenantId": observation.meta.TenantID, "accountId": observation.meta.AccountID,
				"sourceId": observation.meta.SourceID, "eventId": observation.meta.EventID,
				"deliveryId": observation.meta.DeliveryID, "chatId": observation.meta.ChatID,
				"messageId": observation.meta.MessageID, "botId": observation.meta.BotID,
				"tgAccountId": observation.meta.TgAccountID,
				"errorType":   collectorObservationErrorType(result, err)}
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
	})
}

// observeCollectorRecoveryCandidates counts rows already fetched by a recovery
// page. Call once per successful query, including empty pages. No extra query.
func observeCollectorRecoveryCandidates(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, count int) {
	if count <= 0 || (stage != collectorStageEventScan && stage != collectorStageDeliveryScan) {
		return
	}
	instruments := collectorPipelineInstrumentsFor(ctx)
	ctx, meta = collectorObservationContext(ctx, meta)
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
	startCollectorQueueWait(ctx, collectorStageDeliveryConsume, meta, producerEnqueuedUnixMS, receivedAt).Finish(meta)
}

// Numeric event tasks have no trusted enqueue timestamp. Count missing samples
// rather than substituting local first receipt or DB CreatedAt for enqueue.
func observeCollectorEventQueueWait(ctx context.Context, meta collectorObservationMeta, producerEnqueuedUnixMS int64, receivedAt time.Time) {
	startCollectorQueueWait(ctx, collectorStageEventConsume, meta, producerEnqueuedUnixMS, receivedAt).Finish(meta)
}

// A handle belongs to one actual queue-consume attempt. Construct at handler
// entry and finish after existing reads enrich metadata. Repeated Finish calls
// count one sample. Do not use this for a direct/in-process handoff.
// Numeric payload callers MUST pass 0, not ReceivedAt, TG date or DB timestamps.
func startCollectorQueueWait(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, producerEnqueuedUnixMS int64, receivedAt time.Time) *collectorQueueWaitObservation {
	return newCollectorQueueWaitObservation(ctx, stage, meta, producerEnqueuedUnixMS, receivedAt, collectorPipelineInstrumentsFor(ctx))
}

type collectorQueueWaitObservation struct {
	ctx         context.Context
	stage       collectorObserveStage
	meta        collectorObservationMeta
	enqueuedMS  int64
	receivedAt  time.Time
	instruments collectorPipelineInstruments
	timing      *collectobserve.QueueTiming
	once        sync.Once
}

type collectorQueueWaitContextKey struct{}

func collectorQueueWaitContext(ctx context.Context, wait *collectorQueueWaitObservation) context.Context {
	return context.WithValue(ctx, collectorQueueWaitContextKey{}, wait)
}

func collectorQueueWaitFromContext(ctx context.Context) *collectorQueueWaitObservation {
	wait, _ := ctx.Value(collectorQueueWaitContextKey{}).(*collectorQueueWaitObservation)
	return wait
}

func startCollectorQueueWaitFromTiming(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta) *collectorQueueWaitObservation {
	timing, ok := collectobserve.QueueTimingFrom(ctx)
	if !ok {
		return nil
	}
	wait := startCollectorQueueWait(ctx, stage, meta, 0, timing.ReceivedAt)
	if wait != nil {
		wait.timing = &timing
	}
	return wait
}

// Rolling upgrade gate: new consumers accept both formats immediately. Enable
// producers only after every consumer understands envelopes. Unique(payload)
// callers must pass false so timestamps/traces never change their old hash.
func collectorTaskPayloadForEnqueue(ctx context.Context, taskID int64, stableTaskIDOrNonUnique bool) []byte {
	enabled, _ := strconv.ParseBool(os.Getenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED"))
	if !stableTaskIDOrNonUnique || !enabled {
		return collectorTaskPayload(taskID)
	}
	payload, err := collectobserve.EncodeTask(ctx, taskID, time.Now())
	if err != nil {
		// Optional telemetry failure must not change enqueue/ACK/retry behavior.
		g.Log().Warning(ctx, "采集观测载体编码失败，保留原任务协议", g.Map{
			"event": "collector.carrier.encode_fallback", "taskId": taskID, "errorType": "carrier_encode",
		})
		return collectorTaskPayload(taskID)
	}
	return payload
}

func newCollectorQueueWaitObservation(ctx context.Context, stage collectorObserveStage, meta collectorObservationMeta, producerEnqueuedUnixMS int64, receivedAt time.Time, instruments collectorPipelineInstruments) *collectorQueueWaitObservation {
	if stage != collectorStageEventConsume && stage != collectorStageDeliveryConsume {
		return nil
	}
	ctx, meta = collectorObservationContext(ctx, meta)
	return &collectorQueueWaitObservation{
		ctx: ctx, stage: stage, meta: meta, enqueuedMS: producerEnqueuedUnixMS,
		receivedAt: receivedAt, instruments: instruments,
	}
}

func (observation *collectorQueueWaitObservation) Finish(meta collectorObservationMeta) {
	if observation == nil {
		return
	}
	observation.once.Do(func() {
		if meta.QueueClass == "" {
			meta.QueueClass = observation.meta.QueueClass
		}
		ctx, meta := collectorObservationContext(observation.ctx, meta)
		elapsed, sample := collectorDeliveryWaitSample(observation.enqueuedMS, observation.receivedAt)
		if observation.timing != nil {
			elapsed, sample = observation.timing.Sample()
		}
		histogram, samples := observation.instruments.eventQueueWait, observation.instruments.eventWaitSample
		if observation.stage == collectorStageDeliveryConsume {
			histogram, samples = observation.instruments.queueWait, observation.instruments.waitSample
		}
		attrs := append(meta.metricAttributes(), attribute.String("sample", sample))
		if samples != nil {
			samples.Add(ctx, 1, metric.WithAttributes(attrs...))
		}
		if sample == "valid" && histogram != nil {
			histogram.Record(ctx, elapsed.Seconds(), metric.WithAttributes(meta.metricAttributes()...))
		}
	})
}

func collectorObservationLogFields(ctx context.Context, fields g.Map) {
	_, meta := collectorObservationContext(ctx, collectorObservationMeta{})
	for _, attr := range meta.metricAttributes() {
		fields[string(attr.Key)] = attr.Value.AsString()
	}
	fields["traceID"] = trace.SpanContextFromContext(ctx).TraceID().String()
	fields["botId"], fields["tgAccountId"] = meta.BotID, meta.TgAccountID
}

func collectorCollectionAttributes(ctx context.Context) []attribute.KeyValue {
	_, meta := collectorObservationContext(ctx, collectorObservationMeta{})
	return []attribute.KeyValue{
		attribute.String("collection_mode", collectorBoundedMode(meta.Mode)),
	}
}
