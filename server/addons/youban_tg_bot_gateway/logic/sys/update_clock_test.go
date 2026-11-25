package sys

import (
	"context"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"
)

func TestGatewayUpdateClockUsesExistingProducerEpoch(t *testing.T) {
	received := time.Unix(1800000000, 123456789)
	ctx := gatewayUpdateClock(context.Background(), received.UnixNano(), "webhook_received", received.Add(24*time.Hour))
	meta := collectobserve.From(ctx)
	if !meta.ReceivedAt.Equal(received) || meta.TimeBasis != "gateway_ingress" || meta.Mode != "realtime" {
		t.Fatalf("gateway enqueue clock lost: %+v", meta)
	}
	// Collector enriches with the existing ingress instant, without changing
	// the boundary label; retries retain the producer epoch.
	ctx = collectobserve.With(ctx, collectobserve.Metadata{ReceivedAt: meta.ReceivedAt, UpstreamEventID: 42})
	raw, err := collectobserve.StampPayload(ctx, []byte(`{}`), received.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	retry := collectobserve.RestorePayloadClock(context.Background(), raw, received.Add(48*time.Hour))
	got := collectobserve.From(collectobserve.BindPublishEvent(retry, 9001))
	if !got.ReceivedAt.Equal(received) || got.TimeBasis != "gateway_ingress" {
		t.Fatalf("collector/publish handoff changed boundary: %+v", got)
	}
}

func TestGatewayUpdateClockRejectsMissingAndFutureEpoch(t *testing.T) {
	now := time.Unix(1800000000, 0)
	stale := collectobserve.With(context.Background(), collectobserve.Metadata{ReceivedAt: now.Add(-time.Hour)})
	for _, epoch := range []int64{0, -1, now.Add(time.Second).UnixNano()} {
		got := collectobserve.From(gatewayUpdateClock(stale, epoch, "gateway_ingress", now))
		if !got.ReceivedAt.IsZero() || got.TimeBasis != "" {
			t.Fatalf("invalid enqueue epoch inherited a clock: %+v", got)
		}
	}
}
