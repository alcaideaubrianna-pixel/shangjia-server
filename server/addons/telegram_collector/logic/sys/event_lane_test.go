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
	r := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: r.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	const id int64 = 77
	_, err := client.Enqueue(asynq.NewTask(eventTaskType, collectorTaskPayload(id)),
		asynq.Queue(consts.QueueCollectUrgentLive), asynq.TaskID(eventPendingTaskID(id)))
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := eventLiveTaskExists(inspector, id, sysin.EventPriorityUrgent); err != nil || !exists {
		t.Fatalf("legacy v2 task must prevent duplicate recovery: exists=%v err=%v", exists, err)
	}
	collector := &sCollector{queueClient: client, queueInspector: inspector}
	if err := collector.enqueueEventTask(context.Background(), id, sysin.EventPriorityUrgent, eventOriginRecovery); err != nil {
		t.Fatal(err)
	}
	pending, err := inspector.ListPendingTasks(consts.QueueCollectHistory)
	if err != nil && !errors.Is(err, asynq.ErrQueueNotFound) {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("recovery duplicated legacy v2 task: %d", len(pending))
	}
	if _, err := inspector.GetTaskInfo(consts.QueueCollectUrgentLive, eventPendingTaskID(id)); err != nil {
		t.Fatalf("legacy task was removed: %v", err)
	}
	if err := inspector.ArchiveTask(consts.QueueCollectUrgentLive, eventPendingTaskID(id)); err != nil {
		t.Fatal(err)
	}
	if exists, err := eventLiveTaskExists(inspector, id, sysin.EventPriorityUrgent); err != nil || exists {
		t.Fatalf("archived v2 must not block recovery: exists=%v err=%v", exists, err)
	}
}
