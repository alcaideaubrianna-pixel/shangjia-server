package sys

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type CollectMediaQueueRebalanceResult struct {
	ArchivedDeleted int
	TasksRemoved    int
	TasksRequeued   int
	TasksSkipped    int
	ActiveTasks     int
}

func RebalanceCollectMediaQueue(ctx context.Context) (*CollectMediaQueueRebalanceResult, error) {
	result := new(CollectMediaQueueRebalanceResult)
	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer inspector.Close()
	client := asynq.NewClient(telegramQueueRedisOpt(ctx))
	defer client.Close()

	queues := append([]string{tgQueueNameMedia, tgQueueNameMediaRealtime}, collectMediaBulkQueueNames()...)
	existingQueues, err := inspector.Queues()
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体队列失败")
	}
	existing := make(map[string]struct{}, len(existingQueues))
	for _, queue := range existingQueues {
		existing[queue] = struct{}{}
	}
	paused := make([]string, 0, len(queues))
	for _, queue := range queues {
		if _, ok := existing[queue]; !ok {
			continue
		}
		if pauseErr := inspector.PauseQueue(queue); pauseErr == nil {
			paused = append(paused, queue)
		}
	}
	defer func() {
		for _, queue := range paused {
			_ = inspector.UnpauseQueue(queue)
		}
	}()

	queuedEventIDs := make(map[int64]struct{})
	activeEventIDs := make(map[int64]struct{})
	for _, queue := range queues {
		if _, ok := existing[queue]; !ok {
			continue
		}
		activeTasks, listErr := listAllAsynqTasks(inspector.ListActiveTasks, queue)
		if listErr != nil {
			return nil, gerror.Wrapf(listErr, "读取活动媒体任务失败 queue:%s", queue)
		}
		for _, task := range activeTasks {
			if payload, ok := collectMediaPayloadFromTaskInfo(task); ok {
				activeEventIDs[payload.EventId] = struct{}{}
			}
		}
		result.ActiveTasks += len(activeTasks)

		for _, list := range []func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error){
			inspector.ListPendingTasks,
			inspector.ListScheduledTasks,
			inspector.ListRetryTasks,
		} {
			tasks, listErr := listAllAsynqTasks(list, queue)
			if listErr != nil {
				return nil, gerror.Wrapf(listErr, "读取待重排媒体任务失败 queue:%s", queue)
			}
			for _, task := range tasks {
				if payload, ok := collectMediaPayloadFromTaskInfo(task); ok {
					if _, active := activeEventIDs[payload.EventId]; !active {
						queuedEventIDs[payload.EventId] = struct{}{}
					}
				}
			}
		}

		deleted, deleteErr := deleteCollectMediaQueueWaitingTasks(inspector, queue)
		if deleteErr != nil {
			return nil, deleteErr
		}
		result.TasksRemoved += deleted
		archived, archiveErr := inspector.DeleteAllArchivedTasks(queue)
		if archiveErr != nil && !errors.Is(archiveErr, asynq.ErrQueueNotFound) {
			return nil, gerror.Wrapf(archiveErr, "清理媒体归档任务失败 queue:%s", queue)
		}
		result.ArchivedDeleted += archived
		_, _ = inspector.DeleteAllCompletedTasks(queue)
	}

	payloads, err := collectMediaRebalancePayloads(ctx, queuedEventIDs)
	if err != nil {
		return nil, err
	}
	result.TasksSkipped = len(queuedEventIDs) - len(payloads)
	for index, payload := range fairCollectMediaQueuePayloads(payloads) {
		body, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, marshalErr
		}
		_, enqueueErr := client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectMedia, body),
			asynq.Queue(collectMediaQueueName(ctx, payload)),
			asynq.Unique(collectMediaTaskUniqueTTL),
			asynq.MaxRetry(10),
			asynq.Timeout(30*time.Minute),
		)
		if enqueueErr != nil && !errors.Is(enqueueErr, asynq.ErrDuplicateTask) && !errors.Is(enqueueErr, asynq.ErrTaskIDConflict) {
			return nil, gerror.Wrapf(enqueueErr, "重新投递媒体任务失败 eventId:%d", payload.EventId)
		}
		if enqueueErr == nil {
			result.TasksRequeued++
		}
		if index > 0 && index%200 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	g.Log().Infof(ctx, "采集媒体队列重排完成 archived:%d removed:%d requeued:%d skipped:%d active:%d",
		result.ArchivedDeleted, result.TasksRemoved, result.TasksRequeued, result.TasksSkipped, result.ActiveTasks)
	return result, nil
}

func collectMediaRebalancePayloads(ctx context.Context, eventIDs map[int64]struct{}) ([]collectMediaQueuePayload, error) {
	ids := make([]int64, 0, len(eventIDs))
	for eventID := range eventIDs {
		ids = append(ids, eventID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	payloads := make([]collectMediaQueuePayload, 0, len(ids))
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	for start := 0; start < len(ids); start += 200 {
		end := start + 200
		if end > len(ids) {
			end = len(ids)
		}
		rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			WhereIn(eventCols.Id, ids[start:end]).
			Where(eventCols.Status, sysin.CollectEventStatusMediaPending).
			WhereNull(eventCols.ProcessedAt).
			OrderAsc(eventCols.ReceivedAt).
			OrderAsc(eventCols.Id).
			All()
		if err != nil {
			return nil, gerror.Wrap(err, "读取待重排媒体事件失败")
		}
		for _, row := range rows {
			payloads = append(payloads, collectMediaQueuePayloadFromEvent(row))
		}
	}
	return payloads, nil
}

func fairCollectMediaQueuePayloads(payloads []collectMediaQueuePayload) []collectMediaQueuePayload {
	realtime := make([]collectMediaQueuePayload, 0)
	accountOrder := make([]string, 0)
	bulkByAccount := make(map[string][]collectMediaQueuePayload)
	for _, payload := range payloads {
		if !payload.Bulk {
			realtime = append(realtime, payload)
			continue
		}
		key := collectMediaAccountKey(payload.TenantId, payload.TgAccountId)
		if _, exists := bulkByAccount[key]; !exists {
			accountOrder = append(accountOrder, key)
		}
		bulkByAccount[key] = append(bulkByAccount[key], payload)
	}
	result := make([]collectMediaQueuePayload, 0, len(payloads))
	result = append(result, realtime...)
	for len(result) < len(payloads) {
		progressed := false
		for _, key := range accountOrder {
			items := bulkByAccount[key]
			if len(items) == 0 {
				continue
			}
			result = append(result, items[0])
			bulkByAccount[key] = items[1:]
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return result
}

func collectMediaPayloadFromTaskInfo(task *asynq.TaskInfo) (collectMediaQueuePayload, bool) {
	if task == nil || task.Type != tgTaskTypeCollectMedia {
		return collectMediaQueuePayload{}, false
	}
	var payload collectMediaQueuePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil || payload.EventId <= 0 {
		return collectMediaQueuePayload{}, false
	}
	return payload, true
}

func listAllAsynqTasks(list func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error), queue string) ([]*asynq.TaskInfo, error) {
	const pageSize = 1000
	result := make([]*asynq.TaskInfo, 0)
	for page := 1; ; page++ {
		tasks, err := list(queue, asynq.Page(page), asynq.PageSize(pageSize))
		if err != nil {
			return nil, err
		}
		result = append(result, tasks...)
		if len(tasks) < pageSize {
			return result, nil
		}
	}
}

func deleteCollectMediaQueueWaitingTasks(inspector *asynq.Inspector, queue string) (int, error) {
	total := 0
	for _, deleteAll := range []func(string) (int, error){
		inspector.DeleteAllPendingTasks,
		inspector.DeleteAllScheduledTasks,
		inspector.DeleteAllRetryTasks,
	} {
		count, err := deleteAll(queue)
		if err != nil && !errors.Is(err, asynq.ErrQueueNotFound) {
			return total, gerror.Wrapf(err, "清理媒体待执行任务失败 queue:%s", queue)
		}
		total += count
	}
	return total, nil
}
