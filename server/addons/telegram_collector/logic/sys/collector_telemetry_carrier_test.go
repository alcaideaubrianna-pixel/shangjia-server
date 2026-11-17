package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"

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
	redis := miniredis.RunT(t)
	options := asynq.RedisClientOpt{Addr: redis.Addr()}
	client, inspector := asynq.NewClient(options), asynq.NewInspector(options)
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close() })
	collector := &sCollector{queueClient: client, queueInspector: inspector}
	ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
		UpstreamEventID: 123, ReceivedAt: time.Now().Add(-time.Second), SourceType: "bot", Mode: "realtime",
	})
	for i := 0; i < 3; i++ {
		if err := collector.enqueueEventTask(ctx, 123, sysin.EventPriorityRealtime); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := inspector.ListPendingTasks(consts.QueueCollectRealtime)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("fixed TaskID dedup changed: tasks=%d err=%v", len(tasks), err)
	}
	task := tasks[0]
	if task.ID != eventPendingTaskID(123) || task.MaxRetry != eventMaxAttempts-1 || task.Timeout != eventLeaseDuration {
		t.Fatalf("carrier changed task identity or retry budget: %+v", task)
	}
	ctx, id, err := collectobserve.DecodeTask(context.Background(), task.Payload, time.Now())
	if err != nil || id != 123 {
		t.Fatalf("stored carrier failed to decode: %v", err)
	}
	timing, ok := collectobserve.QueueTimingFrom(ctx)
	if !ok || timing.EnqueuedAtUnixNano <= 0 {
		t.Fatal("actual producer did not persist a trusted enqueue epoch")
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
