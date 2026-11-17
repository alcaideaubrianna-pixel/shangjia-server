package collectobserve

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func carrierTestContext(t *testing.T, receivedAt time.Time) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	state, err := trace.ParseTraceState("vendor=value")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, TraceState: state,
	}))
	return With(ctx, Metadata{
		TenantID: 78, AccountID: 615, SourceID: 351, BotID: 64,
		UpstreamEventID: 19184281, DeliveryID: 9123,
		ChatID: "-1003763589169", SourceType: "bot", Mode: "realtime", ReceivedAt: receivedAt,
	})
}

func TestCarrierRoundTripW3CAndNanosecondTiming(t *testing.T) {
	enqueuedAt := time.Unix(1794888000, 123456789)
	receivedAt := enqueuedAt.Add(-time.Second)
	producer := carrierTestContext(t, receivedAt)
	carrier := Capture(producer, enqueuedAt)
	payload, err := json.Marshal(carrier)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"metadata"`, `"traceparent"`, `"tracestate"`, `"enqueuedAtUnixNano"`, `"receivedAtUnixNano"`} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("missing fixed wire field %s", field)
		}
	}
	if strings.Contains(string(payload), `"ReceivedAt"`) || strings.Contains(string(payload), `"TenantID"`) {
		t.Fatal("wire contract serialized Go field names/time.Time")
	}
	var restoredCarrier Carrier
	if err := json.Unmarshal(payload, &restoredCarrier); err != nil {
		t.Fatal(err)
	}
	consumerAt := enqueuedAt.Add(250*time.Millisecond + 7*time.Nanosecond)
	consumer := restoredCarrier.RestoreAt(context.Background(), consumerAt)
	wantSpan, gotSpan := trace.SpanContextFromContext(producer), trace.SpanContextFromContext(consumer)
	if !gotSpan.IsRemote() || gotSpan.TraceID() != wantSpan.TraceID() || gotSpan.SpanID() != wantSpan.SpanID() ||
		gotSpan.TraceState().String() != "vendor=value" || !gotSpan.IsSampled() {
		t.Fatalf("W3C context lost: %+v", gotSpan)
	}
	meta := From(consumer)
	if meta.EventID != 0 || meta.UpstreamEventID != 19184281 || meta.SourceID != 351 || !meta.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("metadata changed across transport: %+v", meta)
	}
	timing, ok := QueueTimingFrom(consumer)
	wait, sample := timing.Sample()
	if !ok || sample != "valid" || wait != 250*time.Millisecond+7*time.Nanosecond {
		t.Fatalf("nanosecond queue timing lost: wait=%s sample=%s", wait, sample)
	}
}

func TestCarrierSourceWakeClearsEventIdentityAndIngressClock(t *testing.T) {
	now := time.Unix(1794888000, 0)
	parent := With(carrierTestContext(t, now.Add(-time.Second)), Metadata{EventID: 440157, RuleID: 338, ProfileID: 831923})
	carrier := CaptureSourceWake(parent, now)
	consumer := carrier.RestoreAt(parent, now.Add(time.Second))
	meta := From(consumer)
	if meta.EventID != 0 || meta.UpstreamEventID != 0 || meta.DeliveryID != 0 ||
		meta.RuleID != 0 || meta.ProfileID != 0 || !meta.ReceivedAt.IsZero() ||
		meta.TenantID != 78 || meta.AccountID != 615 || meta.SourceID != 351 {
		t.Fatalf("source wake inherited one event's identity/age: %+v", meta)
	}
	if From(parent).EventID != 440157 {
		t.Fatal("source capture mutated its parent")
	}
}

func TestTaskCarrierAcceptsNumericLegacyWithoutFabricatingTime(t *testing.T) {
	now := time.Unix(1794888000, 0)
	parent := carrierTestContext(t, now.Add(-time.Second))
	ctx, id, err := DecodeTask(parent, []byte(" 12345\n"), now)
	if err != nil || id != 12345 {
		t.Fatalf("numeric payload rejected: id=%d err=%v", id, err)
	}
	timing, ok := QueueTimingFrom(ctx)
	if wait, sample := timing.Sample(); !ok || wait != 0 || sample != "missing" || !From(ctx).ReceivedAt.IsZero() ||
		From(ctx).UpstreamEventID != 0 {
		t.Fatal("legacy task inherited an unrelated event's clock or identity")
	}
}

func TestTaskCarrierEnvelopeKeepsBusinessIDAndIgnoresInvalidTrace(t *testing.T) {
	now := time.Unix(1794888000, 0)
	payload, err := EncodeTask(carrierTestContext(t, now.Add(-time.Second)), 12345, now)
	if err != nil {
		t.Fatal(err)
	}
	var envelope TaskCarrier
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Observation.Traceparent, envelope.Observation.Tracestate = "invalid", "invalid"
	payload, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	ctx, id, err := DecodeTask(context.Background(), payload, now.Add(time.Second))
	if err != nil || id != 12345 || trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("invalid optional trace altered task identity or became a valid remote trace")
	}
	for _, payload := range []string{"", "0", "-1", "garbage", `{"observationVersion":2,"taskId":12345}`, `{"observationVersion":1,"taskId":0}`} {
		if _, _, err := DecodeTask(context.Background(), []byte(payload), now); err == nil {
			t.Fatalf("invalid business task accepted: %q", payload)
		}
	}
}

func TestCarrierMissingAndFutureClocksAreNotZeroWaits(t *testing.T) {
	now := time.Unix(1794888000, 0)
	for _, tc := range []struct {
		timing QueueTiming
		sample string
	}{
		{QueueTiming{ReceivedAt: now}, "missing"},
		{QueueTiming{EnqueuedAtUnixNano: now.UnixNano()}, "missing"},
		{QueueTiming{EnqueuedAtUnixNano: now.Add(time.Second).UnixNano(), ReceivedAt: now}, "future"},
	} {
		if wait, sample := tc.timing.Sample(); sample != tc.sample || wait != 0 {
			t.Fatalf("invalid timing manufactured a wait: %s %s", wait, sample)
		}
	}
	meta := Metadata{EventID: 123, ReceivedAt: now.Add(time.Second)}
	carrier := Capture(With(context.Background(), meta), now)
	if !carrier.Metadata.ReceivedAt.IsZero() {
		t.Fatal("future ingress clock propagated as trusted")
	}
	carrier = Capture(With(context.Background(), Metadata{SourceID: 351, ReceivedAt: now.Add(-time.Second)}), now)
	if !carrier.Metadata.ReceivedAt.IsZero() {
		t.Fatal("source-only capture retained an event age")
	}
}
