package collectobserve

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBindPublishEventClearsBranchForSameNumericIdentity(t *testing.T) {
	received := time.Unix(1800000000, 123)
	ctx := With(context.Background(), Metadata{EventID: 42, RuleID: 9, ProfileID: 10,
		DeliveryID: 77, ReceivedAt: received, Mode: "realtime", TimeBasis: "gateway_ingress"})
	got := From(BindPublishEvent(ctx, 42))
	if got.EventID != 42 || got.UpstreamEventID != 42 || got.RuleID != 0 || got.ProfileID != 0 ||
		got.DeliveryID != 77 || !got.ReceivedAt.Equal(received) || got.TimeBasis != "gateway_ingress" {
		t.Fatalf("same numeric namespace handoff leaked branch: %+v", got)
	}
	if From(ctx).RuleID != 9 || From(ctx).ProfileID != 10 {
		t.Fatal("namespace binding mutated parent metadata")
	}
}

func TestLocalClockSurvivesNumericQueuesAndPublishIdentity(t *testing.T) {
	received := time.Unix(1800000000, 123456789)
	enqueued := received.Add(time.Second)
	ingress := With(context.Background(), Metadata{ReceivedAt: received, Mode: "history"})
	raw, err := StampPayload(ingress, []byte(`{"date":100,"receivedAt":"1990-01-01T00:00:00Z"}`), enqueued)
	if err != nil {
		t.Fatal(err)
	}
	// Default carrier-off/Unique tasks continue to use the original numeric ID.
	consumer, id, err := DecodeTask(context.Background(), []byte("42"), enqueued.Add(8*24*time.Hour))
	if err != nil || id != 42 || !From(consumer).ReceivedAt.IsZero() {
		t.Fatalf("numeric task changed: id=%d meta=%+v err=%v", id, From(consumer), err)
	}
	consumer = With(consumer, Metadata{UpstreamEventID: id, DeliveryID: 77})
	consumer = RestorePayloadClock(consumer, raw, enqueued.Add(8*24*time.Hour))
	publish := BindPublishEvent(consumer, 9001)
	meta := From(publish)
	if meta.EventID != 9001 || meta.UpstreamEventID != 42 || meta.DeliveryID != 77 ||
		!meta.ReceivedAt.Equal(received) || meta.Mode != "history" {
		t.Fatalf("collector to publish handoff lost observation: %+v", meta)
	}
	stored, err := StampPayload(publish, []byte(`{}`), enqueued.Add(8*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// Recovery and retries bind the retained event payload, not a fresh clock.
	retry := RestorePayloadClock(context.Background(), stored, enqueued.Add(9*24*time.Hour))
	if got := From(retry); !got.ReceivedAt.Equal(received) || got.Mode != "history" {
		t.Fatalf("retry lost original clock/mode: %+v", got)
	}
	var business struct {
		Date int `json:"date"`
	}
	if err := json.Unmarshal(raw, &business); err != nil || business.Date != 100 {
		t.Fatalf("business JSON changed: %+v %v", business, err)
	}
}

func TestPayloadClockRejectsLegacyAndUnrelatedParent(t *testing.T) {
	now := time.Unix(1800000000, 0)
	parent := With(context.Background(), Metadata{EventID: 42, Mode: "realtime", ReceivedAt: now.Add(-time.Hour)})
	for _, raw := range []string{
		`[]`, `{}`, `null`, `invalid`,
		`{"date":1800000000,"receivedAt":"2026-11-18T12:00:00Z"}`,
		`{"_collectLocalObservation":{"version":2,"receivedAtUnixNano":1,"capturedAtUnixNano":2}}`,
		`{"_collectLocalObservation":{"version":1,"receivedAtUnixNano":3,"capturedAtUnixNano":2}}`,
		`{"_collectLocalObservation":{"version":1,"receivedAtUnixNano":1,"capturedAtUnixNano":9000000000000000000}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			got := From(RestorePayloadClock(parent, []byte(raw), now))
			if !got.ReceivedAt.IsZero() || got.Mode != "unknown" || got.EventID != 42 {
				t.Fatalf("legacy/bad payload inherited parent clock: %+v", got)
			}
		})
	}
	if !From(parent).ReceivedAt.Equal(now.Add(-time.Hour)) {
		t.Fatal("restoring a branch mutated its parent")
	}
}

func TestStampPayloadOverwritesUntrustedMarkerAndFailsOpen(t *testing.T) {
	now := time.Unix(1800000000, 0)
	raw := []byte(`{"date":123,"_collectLocalObservation":{"version":1,"mode":"history","receivedAtUnixNano":1,"capturedAtUnixNano":2}}`)
	clean, err := StampPayload(context.Background(), raw, now)
	if err != nil || !PayloadReceivedAt(clean, now).IsZero() {
		t.Fatalf("caller-supplied clock was trusted: %s %v", clean, err)
	}
	for _, raw := range []string{`broken`, `[]`, `42`} {
		got, err := StampPayload(context.Background(), []byte(raw), now)
		if err == nil || string(got) != raw {
			t.Fatalf("telemetry failure must retain business bytes: %q %v", got, err)
		}
	}
	future := With(context.Background(), Metadata{ReceivedAt: now.Add(time.Second)})
	got, err := StampPayload(future, []byte(`{"date":123}`), now)
	if err != nil || !PayloadReceivedAt(got, now).IsZero() {
		t.Fatalf("future clock persisted: %s %v", got, err)
	}
}
