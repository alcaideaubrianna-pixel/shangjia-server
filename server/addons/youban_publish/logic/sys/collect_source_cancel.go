package sys

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectSourceCanceledMessage = "采集源已删除，未完成任务已取消"

func (s *sSysPublish) executeCollectSourceDeleteCleanup(ctx context.Context, sourceId, tenantId, accountId int64) error {
	now := gtime.Now()
	taskIDs, err := s.cancelCollectSourceHistoryTasks(ctx, sourceId, tenantId, accountId, now)
	if err != nil {
		return err
	}
	if err = s.cancelCollectSourceEvents(ctx, sourceId, tenantId, accountId, now); err != nil {
		return err
	}
	if err = s.clearCollectSourceAsynqTasks(ctx, sourceId, taskIDs); err != nil {
		return err
	}
	if err = clearCollectDedupeCacheForAccount(ctx, tenantId, accountId); err != nil {
		return gerror.Wrap(err, "清理采集源去重缓存失败")
	}
	g.Log().Infof(ctx, "采集源删除清理完成 sourceId:%d historyTasks:%d", sourceId, len(taskIDs))
	return nil
}

func (s *sSysPublish) cancelCollectSourceRuntime(ctx context.Context, sourceId int64, tenantId int64, accountId int64) error {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return gerror.New("采集源取消参数不完整")
	}
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取待删除采集源失败")
	}
	if source.IsEmpty() {
		return gerror.New("采集源不存在")
	}
	now := gtime.Now()
	if _, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", sourceId).
		Data(g.Map{"collect_enabled": 0, "updated_by": accountId, "updated_at": now}).
		Update(); err != nil {
		return gerror.Wrap(err, "停止待删除采集源失败")
	}
	taskIDs, err := s.cancelCollectSourceHistoryTasks(ctx, sourceId, tenantId, accountId, now)
	if err != nil {
		return err
	}
	if err = s.cancelCollectSourceEvents(ctx, sourceId, tenantId, accountId, now); err != nil {
		return err
	}
	if err = s.clearCollectSourceAsynqTasks(ctx, sourceId, taskIDs); err != nil {
		return err
	}
	s.refreshCollectSourceCache(ctx)
	s.refreshAccountCollectSupervisor()
	g.Log().Infof(ctx, "采集源未完成任务已取消 sourceId:%d historyTasks:%d", sourceId, len(taskIDs))
	return nil
}

func (s *sSysPublish) cancelCollectSourceHistoryTasks(ctx context.Context, sourceId int64, tenantId int64, accountId int64, now *gtime.Time) ([]int64, error) {
	rows, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Fields("id,tenant_id,account_id").
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", []string{
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			sysin.CollectHistoryTaskStatusPaused,
		}).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待取消历史采集任务失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ids, nil
	}
	if _, err = pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		WhereIn("id", ids).
		Data(g.Map{
			"status":        sysin.CollectHistoryTaskStatusCanceled,
			"error_message": collectSourceCanceledMessage,
			"next_run_at":   nil,
			"finished_at":   now,
			"updated_at":    now,
		}).Update(); err != nil {
		return nil, gerror.Wrap(err, "取消历史采集任务失败")
	}
	for _, taskId := range ids {
		s.appendCollectHistoryLog(ctx, taskId, tenantId, accountId, "info", "canceled", collectSourceCanceledMessage, nil)
	}
	return ids, nil
}

func (s *sSysPublish) cancelCollectSourceEvents(ctx context.Context, sourceId int64, tenantId int64, accountId int64, now *gtime.Time) error {
	eventTable := pdao.YoubanPublishCollectEvent.Table()
	reviewTable := pdao.YoubanPublishCollectReview.Table()
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("NOT EXISTS (SELECT 1 FROM "+reviewTable+" r WHERE r.event_id="+eventTable+".id AND r.tenant_id=? AND r.account_id=? AND r.status=?)",
			tenantId, accountId, sysin.CollectReviewStatusPending).
		WhereIn("status", []string{
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusGroupCollect,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked,
			sysin.CollectEventStatusMediaPending,
			sysin.CollectEventStatusMediaReady,
			sysin.CollectEventStatusFailed,
		}).
		Data(g.Map{
			"status":        sysin.CollectEventStatusIgnored,
			"error_message": collectSourceCanceledMessage,
			"processed_at":  now,
			"updated_at":    now,
		}).Update()
	return gerror.Wrap(err, "取消采集源未完成事件失败")
}

func (s *sSysPublish) clearCollectSourceAsynqTasks(ctx context.Context, sourceId int64, historyTaskIDs []int64) error {
	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer func() { _ = inspector.Close() }()
	if err := clearCollectHistoryAsynqTasks(inspector, historyTaskIDs); err != nil {
		return err
	}
	if err := clearCollectSourceTasks(inspector, sourceId); err != nil {
		return err
	}
	return nil
}

func clearCollectSourceTasks(inspector *asynq.Inspector, sourceId int64) error {
	if inspector == nil || sourceId <= 0 {
		return nil
	}
	type taskListFunc func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	lists := []struct {
		active bool
		list   taskListFunc
	}{
		{list: inspector.ListPendingTasks},
		{list: inspector.ListScheduledTasks},
		{list: inspector.ListRetryTasks},
		{list: inspector.ListArchivedTasks},
		{active: true, list: inspector.ListActiveTasks},
	}
	queues := []struct {
		name  string
		label string
	}{
		{name: tgQueueNameBackground, label: "采集处理"},
		{name: tgQueueNameMediaRealtime, label: "实时媒体缓存"},
		{name: tgQueueNameMedia, label: "旧媒体缓存"},
	}
	for _, name := range collectMediaBulkQueueNames() {
		queues = append(queues, struct {
			name  string
			label string
		}{name: name, label: "历史媒体缓存"})
	}
	for _, queue := range queues {
		for _, item := range lists {
			matched, err := listCollectSourceTasks(item.list, queue.name, sourceId)
			if err != nil && !errors.Is(err, asynq.ErrQueueNotFound) {
				return gerror.Wrapf(err, "读取%s队列失败", queue.label)
			}
			if err == nil {
				if err = deleteCollectSourceTasks(inspector, matched, item.active); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func listCollectSourceTasks(list func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error), queue string, sourceId int64) ([]*asynq.TaskInfo, error) {
	const pageSize = 1000
	matched := make([]*asynq.TaskInfo, 0)
	for page := 1; ; page++ {
		tasks, err := list(queue, asynq.Page(page), asynq.PageSize(pageSize))
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if task == nil || (task.Type != tgTaskTypeCollectProcess && task.Type != tgTaskTypeCollectMedia) {
				continue
			}
			var payload struct {
				SourceId int64 `json:"sourceId"`
			}
			if json.Unmarshal(task.Payload, &payload) == nil && payload.SourceId == sourceId {
				matched = append(matched, task)
			}
		}
		if len(tasks) < pageSize {
			return matched, nil
		}
	}
}

func deleteCollectSourceTasks(inspector *asynq.Inspector, tasks []*asynq.TaskInfo, active bool) error {
	for _, task := range tasks {
		if task == nil || (task.Type != tgTaskTypeCollectProcess && task.Type != tgTaskTypeCollectMedia) {
			continue
		}
		if active {
			if err := inspector.CancelProcessing(task.ID); err != nil && !errors.Is(err, asynq.ErrTaskNotFound) {
				return gerror.Wrap(err, "取消采集运行中任务失败")
			}
			continue
		}
		if err := inspector.DeleteTask(task.Queue, task.ID); err != nil && !errors.Is(err, asynq.ErrTaskNotFound) {
			return gerror.Wrap(err, "删除采集排队任务失败")
		}
	}
	return nil
}

func clearCollectHistoryAsynqTasks(inspector *asynq.Inspector, taskIDs []int64) error {
	if inspector == nil || len(taskIDs) == 0 {
		return nil
	}
	taskSet := make(map[int64]struct{}, len(taskIDs))
	for _, taskId := range taskIDs {
		if taskId > 0 {
			taskSet[taskId] = struct{}{}
		}
	}
	type taskListFunc func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	lists := []struct {
		active bool
		list   taskListFunc
	}{
		{list: inspector.ListPendingTasks},
		{list: inspector.ListScheduledTasks},
		{list: inspector.ListRetryTasks},
		{list: inspector.ListArchivedTasks},
		{active: true, list: inspector.ListActiveTasks},
	}
	for _, item := range lists {
		tasks, err := item.list(tgQueueNameBackground, asynq.PageSize(1000))
		if err != nil {
			if errors.Is(err, asynq.ErrQueueNotFound) {
				continue
			}
			return gerror.Wrap(err, "读取历史采集队列失败")
		}
		for _, task := range tasks {
			if task == nil || task.Type != tgTaskTypeCollectHistory {
				continue
			}
			var payload collectHistoryQueuePayload
			if json.Unmarshal(task.Payload, &payload) != nil {
				continue
			}
			if _, ok := taskSet[payload.TaskId]; !ok {
				continue
			}
			if item.active {
				if err = inspector.CancelProcessing(task.ID); err != nil {
					return gerror.Wrap(err, "取消运行中的历史采集任务失败")
				}
				continue
			}
			if err = inspector.DeleteTask(task.Queue, task.ID); err != nil && !errors.Is(err, asynq.ErrTaskNotFound) {
				return gerror.Wrap(err, "删除历史采集排队任务失败")
			}
		}
	}
	return nil
}
