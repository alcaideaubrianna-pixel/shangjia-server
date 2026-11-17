package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/internal/library/collectobserve"
)

// Real Asynq consumption exercises its queue context and ACK/retry behavior.
// No DB is configured: an accidental fallthrough into processEvent cannot pass.
func TestExpiredFreshEventReliableHistoryHandoff(t *testing.T) {
	previous := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent(`{"database":{},"telegramCollector":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previous) })
	t.Setenv("YOUBAN_COLLECTOR_REALTIME_MAX_WAIT_SECONDS", "120")
	t.Setenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED", "false")
	for _, failHandoff := range []bool{false, true} {
		name := "ack_after_history"
		if failHandoff {
			name = "retry_on_enqueue_failure"
		}
		t.Run(name, func(t *testing.T) {
			r := miniredis.RunT(t)
			opt := asynq.RedisClientOpt{Addr: r.Addr()}
			producer := asynq.NewClient(opt)
			defer producer.Close()
			handoffClient := asynq.NewClient(opt)
			defer handoffClient.Close()
			inspector := asynq.NewInspector(opt)
			defer inspector.Close()
			collector := &sCollector{queueClient: handoffClient, queueInspector: inspector}
			if failHandoff {
				// Only break the handoff client; the worker's ACK/retry Redis remains healthy.
				if err := handoffClient.Close(); err != nil {
					t.Fatal(err)
				}
			}
			const eventID int64 = 987654321
			raw, err := collectorFreshEventPayload(context.Background(), eventID, time.Now().Add(-3*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			const freshID = "fresh-handoff-test"
			if _, err := producer.Enqueue(asynq.NewTask(eventTaskType, raw), asynq.Queue(consts.QueueCollectFresh), asynq.TaskID(freshID), asynq.MaxRetry(2), asynq.Retention(time.Minute)); err != nil {
				t.Fatal(err)
			}
			observed := make(chan error, 1)
			mux := asynq.NewServeMux()
			mux.HandleFunc(eventTaskType, func(ctx context.Context, task *asynq.Task) error {
				err := collector.handleEventTask(ctx, task)
				if err == nil {
					info, inspectErr := inspector.GetTaskInfo(consts.QueueCollectHistory, eventPendingTaskID(eventID))
					if inspectErr != nil {
						err = inspectErr
					} else {
						_, id, decodeErr := collectobserve.DecodeTask(ctx, info.Payload, time.Now())
						if info.Type != eventTaskType || id != eventID || decodeErr != nil || info.State != asynq.TaskStatePending {
							err = errors.New("history task identity/state not durable before ACK")
						}
					}
				}
				observed <- err
				return err
			})
			server := asynq.NewServer(opt, asynq.Config{
				Concurrency: 1, Queues: map[string]int{consts.QueueCollectFresh: 1},
				ShutdownTimeout: time.Second,
				RetryDelayFunc:  func(int, error, *asynq.Task) time.Duration { return time.Hour },
			})
			if err := server.Start(mux); err != nil {
				t.Fatal(err)
			}
			defer server.Shutdown()
			select {
			case err := <-observed:
				if (err != nil) != failHandoff {
					t.Fatalf("handoff error=%v failHandoff=%v", err, failHandoff)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("fresh task was not consumed")
			}
			want := asynq.TaskStateCompleted
			if failHandoff {
				want = asynq.TaskStateRetry
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				info, err := inspector.GetTaskInfo(consts.QueueCollectFresh, freshID)
				if err == nil && info.State == want {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("fresh ACK/retry state: info=%+v err=%v want=%v", info, err, want)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if failHandoff {
				if _, err := inspector.GetTaskInfo(consts.QueueCollectHistory, eventPendingTaskID(eventID)); !errors.Is(err, asynq.ErrTaskNotFound) && !errors.Is(err, asynq.ErrQueueNotFound) {
					t.Fatalf("failed handoff unexpectedly created history task: %v", err)
				}
			}
		})
	}
}
