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
	servers, err := inspector.Servers()
	if err != nil {
		g.Log().Warningf(ctx, "读取Asynq消费者状态失败：%+v", err)
		return
	}
	observeAsynqConsumerMetrics(ctx, servers)
	for _, queue := range queues {
		info, err := inspector.GetQueueInfo(queue)
		if err != nil {
			if errors.Is(err, asynq.ErrQueueNotFound) {
				// 队列尚未创建是正常情况，不应持续制造告警日志。
				recordAsynqQueueGauge(ctx, queue, "pending", 0)
				recordAsynqQueueGauge(ctx, queue, "active", 0)
				recordAsynqQueueGauge(ctx, queue, "scheduled", 0)
				recordAsynqQueueGauge(ctx, queue, "retry", 0)
				recordAsynqQueueGauge(ctx, queue, "archived", 0)
				recordAsynqQueueGauge(ctx, queue, "orphan_latency_seconds", 0)
				continue
			}
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
	observeQueuedJobsWithoutConsumer(ctx, servers)
	s.observeDatabaseJobsMissingAsynqTask(ctx, inspector)
	s.observeAsynqTasksWithTerminalDatabaseJob(ctx, inspector, queues)
}

func observeAsynqConsumerMetrics(ctx context.Context, servers []*asynq.ServerInfo) {
	serversGauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.asynq.consumer_servers")
	workersGauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.asynq.consumer_workers")
	queueConsumersGauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.asynq.queue_consumers")
	queueSet := make(map[string]struct{})
	workers := 0
	for _, server := range servers {
		if server == nil {
			continue
		}
		workers += len(server.ActiveWorkers)
		for queue := range server.Queues {
			queueSet[queue] = struct{}{}
		}
	}
	serversGauge.Record(ctx, int64(len(servers)))
	workersGauge.Record(ctx, int64(workers))
	queueConsumersGauge.Record(ctx, int64(len(queueSet)))
}

type asynqQueuedJobCount struct {
	QueueName string `json:"queue_name"`
	Count     int    `json:"count"`
}

func observeQueuedJobsWithoutConsumer(ctx context.Context, servers []*asynq.ServerInfo) {
	consumerQueues := make(map[string]struct{})
	for _, server := range servers {
		if server == nil {
			continue
		}
		for queue := range server.Queues {
			consumerQueues[queue] = struct{}{}
		}
	}
	var jobs []asynqQueuedJobCount
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("queue_name, COUNT(*) AS count").
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		WhereIn("status", []string{"pending", "failed_retry", "unknown", "sending"}).
		Group("queue_name").Scan(&jobs)
	if err != nil {
		g.Log().Warningf(ctx, "读取无消费者TG任务指标失败：%+v", err)
		return
	}
	withoutConsumer := 0
	withoutConsumerTasks := 0
	for _, job := range jobs {
		if _, ok := consumerQueues[job.QueueName]; ok {
			continue
		}
		withoutConsumer++
		withoutConsumerTasks += job.Count
	}
	queuesGauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.invariant.queued_jobs_without_consumer")
	tasksGauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.invariant.queue_pending_without_consumer")
	queuesGauge.Record(ctx, int64(withoutConsumer))
	tasksGauge.Record(ctx, int64(withoutConsumerTasks))
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
