package sys

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/hibiken/asynq"
)

func TestCollectQueuesUseSharedWorkers(t *testing.T) {
	if tgQueueNameBackground == tgQueueNameMediaRealtime {
		t.Fatal("collect processing and media downloading must use separate shared queues")
	}
}

func TestCollectMediaQueueNamePrioritizesRealtime(t *testing.T) {
	if got := collectMediaQueueName(context.Background(), collectMediaQueuePayload{TgAccountId: 13}); got != tgQueueNameMediaRealtime {
		t.Fatalf("realtime queue = %s", got)
	}
	cases := map[int64]string{
		12: collectMediaBulkQueueName(12),
		13: collectMediaBulkQueueName(13),
		14: collectMediaBulkQueueName(14),
		15: collectMediaBulkQueueName(15),
		17: collectMediaBulkQueueName(1),
	}
	for accountID, want := range cases {
		if got := collectMediaQueueName(context.Background(), collectMediaQueuePayload{TgAccountId: accountID, Bulk: true}); got != want {
			t.Fatalf("account %d queue = %s, want %s", accountID, got, want)
		}
	}
}

func TestCollectMediaWorkerQueuesIncludeAllBulkShards(t *testing.T) {
	queues := collectMediaWorkerQueues(context.Background())
	if len(queues) != collectMediaMaxBulkQueueShards+2 {
		t.Fatalf("queue count = %d", len(queues))
	}
	if queues[tgQueueNameMediaRealtime] <= queues[collectMediaBulkQueueName(0)] {
		t.Fatal("realtime queue must have a higher default weight")
	}
}

func TestFairCollectMediaQueuePayloadsRoundRobinsAccounts(t *testing.T) {
	payloads := []collectMediaQueuePayload{
		{EventId: 1, TenantId: 1, TgAccountId: 10, Bulk: true},
		{EventId: 2, TenantId: 1, TgAccountId: 10, Bulk: true},
		{EventId: 3, TenantId: 1, TgAccountId: 20, Bulk: true},
		{EventId: 4, TenantId: 1, TgAccountId: 20, Bulk: true},
		{EventId: 5, TenantId: 1, TgAccountId: 30},
	}
	ordered := fairCollectMediaQueuePayloads(payloads)
	want := []int64{5, 1, 3, 2, 4}
	if len(ordered) != len(want) {
		t.Fatalf("ordered length = %d", len(ordered))
	}
	for index, eventID := range want {
		if ordered[index].EventId != eventID {
			t.Fatalf("ordered[%d] = %d, want %d", index, ordered[index].EventId, eventID)
		}
	}
}

func TestCollectProcessTaskBodyIsSourceScoped(t *testing.T) {
	first, err := collectProcessTaskBody(collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, EventId: 100})
	if err != nil {
		t.Fatalf("marshal first payload: %v", err)
	}
	second, err := collectProcessTaskBody(collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, EventId: 200})
	if err != nil {
		t.Fatalf("marshal second payload: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("source task payload must ignore event id: %s != %s", first, second)
	}
	var payload collectProcessQueuePayload
	if err = json.Unmarshal(first, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.EventId != 0 || payload.SourceId != 4 {
		t.Fatalf("unexpected normalized payload: %+v", payload)
	}
}

func TestListCollectSourceTasksScansAllPages(t *testing.T) {
	calls := 0
	list := func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
		calls++
		if calls == 1 {
			tasks := make([]*asynq.TaskInfo, 1000)
			for index := range tasks {
				tasks[index] = &asynq.TaskInfo{Type: tgTaskTypePublish}
			}
			return tasks, nil
		}
		payload, _ := json.Marshal(collectMediaQueuePayload{SourceId: 42, EventId: 9})
		return []*asynq.TaskInfo{{ID: "target", Queue: tgQueueNameMedia, Type: tgTaskTypeCollectMedia, Payload: payload}}, nil
	}
	tasks, err := listCollectSourceTasks(list, tgQueueNameMedia, 42)
	if err != nil {
		t.Fatalf("list source tasks: %v", err)
	}
	if calls != 2 || len(tasks) != 1 || tasks[0].ID != "target" {
		t.Fatalf("unexpected pagination result calls=%d tasks=%+v", calls, tasks)
	}
}

func TestExpectedTelegramObserveShutdownError(t *testing.T) {
	if !isExpectedTelegramObserveShutdownError(context.Canceled) {
		t.Fatal("context cancellation should be treated as an expected shutdown")
	}
	if !isExpectedTelegramObserveShutdownError(errors.New("sql: transaction has already been committed or rolled back")) {
		t.Fatal("completed transaction rollback should be treated as an expected shutdown")
	}
}

func TestAccountCollectOperationReturnsWhenTaskIsCanceled(t *testing.T) {
	worker := &accountCollectWorker{}
	taskCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.runOperation(context.Background(), nil, accountCollectOperationTask{
			ctx: taskCtx,
			run: func(ctx context.Context, _ *telegram.Client) error {
				<-ctx.Done()
				return ctx.Err()
			},
		})
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled account collect operation did not return")
	}
}
