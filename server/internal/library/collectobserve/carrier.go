package collectobserve

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"go.opentelemetry.io/otel/propagation"
)

const taskCarrierVersion = 1
const taskCarrierMaxBytes = 16 * 1024

// TaskCarrier contains observation only. TaskID remains the existing durable
// task identity; Metadata MUST NOT authorize, route or change a retry budget.
// Never put this envelope under payload-based Asynq Unique: changing trace/time
// changes its unique hash. Fixed-TaskID or intentionally non-unique tasks only.
type TaskCarrier struct {
	Version     int     `json:"observationVersion"`
	TaskID      int64   `json:"taskId"`
	Observation Carrier `json:"observation"`
}

// Carrier is transport-independent. Its JSON contract uses explicit nanosecond
// epochs; Metadata's time.Time representation is never serialized.
type Carrier struct {
	Metadata           Metadata
	Traceparent        string
	Tracestate         string
	EnqueuedAtUnixNano int64
}

type carrierMetadata struct {
	TenantID           int64  `json:"tenantId,omitempty"`
	AccountID          int64  `json:"accountId,omitempty"`
	SourceID           int64  `json:"sourceId,omitempty"`
	BotID              int64  `json:"botId,omitempty"`
	TgAccountID        int64  `json:"tgAccountId,omitempty"`
	EventID            int64  `json:"eventId,omitempty"`
	UpstreamEventID    int64  `json:"upstreamEventId,omitempty"`
	DeliveryID         int64  `json:"deliveryId,omitempty"`
	RuleID             int64  `json:"ruleId,omitempty"`
	ProfileID          int64  `json:"profileId,omitempty"`
	ChatID             string `json:"chatId,omitempty"`
	SourceType         string `json:"sourceType"`
	Mode               string `json:"mode"`
	ReceivedAtUnixNano int64  `json:"receivedAtUnixNano,omitempty"`
	TimeBasis          string `json:"timeBasis,omitempty"`
}

type carrierWire struct {
	Metadata           carrierMetadata `json:"metadata"`
	Traceparent        string          `json:"traceparent,omitempty"`
	Tracestate         string          `json:"tracestate,omitempty"`
	EnqueuedAtUnixNano int64           `json:"enqueuedAtUnixNano"`
}

func Capture(ctx context.Context, enqueuedAt time.Time) Carrier {
	if ctx == nil {
		ctx = context.Background()
	}
	w3c := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, w3c)
	meta := From(ctx)
	meta.SourceType, meta.Mode = SourceType(meta.SourceType), Mode(meta.Mode)
	// No event identity means a source-wide wakeup, not one event's age.
	if (meta.EventID <= 0 && meta.UpstreamEventID <= 0) || meta.ReceivedAt.After(enqueuedAt) {
		meta.ReceivedAt = time.Time{}
	}
	var epoch int64
	if !enqueuedAt.IsZero() && enqueuedAt.UnixNano() > 0 {
		epoch = enqueuedAt.UnixNano()
	}
	return Carrier{Metadata: meta, Traceparent: w3c.Get("traceparent"),
		Tracestate: w3c.Get("tracestate"), EnqueuedAtUnixNano: epoch}
}

func CaptureSourceWake(ctx context.Context, enqueuedAt time.Time) Carrier {
	carrier := Capture(ctx, enqueuedAt)
	meta := &carrier.Metadata
	meta.EventID, meta.UpstreamEventID, meta.DeliveryID = 0, 0, 0
	meta.RuleID, meta.ProfileID, meta.ReceivedAt = 0, 0, time.Time{}
	return carrier
}

func (carrier Carrier) MarshalJSON() ([]byte, error) {
	meta := carrier.Metadata
	wire := carrierWire{
		Traceparent: carrier.Traceparent, Tracestate: carrier.Tracestate,
		EnqueuedAtUnixNano: carrier.EnqueuedAtUnixNano,
		Metadata: carrierMetadata{
			TenantID: meta.TenantID, AccountID: meta.AccountID, SourceID: meta.SourceID,
			BotID: meta.BotID, TgAccountID: meta.TgAccountID, EventID: meta.EventID,
			UpstreamEventID: meta.UpstreamEventID, DeliveryID: meta.DeliveryID,
			RuleID: meta.RuleID, ProfileID: meta.ProfileID, ChatID: meta.ChatID,
			SourceType: SourceType(meta.SourceType), Mode: Mode(meta.Mode), TimeBasis: TimeBasis(meta.TimeBasis),
		},
	}
	if !meta.ReceivedAt.IsZero() && meta.ReceivedAt.UnixNano() > 0 &&
		(meta.EventID > 0 || meta.UpstreamEventID > 0) &&
		meta.ReceivedAt.UnixNano() <= carrier.EnqueuedAtUnixNano {
		wire.Metadata.ReceivedAtUnixNano = meta.ReceivedAt.UnixNano()
	}
	return json.Marshal(wire)
}

func (carrier *Carrier) UnmarshalJSON(payload []byte) error {
	var wire carrierWire
	if err := json.Unmarshal(payload, &wire); err != nil {
		return err
	}
	meta := wire.Metadata
	carrier.Metadata = Metadata{
		TenantID: meta.TenantID, AccountID: meta.AccountID, SourceID: meta.SourceID,
		BotID: meta.BotID, TgAccountID: meta.TgAccountID, EventID: meta.EventID,
		UpstreamEventID: meta.UpstreamEventID, DeliveryID: meta.DeliveryID,
		RuleID: meta.RuleID, ProfileID: meta.ProfileID, ChatID: meta.ChatID,
		SourceType: SourceType(meta.SourceType), Mode: Mode(meta.Mode), TimeBasis: TimeBasis(meta.TimeBasis),
	}
	if meta.ReceivedAtUnixNano > 0 && meta.ReceivedAtUnixNano <= wire.EnqueuedAtUnixNano &&
		(meta.EventID > 0 || meta.UpstreamEventID > 0) {
		carrier.Metadata.ReceivedAt = time.Unix(0, meta.ReceivedAtUnixNano)
	}
	carrier.Traceparent, carrier.Tracestate = wire.Traceparent, wire.Tracestate
	carrier.EnqueuedAtUnixNano = wire.EnqueuedAtUnixNano
	return nil
}

func (carrier Carrier) Restore(ctx context.Context) context.Context {
	return carrier.RestoreAt(ctx, time.Now())
}

// RestoreAt lets queue handlers use their actual entry instant, before parsing.
func (carrier Carrier) RestoreAt(ctx context.Context, receivedAt time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{
		"traceparent": carrier.Traceparent, "tracestate": carrier.Tracestate,
	})
	meta := carrier.Metadata
	if receivedAt.IsZero() || meta.ReceivedAt.After(receivedAt) ||
		(meta.EventID <= 0 && meta.UpstreamEventID <= 0) {
		meta.ReceivedAt = time.Time{}
	}
	meta.SourceType, meta.Mode = SourceType(meta.SourceType), Mode(meta.Mode)
	// This is a new asynchronous execution scope, not an enrichment of the
	// worker's preceding event. Zero IDs/clock must erase stale parent values.
	ctx = context.WithValue(ctx, contextKey{}, meta)
	return context.WithValue(ctx, queueTimingKey{}, QueueTiming{
		EnqueuedAtUnixNano: carrier.EnqueuedAtUnixNano, ReceivedAt: receivedAt,
	})
}

// QueueTiming is the producer enqueue to this consumer receipt interval.
// It includes scheduling/retry residence, not only time on the pending list.
// A zero enqueue time denotes a legacy payload, never a zero-second wait.
type QueueTiming struct {
	EnqueuedAtUnixNano int64
	ReceivedAt         time.Time
}

func (timing QueueTiming) Sample() (time.Duration, string) {
	if timing.EnqueuedAtUnixNano <= 0 || timing.ReceivedAt.IsZero() {
		return 0, "missing"
	}
	enqueuedAt := time.Unix(0, timing.EnqueuedAtUnixNano)
	if enqueuedAt.After(timing.ReceivedAt) {
		return 0, "future"
	}
	return timing.ReceivedAt.Sub(enqueuedAt), "valid"
}

type queueTimingKey struct{}

func QueueTimingFrom(ctx context.Context) (QueueTiming, bool) {
	if ctx == nil {
		return QueueTiming{}, false
	}
	timing, ok := ctx.Value(queueTimingKey{}).(QueueTiming)
	return timing, ok
}

// EncodeTask injects only W3C trace context, never baggage. receivedAt comes
// exclusively from trusted local-ingress Metadata; Telegram/DB times are not
// accepted as parameters. The caller captures enqueuedAt immediately before
// the existing enqueue call. No network, storage or global-provider mutation.
func EncodeTask(ctx context.Context, taskID int64, enqueuedAt time.Time) ([]byte, error) {
	if taskID <= 0 || enqueuedAt.IsZero() || enqueuedAt.UnixMilli() <= 0 {
		return nil, gerror.New("采集观测载体缺少有效任务ID或入队时间")
	}
	envelope := TaskCarrier{
		Version: taskCarrierVersion, TaskID: taskID,
		Observation: Capture(ctx, enqueuedAt),
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, gerror.Wrap(err, "编码采集观测载体失败")
	}
	if len(payload) > taskCarrierMaxBytes {
		return nil, gerror.New("采集观测载体超过大小限制")
	}
	return payload, nil
}

// DecodeTask accepts old numeric payloads and versioned envelopes. Invalid W3C
// context is ignored by TraceContext; it never changes the business task ID.
// Missing/future observation time is not replaced with a DB or Telegram date.
func DecodeTask(ctx context.Context, payload []byte, receivedAt time.Time) (context.Context, int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(payload) > taskCarrierMaxBytes {
		return ctx, 0, gerror.New("采集任务载体超过大小限制")
	}
	payload = bytes.TrimSpace(payload)
	if id, err := strconv.ParseInt(string(payload), 10, 64); err == nil && id > 0 {
		ctx = context.WithValue(ctx, contextKey{}, Metadata{})
		return context.WithValue(ctx, queueTimingKey{}, QueueTiming{ReceivedAt: receivedAt}), id, nil
	}
	var envelope TaskCarrier
	if len(payload) == 0 || payload[0] != '{' || json.Unmarshal(payload, &envelope) != nil ||
		envelope.Version != taskCarrierVersion || envelope.TaskID <= 0 {
		return ctx, 0, gerror.New("采集任务ID或观测载体版本无效")
	}
	return envelope.Observation.RestoreAt(ctx, receivedAt), envelope.TaskID, nil
}
