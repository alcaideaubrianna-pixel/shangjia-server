package sys

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func (s *sSysPublish) runAsynqObserveMetrics(ctx context.Context) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.refreshAsynqObserveMetrics(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *sSysPublish) refreshAsynqObserveMetrics(ctx context.Context) {
	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer inspector.Close()
	queues := telegramObserveQueueNames(ctx)
	for _, queue := range queues {
		info, err := inspector.GetQueueInfo(queue)
		if err != nil {
			g.Log().Warningf(ctx, "读取Asynq队列指标失败 queue:%s err:%+v", queue, err)
			continue
		}
		recordAsynqQueueGauge(ctx, queue, "pending", info.Pending)
		recordAsynqQueueGauge(ctx, queue, "active", info.Active)
		recordAsynqQueueGauge(ctx, queue, "scheduled", info.Scheduled)
		recordAsynqQueueGauge(ctx, queue, "retry", info.Retry)
		recordAsynqQueueGauge(ctx, queue, "archived", info.Archived)
		recordAsynqQueueGauge(ctx, queue, "orphan_latency_seconds", int(info.Latency.Seconds()))
	}
	s.observeDatabaseJobsMissingAsynqTask(ctx, inspector)
	s.observeAsynqTasksWithTerminalDatabaseJob(ctx, inspector, queues)
}

func telegramObserveQueueNames(ctx context.Context) []string {
	set := map[string]struct{}{tgQueueNameBackground: {}, tgQueueNameHistory: {}}
	for queue := range telegramPublishForegroundQueueWeights(ctx) {
		set[queue] = struct{}{}
	}
	for queue := range telegramPublishBulkQueueWeights(ctx) {
		set[queue] = struct{}{}
	}
	for queue := range collectMediaRealtimeWorkerQueues() {
		set[queue] = struct{}{}
	}
	for queue := range collectMediaBulkWorkerQueues(ctx) {
		set[queue] = struct{}{}
	}
	queues := make([]string, 0, len(set))
	for queue := range set {
		queues = append(queues, queue)
	}
	sort.Strings(queues)
	return queues
}

func recordAsynqQueueGauge(ctx context.Context, queue, state string, value int) {
	gauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.asynq.tasks")
	gauge.Record(ctx, int64(value), metric.WithAttributes(attribute.String("queue", queue), attribute.String("state", state)))
}

func (s *sSysPublish) observeDatabaseJobsMissingAsynqTask(ctx context.Context, inspector *asynq.Inspector) {
	var jobs []telegramJobRecord
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		WhereIn("status", []string{"pending", "failed_retry", "unknown", "sending"}).
		WhereNot("asynq_task_id", "").OrderAsc("updated_at").Limit(300).Scan(&jobs); err != nil {
		return
	}
	missing := 0
	for _, job := range jobs {
		if _, err := inspector.GetTaskInfo(job.QueueName, job.AsynqTaskId); errors.Is(err, asynq.ErrTaskNotFound) {
			missing++
		}
	}
	gauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.invariant.db_job_missing_asynq_task")
	gauge.Record(ctx, int64(missing))
}

func (s *sSysPublish) observeAsynqTasksWithTerminalDatabaseJob(ctx context.Context, inspector *asynq.Inspector, queues []string) {
	terminal := 0
	for _, queue := range queues {
		tasks, err := inspector.ListPendingTasks(queue, asynq.PageSize(100))
		if err != nil {
			continue
		}
		active, _ := inspector.ListActiveTasks(queue, asynq.PageSize(100))
		tasks = append(tasks, active...)
		for _, task := range tasks {
			if task.Type != tgTaskTypePublish {
				continue
			}
			payload, err := decodeTelegramQueuePayload(asynq.NewTask(task.Type, task.Payload))
			if err != nil {
				continue
			}
			status, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Fields("status").Where("id", payload.JobId).Value()
			if err == nil && isTerminalTelegramJobStatus(status.String()) {
				terminal++
			}
		}
	}
	gauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.invariant.asynq_task_terminal_db_job")
	gauge.Record(ctx, int64(terminal))
}

func isTerminalTelegramJobStatus(status string) bool {
	switch status {
	case "sent", "failed", "superseded":
		return true
	default:
		return false
	}
}
