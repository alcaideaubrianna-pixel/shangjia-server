package sys

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectObserveStaleMediaPendingAfter = 10 * time.Minute

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
			if isAsynqQueueNotFound(err) {
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
	s.observeCollectQueueInvariants(ctx, inspector)
}

func isAsynqQueueNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, asynq.ErrQueueNotFound) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not_found") && strings.Contains(message, "queue") ||
		strings.Contains(message, "queue") && strings.Contains(message, "does not exist")
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
	for _, queue := range telegramObserveQueueNames(ctx) {
		value := int64(0)
		if _, ok := queueSet[queue]; ok {
			value = 1
		}
		queueConsumersGauge.Record(ctx, value, metric.WithAttributes(attribute.String("queue", queue)))
	}
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
	set := map[string]struct{}{
		tgQueueNameAutoDelete:         {},
		tgQueueNameAttemptTimeout:     {},
		tgQueueNameBackground:         {},
		tgQueueNameCycle:              {},
		tgQueueNameCollectProcess:     {},
		tgQueueNameHistory:            {},
		tgQueueNameMediaProcess:       {},
		tgQueueNameMatting:            {},
		tgQueueNameProfileMaintenance: {},
	}
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
	for _, queue := range collectProcessQueueNames() {
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

func (s *sSysPublish) observeCollectQueueInvariants(ctx context.Context, inspector *asynq.Inspector) {
	healthGauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.observe_healthy")
	stats, err := loadCollectQueueDatabaseStats(ctx, collectObserveStaleMediaPendingAfter)
	if err != nil {
		healthGauge.Record(ctx, 0)
		g.Log().Warningf(ctx, "读取采集队列数据库监控指标失败：%+v", err)
		return
	}

	processTasks, err := countCollectProcessAsynqTasks(inspector)
	if err != nil {
		healthGauge.Record(ctx, 0)
		g.Log().Warningf(ctx, "读取采集处理队列任务指标失败：%+v", err)
		return
	}
	missingMediaTasks := 0
	if len(stats.StaleMediaEventIDs) > 0 {
		mediaTaskEventIDs, mediaErr := collectMediaAsynqTaskEventIDs(ctx, inspector)
		if mediaErr != nil {
			healthGauge.Record(ctx, 0)
			g.Log().Warningf(ctx, "读取采集媒体队列任务指标失败：%+v", mediaErr)
			return
		}
		for _, eventID := range stats.StaleMediaEventIDs {
			if _, ok := mediaTaskEventIDs[eventID]; !ok {
				missingMediaTasks++
			}
		}
	}
	recordCollectQueueDatabaseStats(ctx, stats, processTasks, missingMediaTasks)
	healthGauge.Record(ctx, 1)
}

type collectQueueDatabaseStats struct {
	PendingEvents           int
	PendingPartitions       int
	MediaPendingEvents      int
	StaleMediaPendingEvents int
	StaleMediaEventIDs      []int64
	OldestMediaPendingAt    time.Time
}

func loadCollectQueueDatabaseStats(ctx context.Context, staleAfter time.Duration) (collectQueueDatabaseStats, error) {
	var stats collectQueueDatabaseStats
	eventColumns := pdao.YoubanPublishCollectEvent.Columns()
	sourceColumns := pdao.YoubanPublishCollectSource.Columns()
	base := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		As("e").
		InnerJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s."+sourceColumns.Id+"=e."+eventColumns.SourceId).
		WhereIn("e."+eventColumns.Status, collectProcessWindowStatuses()).
		WhereNull("e."+eventColumns.ProcessedAt).
		Where("s."+sourceColumns.CollectEnabled, 1).
		Where("s."+sourceColumns.Status, 1).
		WhereNull("s." + sourceColumns.DeletedAt)
	pendingEvents, err := base.Clone().Count()
	if err != nil {
		return stats, err
	}
	partitionRows, err := base.Clone().
		Fields("e."+eventColumns.TenantId, "e."+eventColumns.AccountId, "e."+eventColumns.SourceId, "e."+eventColumns.SourceChatId).
		Group("e."+eventColumns.TenantId, "e."+eventColumns.AccountId, "e."+eventColumns.SourceId, "e."+eventColumns.SourceChatId).
		All()
	if err != nil {
		return stats, err
	}
	stats.PendingEvents = pendingEvents
	stats.PendingPartitions = len(partitionRows)

	mediaBase := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where(eventColumns.Status, sysin.CollectEventStatusMediaPending).
		WhereNull(eventColumns.ProcessedAt)
	mediaPending, err := mediaBase.Clone().Count()
	if err != nil {
		return stats, err
	}
	stats.MediaPendingEvents = mediaPending
	staleCutoff := time.Now().Add(-staleAfter)
	staleMediaPending, err := mediaBase.Clone().WhereLTE(eventColumns.UpdatedAt, staleCutoff).Count()
	if err != nil {
		return stats, err
	}
	stats.StaleMediaPendingEvents = staleMediaPending
	oldest, err := mediaBase.Clone().Fields("MIN(" + eventColumns.UpdatedAt + ") AS oldest_at").One()
	if err != nil {
		return stats, err
	}
	if !oldest.IsEmpty() && oldest["oldest_at"].GTime() != nil {
		stats.OldestMediaPendingAt = oldest["oldest_at"].GTime().Time
	}
	staleRows, err := mediaBase.Clone().
		Fields(eventColumns.Id).
		WhereLTE(eventColumns.UpdatedAt, staleCutoff).
		OrderAsc(eventColumns.UpdatedAt).
		Limit(5000).
		All()
	if err != nil {
		return stats, err
	}
	stats.StaleMediaEventIDs = make([]int64, 0, len(staleRows))
	for _, row := range staleRows {
		stats.StaleMediaEventIDs = append(stats.StaleMediaEventIDs, row[eventColumns.Id].Int64())
	}
	return stats, nil
}

func countCollectProcessAsynqTasks(inspector *asynq.Inspector) (int, error) {
	total := 0
	for _, queue := range collectProcessQueueNames() {
		info, err := inspector.GetQueueInfo(queue)
		if err != nil {
			if isAsynqQueueNotFound(err) {
				continue
			}
			return 0, err
		}
		total += info.Pending + info.Active + info.Scheduled + info.Retry
	}
	return total, nil
}

func collectMediaAsynqTaskEventIDs(ctx context.Context, inspector *asynq.Inspector) (map[int64]struct{}, error) {
	queues := collectMediaRealtimeWorkerQueues()
	for queue := range collectMediaBulkWorkerQueues(ctx) {
		queues[queue] = 1
	}
	eventIDs := make(map[int64]struct{})
	for queue := range queues {
		for _, list := range []func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error){
			inspector.ListPendingTasks,
			inspector.ListActiveTasks,
			inspector.ListScheduledTasks,
			inspector.ListRetryTasks,
		} {
			tasks, err := listAllAsynqTasks(list, queue)
			if err != nil {
				if isAsynqQueueNotFound(err) {
					continue
				}
				return nil, err
			}
			for _, task := range tasks {
				if payload, ok := collectMediaPayloadFromTaskInfo(task); ok {
					eventIDs[payload.EventId] = struct{}{}
				}
			}
		}
	}
	return eventIDs, nil
}

func recordCollectQueueDatabaseStats(ctx context.Context, stats collectQueueDatabaseStats, processTasks, missingMediaTasks int) {
	recordInt64Gauge(ctx, "xiaohuiji.collect.pending_events", stats.PendingEvents)
	recordInt64Gauge(ctx, "xiaohuiji.collect.pending_partitions", stats.PendingPartitions)
	recordInt64Gauge(ctx, "xiaohuiji.collect.process_queue_tasks", processTasks)
	recordFloat64Gauge(ctx, "xiaohuiji.collect.process_queue_amplification_ratio", collectQueueAmplificationRatio(processTasks, stats.PendingPartitions))
	recordInt64Gauge(ctx, "xiaohuiji.collect.media_pending_events", stats.MediaPendingEvents)
	recordInt64Gauge(ctx, "xiaohuiji.collect.media_pending_stale_events", stats.StaleMediaPendingEvents)
	recordInt64Gauge(ctx, "xiaohuiji.invariant.media_pending_missing_asynq_task", missingMediaTasks)
	oldestAge := 0
	if !stats.OldestMediaPendingAt.IsZero() {
		oldestAge = max(0, int(time.Since(stats.OldestMediaPendingAt).Seconds()))
	}
	recordInt64Gauge(ctx, "xiaohuiji.collect.media_pending_oldest_age_seconds", oldestAge)
}

func collectQueueAmplificationRatio(tasks, partitions int) float64 {
	if partitions <= 0 {
		if tasks > 0 {
			return float64(tasks)
		}
		return 0
	}
	return float64(tasks) / float64(partitions)
}

func recordInt64Gauge(ctx context.Context, name string, value int) {
	gauge, _ := publishObserveMeter.Int64Gauge(name)
	gauge.Record(ctx, int64(value))
}

func recordFloat64Gauge(ctx context.Context, name string, value float64) {
	gauge, _ := publishObserveMeter.Float64Gauge(name)
	gauge.Record(ctx, value)
}
