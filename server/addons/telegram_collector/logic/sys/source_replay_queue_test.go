package sys

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/utility/sourcequeue"
)

func TestSourceReplayPreservesExistingWorkAndQueuesMissingWorkOnce(t *testing.T) {
	_, q := eventSourceQueueRedisFixture(t)
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	collector := &sCollector{queueClient: client, queueInspector: inspector}
	scope := sourcequeue.Scope{TenantID: 78, SourceID: 351}
	ctx := withCollectorSourceScope(context.Background(), scope.TenantID, scope.SourceID)
	const eventID int64 = 12682221
	const missingEventID = eventID + 1
	original, err := client.Enqueue(asynq.NewTask(eventTaskType, collectorTaskPayload(eventID)), asynq.Queue(consts.QueueCollectUrgent), asynq.MaxRetry(eventMaxAttempts-1), asynq.Timeout(eventLeaseDuration))
	if err != nil {
		t.Fatal(err)
	}
	if err = collector.enqueueEventTask(ctx, eventID, sysin.EventPriorityUrgent); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		for _, id := range []int64{eventID, missingEventID} {
			if err = collector.enqueueSourceReplayEventTask(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	for lane, id := range map[sourcequeue.Lane]int64{sourcequeue.Realtime: eventID, sourcequeue.History: missingEventID} {
		taskID := strconv.FormatInt(id, 10)
		exists, err := q.HasTask(ctx, scope, lane, taskID)
		if err != nil || !exists {
			t.Fatalf("lane=%s exists=%v err=%v", lane, exists, err)
		}
		claim, err := q.Claim(ctx, lane, eventLeaseDuration)
		if err != nil || claim == nil || claim.Scope != scope || claim.ID != taskID {
			t.Fatalf("lane=%s claim=%+v err=%v", lane, claim, err)
		}
		var payload collectorSourceEventPayload
		if err = json.Unmarshal(claim.Payload, &payload); err != nil || payload.EventID != id {
			t.Fatalf("identity changed: err=%v payload=%+v", err, payload)
		}
		if err = q.Ack(ctx, claim); err != nil {
			t.Fatal(err)
		}
		if next, err := q.Claim(ctx, lane, eventLeaseDuration); err != nil || next != nil {
			t.Fatalf("replay duplicates lane=%s claim=%+v err=%v", lane, next, err)
		}
	}
	old, err := inspector.GetTaskInfo(consts.QueueCollectUrgent, original.ID)
	if err != nil || old.State != asynq.TaskStatePending || string(old.Payload) != string(collectorTaskPayload(eventID)) || old.MaxRetry != eventMaxAttempts-1 || old.Timeout != eventLeaseDuration {
		t.Fatalf("legacy receipt altered: info=%+v err=%v", old, err)
	}
	tasks, err := inspector.ListPendingTasks(consts.QueueCollectUrgent)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("legacy backlog count=%d err=%v", len(tasks), err)
	}
}
