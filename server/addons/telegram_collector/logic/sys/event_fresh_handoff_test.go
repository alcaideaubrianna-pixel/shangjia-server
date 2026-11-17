package sys

import (
	"context"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/utility/sourcequeue"
	"testing"
	"time"
)

// Override both configuration and a possibly cached default Redis client.
// All consumers in these tests point at disposable local Redis, never a DB.
func eventSourceQueueRedisFixture(t *testing.T) (*miniredis.Miniredis, *sourcequeue.Queue) {
	t.Helper()
	r := miniredis.RunT(t)
	previous := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + r.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	shared := g.Redis()
	oldAdapter := shared.GetAdapter()
	local, err := gredis.New(&gredis.Config{Address: r.Addr()})
	if err != nil {
		g.Cfg().SetAdapter(previous)
		t.Fatal(err)
	}
	shared.SetAdapter(local.GetAdapter())
	t.Cleanup(func() {
		shared.SetAdapter(oldAdapter)
		_ = local.Close(context.Background())
		g.Cfg().SetAdapter(previous)
	})
	q, err := sourcequeue.New(collectorEventSourceNamespace)
	if err != nil {
		t.Fatal(err)
	}
	return r, q
}

// Legacy shared tasks migrate to source history before the Asynq receipt is
// ACKed. The age is legacy input only, never a reason to demote a new source task.
func TestExpiredFreshEventReliableHistoryHandoff(t *testing.T) {
	for _, failHandoff := range []bool{false, true} {
		name := "ack_after_source_handoff"
		if failHandoff {
			name = "retry_on_source_failure"
		}
		t.Run(name, func(t *testing.T) {
			sourceRedis, q := eventSourceQueueRedisFixture(t)
			legacyRedis := miniredis.RunT(t)
			opt := asynq.RedisClientOpt{Addr: legacyRedis.Addr()}
			producer := asynq.NewClient(opt)
			defer producer.Close()
			inspector := asynq.NewInspector(opt)
			defer inspector.Close()
			collector := &sCollector{}
			scope := sourcequeue.Scope{TenantID: 78, SourceID: 351}
			ctx := withCollectorSourceScope(context.Background(), scope.TenantID, scope.SourceID)
			const eventID int64 = 987654321
			raw, err := collectorFreshEventPayload(ctx, eventID, time.Now().Add(-3*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			const taskID = "fresh-handoff-test"
			if _, err = producer.Enqueue(asynq.NewTask(eventTaskType, raw), asynq.Queue(consts.QueueCollectFresh), asynq.TaskID(taskID), asynq.MaxRetry(2), asynq.Retention(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if failHandoff {
				sourceRedis.SetError("ERR injected source queue failure")
			}
			observed := make(chan error, 1)
			mux := asynq.NewServeMux()
			mux.HandleFunc(eventTaskType, func(workerCtx context.Context, task *asynq.Task) error {
				err := collector.handleEventTask(withCollectorSourceScope(workerCtx, scope.TenantID, scope.SourceID), task)
				observed <- err
				return err
			})
			server := asynq.NewServer(opt, asynq.Config{Concurrency: 1, Queues: map[string]int{consts.QueueCollectFresh: 1}, ShutdownTimeout: time.Second, RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return time.Hour }})
			if err = server.Start(mux); err != nil {
				t.Fatal(err)
			}
			defer server.Shutdown()
			select {
			case err = <-observed:
				if (err != nil) != failHandoff {
					t.Fatalf("handoff err=%v failure=%v", err, failHandoff)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("legacy task not handled")
			}
			want := asynq.TaskStateCompleted
			if failHandoff {
				want = asynq.TaskStateRetry
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				info, err := inspector.GetTaskInfo(consts.QueueCollectFresh, taskID)
				if err == nil && info.State == want {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("legacy receipt info=%+v err=%v want=%v", info, err, want)
				}
				time.Sleep(10 * time.Millisecond)
			}
			sourceRedis.SetError("")
			exists, err := q.HasTask(ctx, scope, sourcequeue.History, "987654321")
			if err != nil || exists == failHandoff {
				t.Fatalf("source durable handoff exists=%v err=%v failure=%v", exists, err, failHandoff)
			}
			if failHandoff {
				return
			}
			claim, err := q.Claim(ctx, sourcequeue.History, eventLeaseDuration)
			if err != nil || claim == nil || claim.ID != "987654321" || claim.Scope != scope {
				t.Fatalf("history claim=%+v err=%v", claim, err)
			}
			var payload collectorSourceEventPayload
			if err = json.Unmarshal(claim.Payload, &payload); err != nil || payload.EventID != eventID {
				t.Fatalf("identity lost: payload=%+v err=%v", payload, err)
			}
			if err = q.Ack(ctx, claim); err != nil {
				t.Fatal(err)
			}
		})
	}
}
