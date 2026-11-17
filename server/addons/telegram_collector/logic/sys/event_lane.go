package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

type eventEnqueueOrigin string

const (
	eventOriginIngress  eventEnqueueOrigin = "ingress"
	eventOriginRecovery eventEnqueueOrigin = "recovery"
	eventOriginReplay   eventEnqueueOrigin = "source_replay"
)

// Queue membership records execution class, independent of message age.
// The old urgent queue is drained exclusively with the history budget.
func eventQueueFor(priority int, origin eventEnqueueOrigin) string {
	if origin == eventOriginRecovery || origin == eventOriginReplay || priority < sysin.EventPriorityRealtime {
		return consts.QueueCollectHistory
	}
	if priority >= sysin.EventPriorityUrgent {
		return consts.QueueCollectFresh
	}
	return consts.QueueCollectRealtime
}

func eventQueueIsHistory(queue string) bool {
	return queue == consts.QueueCollectHistory || queue == consts.QueueCollectUrgent || queue == consts.QueueCollectUrgentLive
}

func eventPendingTaskID(id int64) string {
	return fmt.Sprintf("collector-event-%d", id)
}

// Recovery must not manufacture another task for a live event already queued
// by the new producer. UUID legacy tasks remain safely protected by DB claims.
func eventLiveTaskExists(inspector *asynq.Inspector, eventID int64, priority int) (bool, error) {
	queue := eventQueueFor(priority, eventOriginIngress)
	if eventQueueIsHistory(queue) {
		return false, nil
	}
	queues := []string{queue}
	if queue == consts.QueueCollectFresh {
		// During rollout an old producer may still own this event in v2.
		// TaskID is queue-scoped, so inspect both before creating recovery work.
		queues = append(queues, consts.QueueCollectUrgentLive)
	}
	for _, candidate := range queues {
		info, err := inspector.GetTaskInfo(candidate, eventPendingTaskID(eventID))
		if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) ||
			(err != nil && strings.TrimPrefix(err.Error(), "asynq: UNKNOWN: ") == "ERR NOT FOUND") {
			continue
		}
		if err != nil {
			return false, gerror.Wrap(err, "检查实时采集任务是否仍可执行失败")
		}
		switch info.State {
		case asynq.TaskStatePending, asynq.TaskStateActive, asynq.TaskStateScheduled, asynq.TaskStateRetry:
			return true, nil
		}
	}
	return false, nil
}

// TaskID is queue-scoped and atomic. Unlike a short Unique TTL it cannot expire
// while work is still queued. Completed tasks have no retention: durable Event
// state owns the audit, and an explicit later replay must remain possible.
func confirmEventTaskConflict(inspector *asynq.Inspector, queue, taskID string) error {
	info, err := inspector.GetTaskInfo(queue, taskID)
	if err != nil {
		return gerror.Wrap(err, "确认采集任务去重状态失败")
	}
	switch info.State {
	case asynq.TaskStatePending, asynq.TaskStateActive, asynq.TaskStateScheduled, asynq.TaskStateRetry:
		return nil
	case asynq.TaskStateArchived:
		// A known recoverable Event must not be hidden behind an archived receipt.
		return inspector.RunTask(queue, taskID)
	default:
		return gerror.New("采集任务已不处于可执行状态，等待恢复重试")
	}
}

func collectorEventHistoryConcurrency(ctx context.Context) int {
	n := g.Cfg().MustGet(ctx, "telegramCollector.worker.historyConcurrency", 2).Int()
	// Zero pauses historical execution while realtime keeps its own pool.
	if n < 0 {
		return 0
	}
	return n
}
