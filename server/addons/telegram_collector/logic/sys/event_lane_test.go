package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/utility/sourcequeue"
)

func TestEventLaneRouting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		priority int
		origin   eventEnqueueOrigin
		queue    string
	}{
		{"new_bot", sysin.EventPriorityUrgent, eventOriginIngress, consts.QueueCollectFresh},
		{"new_account", sysin.EventPriorityRealtime, eventOriginIngress, consts.QueueCollectRealtime},
		{"bot_recovery", sysin.EventPriorityUrgent, eventOriginRecovery, consts.QueueCollectHistory},
		{"bot_replay", sysin.EventPriorityUrgent, eventOriginReplay, consts.QueueCollectHistory},
		{"account_history", sysin.EventPriorityNormal, eventOriginIngress, consts.QueueCollectHistory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventQueueFor(tc.priority, tc.origin); got != tc.queue {
				t.Fatalf("queue = %q; want %q", got, tc.queue)
			}
		})
	}
	if !eventQueueIsHistory(consts.QueueCollectUrgent) || !eventQueueIsHistory(consts.QueueCollectUrgentLive) || eventQueueIsHistory(consts.QueueCollectFresh) {
		t.Fatal("legacy backlog and live ingress must use separate execution budgets")
	}
}

func TestEventPendingIdentitySurvivesUniqueTTLAndRecovery(t *testing.T) {
	r := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: r.Addr()}
	c := asynq.NewClient(opt)
	defer c.Close()
	i := asynq.NewInspector(opt)
	defer i.Close()
	queue, taskID := consts.QueueCollectFresh, eventPendingTaskID(42)
	_, err := c.EnqueueContext(context.Background(), asynq.NewTask("test:event", []byte("42")), asynq.Queue(queue), asynq.TaskID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	r.FastForward(6 * time.Minute)
	_, err = c.Enqueue(asynq.NewTask("test:event", []byte("42")), asynq.Queue(queue), asynq.TaskID(taskID))
	if !errors.Is(err, asynq.ErrTaskIDConflict) {
		t.Fatalf("duplicate pending task: %v", err)
	}
	if err := confirmEventTaskConflict(i, queue, taskID); err != nil {
		t.Fatal(err)
	}
	if exists, err := eventLiveTaskExists(i, 42, sysin.EventPriorityUrgent); err != nil || !exists {
		t.Fatalf("recovery must preserve existing live task: exists=%v err=%v", exists, err)
	}
	if err := i.ArchiveTask(queue, taskID); err != nil {
		t.Fatal(err)
	}
	if exists, err := eventLiveTaskExists(i, 42, sysin.EventPriorityUrgent); err != nil || exists {
		t.Fatalf("archive is not executable work: exists=%v err=%v", exists, err)
	}
	if err := confirmEventTaskConflict(i, queue, taskID); err != nil {
		t.Fatal(err)
	}
	info, err := i.GetTaskInfo(queue, taskID)
	if err != nil || info.State != asynq.TaskStatePending {
		t.Fatalf("archive recovery: info=%+v err=%v", info, err)
	}
	if err := i.DeleteTask(queue, taskID); err != nil {
		t.Fatal(err)
	}
	if err := confirmEventTaskConflict(i, queue, taskID); err == nil {
		t.Fatal("missing task must not confirm recovery")
	}
	if exists, err := eventLiveTaskExists(i, 42, sysin.EventPriorityUrgent); err != nil || exists {
		t.Fatalf("missing live task: exists=%v err=%v", exists, err)
	}
}

func TestEventRecoveryPreservesLegacyV2DuringFreshRollout(t *testing.T) {
	_, q := eventSourceQueueRedisFixture(t)
	r := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: r.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	const id int64 = 77
	if _, err := client.Enqueue(asynq.NewTask(eventTaskType, collectorTaskPayload(id)), asynq.Queue(consts.QueueCollectUrgentLive), asynq.TaskID(eventPendingTaskID(id))); err != nil {
		t.Fatal(err)
	}
	collector := &sCollector{queueClient: client, queueInspector: inspector}
	scope := sourcequeue.Scope{TenantID: 78, SourceID: 351}
	ctx := withCollectorSourceScope(context.Background(), scope.TenantID, scope.SourceID)
	if err := collector.enqueueEventTask(ctx, id, sysin.EventPriorityUrgent, eventOriginIngress); err != nil {
		t.Fatal(err)
	}
	// Recovery must not duplicate or advance the generation of a live claim.
	claim, err := q.Claim(ctx, sourcequeue.Realtime, eventLeaseDuration)
	if err != nil || claim == nil {
		t.Fatalf("live claim=%+v err=%v", claim, err)
	}
	if err := collector.enqueueEventTask(ctx, id, sysin.EventPriorityUrgent, eventOriginRecovery); err != nil {
		t.Fatal(err)
	}
	if exists, err := q.HasTask(ctx, scope, sourcequeue.History, "77"); err != nil || exists {
		t.Fatalf("recovery interrupted live: history=%v err=%v", exists, err)
	}
	if err = q.Ack(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if exists, err := q.HasTask(ctx, scope, sourcequeue.Realtime, "77"); err != nil || exists {
		t.Fatalf("recovery re-generated live task: exists=%v err=%v", exists, err)
	}
	if _, err := inspector.GetTaskInfo(consts.QueueCollectUrgentLive, eventPendingTaskID(id)); err != nil {
		t.Fatalf("legacy backlog removed: %v", err)
	}
	// Once no source task remains, recovery is explicitly historical.
	if err := collector.enqueueEventTask(ctx, id, sysin.EventPriorityUrgent, eventOriginRecovery); err != nil {
		t.Fatal(err)
	}
	if exists, err := q.HasTask(ctx, scope, sourcequeue.History, "77"); err != nil || !exists {
		t.Fatalf("missing historical recovery: exists=%v err=%v", exists, err)
	}
}
