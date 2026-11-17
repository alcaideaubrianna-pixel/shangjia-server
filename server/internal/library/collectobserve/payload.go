package collectobserve

import (
	"context"
	"encoding/json"
	"time"
)

// payloadClock is written only at local ingress. It is independent of queue
// carriers, business received_at, Telegram dates and database timezone settings.
// TimeBasis names the measured local boundary without claiming HTTP receipt.
func TimeBasis(value string) string {
	switch value {
	case "gateway_ingress", "webhook_received":
		return value
	}
	return "collector_ingress"
}

type payloadClock struct {
	Version            int    `json:"version"`
	Mode               string `json:"mode"`
	TimeBasis          string `json:"timeBasis"`
	ReceivedAtUnixNano int64  `json:"receivedAtUnixNano"`
	CapturedAtUnixNano int64  `json:"capturedAtUnixNano"`
}

type clockPayload struct {
	Clock *payloadClock `json:"_collectLocalObservation,omitempty"`
}

// StampPayload replaces any caller-supplied observation with the trusted local
// context clock. Call after computing business identity/hash from the raw input.
// The returned JSON retains all business fields and is safe to decode into the
// existing business structs (which ignore the reserved observation field).
func StampPayload(ctx context.Context, raw []byte, now time.Time) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw, err
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	delete(fields, "_collectLocalObservation")
	received := From(ctx).ReceivedAt
	if !received.IsZero() && received.UnixNano() > 0 && !now.IsZero() && !received.After(now) {
		clock, err := json.Marshal(payloadClock{Version: 1, Mode: Mode(From(ctx).Mode), TimeBasis: TimeBasis(From(ctx).TimeBasis), ReceivedAtUnixNano: received.UnixNano(), CapturedAtUnixNano: now.UnixNano()})
		if err != nil {
			return raw, err
		}
		fields["_collectLocalObservation"] = clock
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return raw, err
	}
	return encoded, nil
}

// PayloadReceivedAt accepts only versioned local observation, never a business
// receivedAt/date field. Legacy backlog without the marker stays unavailable.
func PayloadReceivedAt(raw []byte, now time.Time) time.Time {
	var payload clockPayload
	if json.Unmarshal(raw, &payload) != nil || payload.Clock == nil {
		return time.Time{}
	}
	clock := payload.Clock
	if clock.Version != 1 || clock.ReceivedAtUnixNano <= 0 ||
		clock.CapturedAtUnixNano < clock.ReceivedAtUnixNano || now.IsZero() ||
		clock.CapturedAtUnixNano > now.UnixNano() {
		return time.Time{}
	}
	return time.Unix(0, clock.ReceivedAtUnixNano)
}

// RestorePayloadClock binds the clock of the already loaded current event.
// Missing observation clears an unrelated parent clock. Queue timing and
// identity are preserved; this is not a queue receipt or a new ingress.
func RestorePayloadClock(ctx context.Context, raw []byte, now time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	meta := From(ctx)
	meta.ReceivedAt = PayloadReceivedAt(raw, now)
	meta.Mode = "unknown"
	meta.TimeBasis = ""
	var payload clockPayload
	if !meta.ReceivedAt.IsZero() && json.Unmarshal(raw, &payload) == nil && payload.Clock != nil {
		meta.Mode = Mode(payload.Clock.Mode)
		meta.TimeBasis = TimeBasis(payload.Clock.TimeBasis)
	}
	return context.WithValue(ctx, contextKey{}, meta)
}

// RestoreIngressClock binds a producer epoch from an existing transport envelope.
// Missing/future epochs clear stale observation and never invent a receipt time.
func RestoreIngressClock(ctx context.Context, epoch int64, basis string, now time.Time) context.Context {
	ctx = RestorePayloadClock(ctx, nil, now)
	if epoch <= 0 || now.IsZero() || epoch > now.UnixNano() {
		return ctx
	}
	return With(ctx, Metadata{ReceivedAt: time.Unix(0, epoch), TimeBasis: TimeBasis(basis), Mode: "realtime"})
}

// BindPublishEvent explicitly crosses identity namespaces even for equal IDs.
// Branch rule/profile identity is always cleared; upstream clock is preserved.
func BindPublishEvent(ctx context.Context, eventID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	meta := From(ctx)
	if meta.UpstreamEventID == 0 {
		meta.UpstreamEventID = meta.EventID
	}
	meta.EventID = eventID
	meta.RuleID, meta.ProfileID = 0, 0
	return context.WithValue(ctx, contextKey{}, meta)
}
