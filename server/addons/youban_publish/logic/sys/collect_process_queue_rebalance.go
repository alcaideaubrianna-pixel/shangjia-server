package sys

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

type CollectProcessQueueRebalanceResult struct {
	TasksRemoved  int
	TasksRequeued int
	TasksSkipped  int
	ActiveTasks   int
}

func RebalanceCollectProcessQueue(ctx context.Context) (*CollectProcessQueueRebalanceResult, error) {
	result := new(CollectProcessQueueRebalanceResult)
	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer inspector.Close()

	existingQueues, err := inspector.Queues()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集处理队列失败")
	}
	existing := make(map[string]struct{}, len(existingQueues))
	for _, queue := range existingQueues {
		existing[queue] = struct{}{}
	}
	paused := make([]string, 0, len(collectProcessQueueNames()))
	for _, queue := range collectProcessQueueNames() {
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

	active := make(map[string]struct{})
	for _, queue := range collectProcessQueueNames() {
		if _, ok := existing[queue]; !ok {
			continue
		}
		tasks, listErr := listAllAsynqTasks(inspector.ListActiveTasks, queue)
		if listErr != nil {
			return nil, gerror.Wrapf(listErr, "读取活动采集处理任务失败 queue:%s", queue)
		}
		for _, task := range tasks {
			if payload, ok := collectProcessPayloadFromTaskInfo(task); ok {
				active[collectProcessScheduleKey(payload)] = struct{}{}
			}
		}
		result.ActiveTasks += len(tasks)
		for _, deleteAll := range []func(string) (int, error){
			inspector.DeleteAllPendingTasks,
			inspector.DeleteAllScheduledTasks,
			inspector.DeleteAllRetryTasks,
		} {
			count, deleteErr := deleteAll(queue)
			if deleteErr != nil && !errors.Is(deleteErr, asynq.ErrQueueNotFound) {
				return nil, gerror.Wrapf(deleteErr, "清理采集处理等待任务失败 queue:%s", queue)
			}
			result.TasksRemoved += count
		}
	}

	payloads, err := pendingCollectProcessPayloads(ctx)
	if err != nil {
		return nil, err
	}
	service := NewSysPublish()
	for _, payload := range payloads {
		key := collectProcessScheduleKey(payload)
		if _, ok := active[key]; ok {
			result.TasksSkipped++
			continue
		}
		removeCollectProcessSchedule(ctx, payload)
		enqueued, enqueueErr := service.enqueueCollectProcessTask(ctx, payload, 0, true)
		if enqueueErr != nil {
			return nil, gerror.Wrapf(enqueueErr, "重新投递采集处理任务失败 sourceId:%d chatId:%s", payload.SourceId, payload.SourceChatId)
		}
		if enqueued {
			result.TasksRequeued++
		} else {
			result.TasksSkipped++
		}
	}
	if service.tgQueueClient != nil {
		_ = service.tgQueueClient.Close()
	}
	g.Log().Infof(ctx, "采集处理队列重排完成 removed:%d requeued:%d skipped:%d active:%d",
		result.TasksRemoved, result.TasksRequeued, result.TasksSkipped, result.ActiveTasks)
	return result, nil
}

func pendingCollectProcessPayloads(ctx context.Context) ([]collectProcessQueuePayload, error) {
	eventColumns := pdao.YoubanPublishCollectEvent.Columns()
	sourceColumns := pdao.YoubanPublishCollectSource.Columns()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		As("e").
		InnerJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s."+sourceColumns.Id+"=e."+eventColumns.SourceId).
		Fields("e."+eventColumns.TenantId, "e."+eventColumns.AccountId, "e."+eventColumns.SourceId, "e."+eventColumns.SourceChatId).
		WhereIn("e."+eventColumns.Status, collectProcessWindowStatuses()).
		WhereNull("e."+eventColumns.ProcessedAt).
		Where("s."+sourceColumns.CollectEnabled, 1).
		Where("s."+sourceColumns.Status, 1).
		WhereNull("s."+sourceColumns.DeletedAt).
		Group("e."+eventColumns.TenantId, "e."+eventColumns.AccountId, "e."+eventColumns.SourceId, "e."+eventColumns.SourceChatId).
		OrderAsc("e." + eventColumns.TenantId).
		OrderAsc("e." + eventColumns.AccountId).
		OrderAsc("e." + eventColumns.SourceId).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待恢复采集处理分区失败")
	}
	payloads := make([]collectProcessQueuePayload, 0, len(rows))
	for _, row := range rows {
		payloads = append(payloads, collectProcessQueuePayload{
			TenantId:     row[eventColumns.TenantId].Int64(),
			AccountId:    row[eventColumns.AccountId].Int64(),
			SourceId:     row[eventColumns.SourceId].Int64(),
			SourceChatId: row[eventColumns.SourceChatId].String(),
		})
	}
	return payloads, nil
}

func collectProcessPayloadFromTaskInfo(task *asynq.TaskInfo) (collectProcessQueuePayload, bool) {
	if task == nil || task.Type != tgTaskTypeCollectProcess {
		return collectProcessQueuePayload{}, false
	}
	var payload collectProcessQueuePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil || payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 {
		return collectProcessQueuePayload{}, false
	}
	return payload, true
}
