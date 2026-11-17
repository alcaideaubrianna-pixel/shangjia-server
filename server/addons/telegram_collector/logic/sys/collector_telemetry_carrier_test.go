package sys

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"
	"hotgo/utility/sourcequeue"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestCollectorTelemetryCarrierGatePreservesNumericProtocol(t *testing.T) {
	t.Setenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED", "false")
	if got := string(collectorTaskPayloadForEnqueue(context.Background(), 123, true)); got != "123" {
		t.Fatalf("disabled gate changed numeric protocol: %s", got)
	}
	t.Setenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED", "true")
	// This is the same disallow argument used by ordinary Unique deliveries.
	if got := string(collectorTaskPayloadForEnqueue(context.Background(), 123, false)); got != "123" {
		t.Fatalf("carrier changed payload-based Unique hash input: %s", got)
	}
	payload := collectorTaskPayloadForEnqueue(context.Background(), 123, true)
	if id, err := parseCollectorTaskID(payload, "事件"); err != nil || id != 123 {
		t.Fatalf("new envelope no longer parses as the same durable ID: %v", err)
	}
}

func TestCollectorTelemetryEventCarrierKeepsFixedTaskIDDedup(t *testing.T) {
	t.Setenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED", "true")
	_, q := eventSourceQueueRedisFixture(t)
	collector := &sCollector{}
	scope := sourcequeue.Scope{TenantID: 78, SourceID: 351}
	ctx := withCollectorSourceScope(collectobserve.With(context.Background(), collectobserve.Metadata{
		UpstreamEventID: 123, ReceivedAt: time.Now().Add(-time.Second), SourceType: "bot", Mode: "realtime",
	}), scope.TenantID, scope.SourceID)
	for i := 0; i < 3; i++ {
		if err := collector.enqueueEventTask(ctx, 123, sysin.EventPriorityRealtime); err != nil {
			t.Fatal(err)
		}
	}
	claim, err := q.Claim(ctx, sourcequeue.Realtime, eventLeaseDuration)
	if err != nil || claim == nil {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	if claim.ID != "123" || claim.Scope != scope {
		t.Fatalf("lost stable identity: %+v", claim)
	}
	var payload collectorSourceEventPayload
	if err = json.Unmarshal(claim.Payload, &payload); err != nil || payload.EventID != 123 {
		t.Fatalf("carrier decode: %v", err)
	}
	restored := payload.Carrier.RestoreAt(context.Background(), time.Now())
	timing, ok := collectobserve.QueueTimingFrom(restored)
	if !ok || timing.EnqueuedAtUnixNano <= 0 {
		t.Fatal("producer lost trusted enqueue epoch")
	}
	if err = q.Ack(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if exists, err := q.HasTask(ctx, scope, sourcequeue.Realtime, "123"); err != nil || exists {
		t.Fatalf("duplicate work survived single ACK: exists=%v err=%v", exists, err)
	}
	if next, err := q.Claim(ctx, sourcequeue.Realtime, eventLeaseDuration); err != nil || next != nil {
		t.Fatalf("duplicate carrier created extra task: claim=%+v err=%v", next, err)
	}
}

func TestCollectorTelemetryUniqueDeliveryKeepsOldDedup(t *testing.T) {
	t.Setenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED", "true")
	redis := miniredis.RunT(t)
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redis.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	for i := 0; i < 2; i++ {
		payload := collectorTaskPayloadForEnqueue(context.Background(), 456, false)
		_, err := client.Enqueue(asynq.NewTask(deliveryTaskType, payload),
			asynq.Queue(consts.QueueDeliveryReady), asynq.Unique(deliveryTaskUniqueTTL))
		if i == 0 && err != nil {
			t.Fatal(err)
		}
		if i == 1 && !errors.Is(err, asynq.ErrDuplicateTask) {
			t.Fatalf("existing Unique no longer deduplicates numeric delivery: %v", err)
		}
	}
}

func TestCollectorTelemetryRestoredWaitRecordsRealNanosecondsOnce(t *testing.T) {
	instruments, reader, _, _ := collectorTelemetryFixture(t, sdktrace.NeverSample())
	enqueuedAt := time.Unix(1794888000, 123456789)
	carrier := collectobserve.Capture(context.Background(), enqueuedAt)
	ctx := carrier.RestoreAt(context.Background(), enqueuedAt.Add(125*time.Millisecond+3*time.Nanosecond))
	timing, _ := collectobserve.QueueTimingFrom(ctx)
	wait := newCollectorQueueWaitObservation(ctx, collectorStageEventConsume, collectorObservationMeta{}, 0, timing.ReceivedAt, instruments)
	wait.timing = &timing
	ctx = collectorQueueWaitContext(ctx, wait)
	collectorQueueWaitFromContext(ctx).Finish(collectorObservationMeta{SourceType: collectorSourceBot})
	wait.Finish(collectorObservationMeta{})
	histogram := collectorTelemetryHistogram(t, collectorTelemetryMeasurements(t, reader), "telegram_collector_pipeline_event_queue_wait_seconds")
	if len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 ||
		histogram.DataPoints[0].Sum != (125*time.Millisecond+3*time.Nanosecond).Seconds() {
		t.Fatalf("restored wait was missing, rounded or counted twice: %+v", histogram)
	}
}
