package sys

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"hotgo/internal/library/collectobserve"
)

func TestCollectLifecycleClockDoesNotClaimWebhookTimeBasis(t *testing.T) {
	now := time.Unix(1800000000, 0)
	ctx := collectobserve.With(context.Background(), collectobserve.Metadata{ReceivedAt: now.Add(-time.Minute)})
	fields := collectProfileLifecycleFields(ctx, 1, true, true, nil, now)
	if fields["timeBasis"] != "collector_ingress" || fields["lifecycleSeconds"] != float64(60) {
		t.Fatalf("collector clock was mislabeled as webhook: %+v", fields)
	}
}

func TestCollectEventClockBindsCurrentGroupPayload(t *testing.T) {
	now := time.Now().Add(-time.Minute)
	groupReceived := now.Add(-24 * time.Hour)
	raw, err := collectobserve.StampPayload(
		collectobserve.With(context.Background(), collectobserve.Metadata{ReceivedAt: groupReceived, Mode: "history"}),
		[]byte(`{}`), now,
	)
	if err != nil {
		t.Fatal(err)
	}
	parent := collectobserve.With(context.Background(), collectobserve.Metadata{
		EventID: 99, ReceivedAt: now.Add(-time.Second), Mode: "realtime", RuleID: 8,
	})
	event := gdb.Record{
		"id": gvar.New(42), "tenant_id": gvar.New(1), "source_id": gvar.New(2),
		"source_type": gvar.New("bot"), "media_json": gvar.New(string(raw)),
		// A Telegram/database date cannot affect lifecycle age.
		"received_at": gvar.New(time.Unix(1, 0)),
	}
	got := collectobserve.From(bindCollectEventClock(parent, event))
	if got.EventID != 42 || got.RuleID != 0 || !got.ReceivedAt.Equal(groupReceived) || got.Mode != "history" {
		t.Fatalf("group inherited unrelated observation: %+v", got)
	}
	// Missing legacy group observation must not inherit even a same-ID context.
	event["media_json"] = gvar.New(`[]`)
	got = collectobserve.From(bindCollectEventClock(bindCollectEventClock(parent, event), event))
	if !got.ReceivedAt.IsZero() || got.Mode != "unknown" {
		t.Fatalf("legacy group manufactured a lifecycle clock: %+v", got)
	}
}
