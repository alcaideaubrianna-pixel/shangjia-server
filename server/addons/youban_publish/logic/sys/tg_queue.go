package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
)

const (
	// tgQueueNameMediaLegacy is read only by the one-time migration. It is
	// intentionally not registered by any worker.
	tgQueueNameMediaLegacy         = "youban_publish_media"
	tgQueueNameInteractive         = "youban_publish_tg_interactive"
	tgQueueNameUrgent              = "youban_publish_tg_urgent"
	tgQueueNameDefault             = "youban_publish_tg"
	tgQueueNameBulk                = "youban_publish_tg_bulk"
	tgQueueNameCleanup             = "youban_publish_tg_cleanup"
	tgQueueNameMediaRealtime       = "youban_publish_media_realtime"
	tgQueueNameMediaRealtimePrefix = "youban_publish_media_realtime_"
	tgQueueNameMediaProcess        = "youban_publish_media_process"
	tgQueueNameMediaBulkPrefix     = "youban_publish_media_bulk_"
	// Keep realtime deletion isolated from explicitly requested historical work.
	tgQueueNameAutoDeleteRealtime                 = "youban_publish_auto_delete_realtime"
	tgQueueNameAutoDeleteBackfill                 = "youban_publish_auto_delete_backfill"
	tgQueueNameAttemptTimeout                     = "youban_publish_attempt_timeout"
	tgQueueNameBackground                         = "youban_publish_background"
	tgQueueNameAccountSchedule                    = "youban_publish_account_schedule"
	tgQueueNameMaterialImportPull                 = "youban_publish_material_import_pull"
	tgQueueNameMaterialImportMedia                = "youban_publish_material_import_media"
	tgQueueNameCycle                              = "youban_publish_cycle"
	tgQueueNameCollectProcessRealtime             = "youban_publish_collect_process_realtime"
	tgQueueNameCollectProcess                     = "youban_publish_collect_process"
	tgQueueNameCollectProcessPrefix               = "youban_publish_collect_process_"
	tgQueueNameCollectProcessAccountPrefix        = "youban_publish_collect_account_realtime_"
	tgQueueNameCollectProcessAccountHistoryPrefix = "youban_publish_collect_account_history_"
	tgQueueNameHistory                            = "youban_publish_history"
	tgQueueNameProfileMaintenance                 = "youban_publish_profile_maintenance"
	tgQueueNameDuplicateScan                      = "youban_publish_duplicate_scan"
	tgQueueNameMatting                            = "youban_publish_matting"
	tgTaskTypePublish                             = "youban_publish:tg:publish"
	tgTaskTypeCleanup                             = "youban_publish:tg:cleanup"
	tgTaskTypeAutoDelete                          = "youban_publish:tg:auto_delete"
	tgTaskTypeAutoDeleteBackfill                  = "youban_publish:tg:auto_delete_backfill"
	tgTaskTypeImport                              = "youban_publish:import:legacy"
	tgTaskTypeRepair                              = "youban_publish:tg:message_repair"
	tgTaskTypeImportMatch                         = "youban_publish:import:tg_match"
	tgTaskTypeImportSync                          = "youban_publish:import:tg_sync"
	tgTaskTypeDown                                = "youban_publish:profile:down"
	tgTaskTypeCycleRun                            = "youban_publish:cycle:run"
	tgTaskTypeCycleReschedule                     = "youban_publish:cycle:reschedule"
	tgTaskTypeCycleRefresh                        = "youban_publish:cycle:refresh"
	tgTaskTypeCollectMedia                        = "youban_publish:collect:media_cache"
	tgTaskTypeMediaProcess                        = "youban_publish:media:process"
	tgTaskTypeCollectProcess                      = "youban_publish:collect:process"
	tgTaskTypeCollectHistory                      = "youban_publish:collect:history"
	tgTaskTypeCollectTrigger                      = "youban_publish:collect:trigger"
	tgTaskTypeCollectSourceDelete                 = "youban_publish:collect:source_delete"
	tgTaskTypeChannelMemberSync                   = "youban_publish:tg:channel_member_sync"
	tgTaskTypeBotMediaRepair                      = "youban_publish:bot:media_repair"
	tgTaskTypeProfileMaintenance                  = "youban_publish:profile:maintenance"
	tgTaskTypeProfileSubmit                       = "youban_publish:profile:submit"
	tgTaskTypePublishRecovery                     = "youban_publish:profile:publish_recovery"
	tgTaskTypeDuplicateScan                       = "youban_publish:profile:duplicate_scan"
	tgTaskTypeMatting                             = "youban_publish:anti_scan:matting"
	tgTaskTypeAttemptTimeout                      = "youban_publish:tg:attempt_timeout"
	tgTaskTypeRelayMedia                          = "youban_publish:relay:media"
	tgTaskTypeRelayMediaGroup                     = "youban_publish:relay:media_group"
	tgQueueNameRelay                              = "youban_publish_relay"
)

const collectMediaMaxBulkQueueShards = 16

const collectProcessQueueShards = 16

// 采集处理按账户固定分片，确保一个账户的积压不能占满其他账户的采集消费者。
const collectProcessAccountQueueShards = collectProcessQueueShards

const (
	tgQueueNameCollectProcessRealtimeV2 = "youban_publish_collect_process_realtime_v2"
	tgQueueNameCollectProcessRecoveryV2 = "youban_publish_collect_process_recovery_v2"
)

func collectProcessQueueName(payload collectProcessQueuePayload) string {
	if payload.TenantId > 0 && payload.AccountId > 0 && (payload.EventId == 0 || payload.OutboxOwner != "") {
		return collectProcessAccountQueueName(payload)
	}
	if payload.EventId > 0 {
		if payload.OutboxOwner != "" {
			if payload.Recovery || payload.History {
				return tgQueueNameCollectProcessRecoveryV2
			}
			return tgQueueNameCollectProcessRealtimeV2
		}
		return tgQueueNameCollectProcessRealtime
	}
	// 兼容没有账户归属的历史维护任务，继续使用来源分片。
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(fmt.Sprintf("%d:%d", payload.TenantId, payload.AccountId)))
	return fmt.Sprintf("%s%02d", tgQueueNameCollectProcessPrefix, hash.Sum32()%collectProcessQueueShards)
}

// collectProcessAccountQueueName 返回稳定的账户采集分片，不按消息或规则分片，保留同账户公平调度。
func collectProcessAccountQueueName(payload collectProcessQueuePayload) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(fmt.Sprintf("%d:%d", payload.TenantId, payload.AccountId)))
	prefix := tgQueueNameCollectProcessAccountPrefix
	if payload.History || payload.Recovery {
		prefix = tgQueueNameCollectProcessAccountHistoryPrefix
	}
	return fmt.Sprintf("%s%02d", prefix, hash.Sum32()%collectProcessAccountQueueShards)
}

func collectProcessQueueNames() []string {
	queues := make([]string, 0, collectProcessQueueShards+1)
	queues = append(queues, tgQueueNameCollectProcess)
	for shard := 0; shard < collectProcessQueueShards; shard++ {
		queues = append(queues, fmt.Sprintf("%s%02d", tgQueueNameCollectProcessPrefix, shard))
	}
	return queues
}

func collectProcessWorkerQueues() map[string]int {
	queues := make(map[string]int, collectProcessAccountQueueShards*2+3)
	for _, queue := range collectProcessQueueNames() {
		queues[queue] = 1
	}
	queues[tgQueueNameCollectProcessRealtime] = 1
	queues[tgQueueNameCollectProcessRecoveryV2] = 4
	for shard := 0; shard < collectProcessAccountQueueShards; shard++ {
		queues[fmt.Sprintf("%s%02d", tgQueueNameCollectProcessAccountPrefix, shard)] = 1
		queues[fmt.Sprintf("%s%02d", tgQueueNameCollectProcessAccountHistoryPrefix, shard)] = 1
	}
	return queues
}

func backgroundWorkerQueues(collectorWorker, backgroundWorker bool) map[string]int {
	queues := make(map[string]int)
	if collectorWorker {
		for queue, weight := range collectProcessWorkerQueues() {
			queues[queue] = weight
		}
	}
	if backgroundWorker {
		queues[tgQueueNameBackground] = 1
	}
	return queues
}

// A display send and a verification video can each consume the full media
// request timeout. Keep the task deadline above their combined budget.
const telegramPublishTaskTimeout = 25 * time.Minute

func collectMediaBulkQueueName(shard int) string {
	if shard < 0 {
		shard = -shard
	}
	return fmt.Sprintf("%s%d", tgQueueNameMediaBulkPrefix, shard%collectMediaMaxBulkQueueShards)
}

func collectMediaBulkQueueNames() []string {
	queues := make([]string, 0, collectMediaMaxBulkQueueShards)
	for shard := 0; shard < collectMediaMaxBulkQueueShards; shard++ {
		queues = append(queues, collectMediaBulkQueueName(shard))
	}
	return queues
}

func collectMediaRealtimeQueueName(shard int) string {
	if shard < 0 {
		shard = -shard
	}
	return fmt.Sprintf("%s%d", tgQueueNameMediaRealtimePrefix, shard%collectMediaMaxBulkQueueShards)
}

func collectMediaRealtimeQueueNames() []string {
	queues := make([]string, 0, collectMediaMaxBulkQueueShards)
	for shard := 0; shard < collectMediaMaxBulkQueueShards; shard++ {
		queues = append(queues, collectMediaRealtimeQueueName(shard))
	}
	return queues
}

const (
	tgJobPriorityInteractive = 1
	tgJobPriorityUrgent      = 10
	tgJobPriorityDefault     = 50
	tgJobPriorityFullPush    = 70
	tgJobPriorityBulk        = 90
)

type tgQueuePayload struct {
	JobId int64 `json:"jobId"`
}

type tgAttemptQueuePayload struct {
	AttemptId int64 `json:"attemptId"`
}

type autoDeleteQueuePayload struct {
	BotId     int64  `json:"botId"`
	TenantId  int64  `json:"tenantId"`
	AccountId int64  `json:"accountId"`
	ChannelId int64  `json:"channelId"`
	ChatId    string `json:"chatId"`
	MessageId int    `json:"messageId"`
	Keyword   string `json:"keyword"`
}

type autoDeleteBackfillQueuePayload struct {
	CursorId    int64  `json:"cursorId"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt"`
	Scanned     int    `json:"scanned"`
	MaxMessages int    `json:"maxMessages"`
}

type mediaProcessQueuePayload struct {
	MediaId            int64 `json:"mediaId"`
	RetryPolicyVersion int   `json:"retryPolicyVersion,omitempty"`
}

type botMediaRepairQueuePayload struct {
	ProfileId int64 `json:"profileId"`
	JobId     int64 `json:"jobId,omitempty"`
}

type importQueuePayload struct {
	Id    int64 `json:"id"`
	RunId int64 `json:"runId"`
}

type tgMessageRepairQueuePayload struct {
	RunId int64 `json:"runId"`
}

type importMatchQueuePayload struct {
	MatchRunId int64 `json:"matchRunId"`
}

type profileDownQueuePayload struct {
	TenantId    int64   `json:"tenantId"`
	ProfileIds  []int64 `json:"profileIds"`
	DownAt      string  `json:"downAt"`
	OperationNo string  `json:"operationNo"`
}

type cycleRunQueuePayload struct {
	RunId int64 `json:"runId"`
}

type cycleRescheduleQueuePayload struct {
	ChannelId int64 `json:"channelId"`
}

type cycleRefreshQueuePayload struct {
	ChannelId int64 `json:"channelId"`
}

type collectHistoryQueuePayload struct {
	TaskId int64 `json:"taskId"`
}

type collectTriggerQueuePayload struct {
	AccountId int64 `json:"accountId"`
	SourceId  int64 `json:"sourceId"`
	TenantId  int64 `json:"tenantId"`
}

type collectSourceDeleteQueuePayload struct {
	AccountId int64 `json:"accountId"`
	SourceId  int64 `json:"sourceId"`
	TenantId  int64 `json:"tenantId"`
}

type tgRetryAfterError struct {
	after time.Duration
	err   error
}

func (e *tgRetryAfterError) Error() string {
	return e.err.Error()
}

func (e *tgRetryAfterError) Unwrap() error {
	return e.err
}

func (s *sSysPublish) enqueueTelegramJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramJobDirect(ctx, jobId, delay)
}

func (s *sSysPublish) enqueueTelegramJobDirect(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramJobDirectWithUnique(ctx, jobId, delay, true)
}

func (s *sSysPublish) enqueueTelegramJobDirectWithUnique(ctx context.Context, jobId int64, delay time.Duration, unique bool) (resultErr error) {
	ctx = telegramPublishTraceContext(ctx, jobId)
	stage := beginTelegramPublishStage(ctx, "dispatch.enqueue", g.Map{"delayMs": delay.Milliseconds(), "unique": unique})
	defer stage.Finish(&resultErr)
	if jobId <= 0 {
		return nil
	}
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	if paused, pauseErr := s.postponePermissionPausedJob(ctx, job); pauseErr != nil || paused {
		return pauseErr
	}
	step := beginTelegramPublishStage(ctx, "dispatch.account_availability")
	tgAccountId, available, checkErr := s.telegramJobAccountCanPublish(ctx, job)
	step.End(checkErr, g.Map{"tgAccountId": tgAccountId, "available": available})
	if checkErr != nil {
		if errors.Is(checkErr, errTelegramPublishAccountMissing) {
			return s.failTelegramMissingAccountJob(ctx, job, checkErr)
		}
		return checkErr
	} else if !available {
		return s.pauseTelegramJobsForUnavailableAccount(ctx, tgAccountId, "TG账号当前未授权或会话不可用")
	}
	if delay <= 0 && !telegramJobBypassesAutomaticSchedule(job) {
		if windowDelay, enabled := s.telegramPublishWindowDelay(ctx, job.TenantId, job.AccountId); enabled && windowDelay > 0 {
			delay = windowDelay
		}
	}
	if active, checkErr := s.telegramJobChannelIsActive(ctx, job); checkErr != nil {
		return checkErr
	} else if !active {
		message := "目标频道已删除或禁用，任务自动终止"
		if isMessagePushOperationNo(job.OperationNo) {
			message = "快速推送目标已失效，任务自动终止"
		}
		_, updateErr := s.transitionTelegramJob(ctx, telegramJobTransition{
			JobId: jobId,
			From: []string{
				telegramJobStatusPending, telegramJobStatusRetryWait,
				telegramJobStatusConfirming, telegramJobStatusSending,
			},
			To:           telegramJobStatusSuperseded,
			ErrorMessage: message,
			Additional:   g.Map{"last_dispatch_error": message},
		})
		if updateErr != nil {
			return gerror.Wrap(updateErr, "终止无效频道TG任务失败")
		}
		if recordErr := s.upsertPublishJobRecord(ctx, job, "superseded", message); recordErr != nil {
			g.Log().Warningf(ctx, "更新无效频道发布记录失败 jobId:%d err:%+v", job.Id, recordErr)
		}
		return nil
	}
	priority := s.telegramJobPriority(job)
	queueName := telegramQueueNameByPriorityAndChannel(ctx, priority, job.ChannelId)
	// 新引擎统一执行队列，频道顺序和优先级仍由入队前的频道调度保证。
	// 旧引擎保留原队列，便于按账户灰度和迁移回滚。
	queueName = queueNameForPushEngine(s.accountPushEngine(ctx, job.TenantId, job.AccountId), queueName)
	data := g.Map{
		"priority":            priority,
		"queue_name":          queueName,
		"dispatch_status":     tgDispatchStatusQueued,
		"dispatched_at":       gtime.Now(),
		"dispatch_count":      gdb.Raw("dispatch_count + 1"),
		"last_dispatch_error": "",
		"updated_at":          gtime.Now(),
	}
	if delay > 0 {
		data["next_retry_at"] = gtime.Now().Add(delay)
	} else {
		data["next_retry_at"] = nil
	}
	reserveReason := ""
	reserved, err := s.withTelegramChannelDispatchLease(ctx, job, func(reserveCtx context.Context) (bool, error) {
		step := beginTelegramPublishStage(reserveCtx, "dispatch.channel_order")
		hasEarlier, orderErr := s.telegramChannelHasEarlierActiveJob(reserveCtx, job)
		step.End(orderErr, g.Map{"hasEarlier": hasEarlier})
		if orderErr != nil || hasEarlier {
			if hasEarlier {
				reserveReason = "earlier_job"
			}
			return false, orderErr
		}
		step = beginTelegramPublishStage(reserveCtx, "dispatch.channel_capacity")
		hasCapacity, capacityErr := s.telegramChannelHasDispatchCapacity(reserveCtx, job)
		step.End(capacityErr, g.Map{"hasCapacity": hasCapacity})
		if capacityErr != nil || !hasCapacity {
			if !hasCapacity {
				reserveReason = "channel_capacity"
			}
			return false, capacityErr
		}
		result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(reserveCtx).
			Where("id", jobId).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
			Data(data).Update()
		if updateErr != nil {
			return false, updateErr
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			reserveReason = "job_state_changed"
		}
		return affected > 0, nil
	})
	beginTelegramPublishStage(ctx, "dispatch.reserve_result").End(err, g.Map{"reserved": reserved, "targetQueue": queueName})
	if err != nil {
		// 频道租约或顺序检查失败时，Stream 通知不能保持 delivered；重新激活 Outbox，避免 Job 永久 idle。
		retryDelay := telegramChannelDispatchRetryDelay(job.DispatchCount)
		if rearmErr := s.rearmTelegramJobOutbox(ctx, jobId, retryDelay); rearmErr != nil {
			g.Log().Warningf(ctx, "恢复TG任务Outbox失败 jobId:%d err:%+v", jobId, rearmErr)
		}
		g.Log().Warning(ctx, "频道调度租约获取失败，已延迟重入", g.Map{"jobId": jobId, "delayMs": retryDelay.Milliseconds(), "event": "channel_dispatch_retry"})
		return gerror.Wrap(err, "锁定TG任务入队状态失败")
	}
	if !reserved {
		g.Log().Debugf(ctx, "TG Stream任务暂未入发送队列 jobId:%d reason:%s", jobId, reserveReason)
		// Stream 事件只唤醒频道队首；非队首任务不能直接 ACK 后丢弃，
		// 必须重新激活 Outbox，待前序任务完成后再次进入就绪流。
		if globalStreamPushEngineEnabled() {
			if rearmErr := s.rearmTelegramJobOutbox(ctx, jobId, time.Second); rearmErr != nil {
				return gerror.Wrap(rearmErr, "恢复非队首TG任务Outbox失败")
			}
		}
		return nil
	}
	if err = s.enqueueTelegramTaskWithQueue(ctx, tgTaskTypePublish, jobId, delay, unique, queueName); err != nil {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).
			Where("dispatch_status", tgDispatchStatusQueued).
			Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"last_dispatch_error": err.Error(),
				"updated_at":          gtime.Now(),
			}).Update()
		return gerror.Wrap(err, "TG任务直接入队失败")
	}
	return nil
}

// telegramChannelDispatchRetryDelay 根据任务调度次数计算退避，避免锁竞争时热循环。
func telegramChannelDispatchRetryDelay(dispatchCount int) time.Duration {
	if dispatchCount < 0 {
		dispatchCount = 0
	}
	if dispatchCount > 4 {
		dispatchCount = 4
	}
	delay := time.Second << dispatchCount
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func (s *sSysPublish) enqueueTelegramCleanupJob(ctx context.Context, jobId int64, delay time.Duration) error {
	queueName := tgQueueNameCleanup
	var owner struct {
		TenantId  int64 `orm:"tenant_id"`
		AccountId int64 `orm:"account_id"`
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id").Where("id", jobId).Scan(&owner); err == nil &&
		s.unifiedPushEngineEnabled(ctx, owner.TenantId, owner.AccountId) {
		queueName = channelDispatchDeleteQueueName(channelPriorityManualDelete)
	}
	return s.enqueueTelegramTaskWithQueue(ctx, tgTaskTypeCleanup, jobId, delay, true, queueName)
}

func (s *sSysPublish) enqueueTelegramAutoDelete(ctx context.Context, payload autoDeleteQueuePayload) error {
	if payload.ChannelId <= 0 || payload.MessageId <= 0 || strings.TrimSpace(payload.ChatId) == "" {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeAutoDelete, body)
	queueName := tgQueueNameAutoDeleteRealtime
	if s.unifiedPushEngineEnabled(ctx, payload.TenantId, payload.AccountId) {
		queueName = channelDispatchDeleteQueueName(channelPriorityKeywordDelete)
	}
	_, err = client.EnqueueContext(ctx, task,
		asynq.Queue(queueName),
		asynq.MaxRetry(10),
		asynq.Timeout(time.Minute),
		asynq.Unique(2*time.Minute),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueTelegramAutoDeleteBackfill(ctx context.Context, payload autoDeleteBackfillQueuePayload, delay time.Duration) error {
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	options := []asynq.Option{
		asynq.Queue(tgQueueNameAutoDeleteBackfill),
		asynq.MaxRetry(10),
		asynq.Timeout(2 * time.Minute),
		asynq.Unique(10 * time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeAutoDeleteBackfill, body), options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueTelegramAutoDeleteBackfillDelete(ctx context.Context, payload autoDeleteQueuePayload, delay time.Duration) error {
	if payload.ChannelId <= 0 || payload.MessageId <= 0 || strings.TrimSpace(payload.ChatId) == "" {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	options := []asynq.Option{
		asynq.Queue(tgQueueNameAutoDeleteBackfill),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
		asynq.Unique(24 * time.Hour),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeAutoDelete, body), options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) requeueTelegramJob(ctx context.Context, taskType string, jobId int64, delay time.Duration) error {
	if taskType == tgTaskTypePublish {
		return s.enqueueTelegramJobDirect(ctx, jobId, delay)
	}
	return s.enqueueTelegramTask(ctx, taskType, jobId, delay, false)
}

func (s *sSysPublish) enqueueImportTask(ctx context.Context, id int64, delay time.Duration) error {
	if id <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(importQueuePayload{Id: id})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeImport, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueImportRun(ctx context.Context, runId int64, delay time.Duration) error {
	if runId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(importQueuePayload{RunId: runId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeImport, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueTgMessageRepairRun(ctx context.Context, runId int64, delay time.Duration) error {
	if runId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(tgMessageRepairQueuePayload{RunId: runId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeRepair, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueImportMatchRun(ctx context.Context, matchRunId int64, delay time.Duration) error {
	return s.enqueueImportRunMatchTask(ctx, tgTaskTypeImportMatch, matchRunId, delay)
}

func (s *sSysPublish) enqueueImportTgSyncRun(ctx context.Context, matchRunId int64, delay time.Duration) error {
	return s.enqueueImportRunMatchTask(ctx, tgTaskTypeImportSync, matchRunId, delay)
}

func (s *sSysPublish) enqueueImportRunMatchTask(ctx context.Context, taskType string, matchRunId int64, delay time.Duration) error {
	if matchRunId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(importMatchQueuePayload{MatchRunId: matchRunId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueProfileDownRun(ctx context.Context, tenantId int64, profileIds []int64, delay time.Duration) error {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(profileDownQueuePayload{
		TenantId: tenantId, ProfileIds: profileIds, DownAt: gtime.Now().String(),
		OperationNo: newTelegramOperationNo("down", profileIds[0]),
	})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeDown, payload)
	options := []asynq.Option{
		// Profile down runs perform Telegram cleanup and must not wait behind
		// interactive publish jobs sharing the urgent queue. The publish worker
		// already owns a dedicated cleanup server with independent concurrency.
		asynq.Queue(tgQueueNameCleanup),
		asynq.MaxRetry(10),
		asynq.Timeout(30 * time.Minute),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	info, err := client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		g.Log().Infof(ctx, "资料下架任务已存在，跳过重复入队 tenantId:%d profileIds:%v queue:%s", tenantId, profileIds, tgQueueNameCleanup)
		return nil
	}
	if err != nil {
		g.Log().Errorf(ctx, "资料下架任务入队失败 tenantId:%d profileIds:%v queue:%s err:%+v", tenantId, profileIds, tgQueueNameCleanup, err)
		return err
	}
	if info != nil {
		g.Log().Infof(ctx, "资料下架任务入队成功 tenantId:%d profileIds:%v queue:%s taskId:%s", tenantId, profileIds, tgQueueNameCleanup, info.ID)
	}
	return err
}

// enqueueCycleRun 提交批次生产任务；新引擎复用后台队列并限制单页执行预算。
func (s *sSysPublish) enqueueCycleRun(ctx context.Context, runId int64, delay time.Duration) error {
	if runId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(cycleRunQueuePayload{RunId: runId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCycleRun, payload)
	queueName, budget := tgQueueNameCycle, 30*time.Minute
	newEngine := globalStreamPushEngineEnabled()
	if !newEngine {
		var owner struct {
			TenantID  int64 `orm:"tenant_id"`
			AccountID int64 `orm:"account_id"`
		}
		if err := g.DB().Model(publishCycleRunTable+" r").Ctx(ctx).InnerJoin(publishChannelTable+" c", "c.id=r.channel_id").Where("r.id", runId).Fields("r.tenant_id,c.tg_account_id AS account_id").Scan(&owner); err != nil {
			return err
		}
		newEngine = s.accountPushEngine(ctx, owner.TenantID, owner.AccountID) == pushEngineUnifiedStream
	}
	// 新批次只生产任务，不执行TG发送；复用可并发扩容的后台队列，避免旧cycle单消费者阻塞所有账户。
	if newEngine {
		queueName, budget = tgQueueNameBackground, 25*time.Second
	}
	options := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(3),
		asynq.Timeout(budget),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueCycleReschedule(ctx context.Context, channelId int64, delay time.Duration) error {
	if channelId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(cycleRescheduleQueuePayload{ChannelId: channelId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCycleReschedule, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameCycle),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Hour),
		// Prevent scheduler ticks from creating duplicate rebuilds for the same
		// channel while an earlier reschedule task is queued or running.
		asynq.Unique(30 * time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueCycleSummaryRefresh(ctx context.Context, channelId int64, delay time.Duration) error {
	if channelId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(cycleRefreshQueuePayload{ChannelId: channelId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCycleRefresh, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameCycle),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
		asynq.Unique(2 * time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func decodeCycleRunQueuePayload(task *asynq.Task) (cycleRunQueuePayload, error) {
	var payload cycleRunQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func decodeCycleRescheduleQueuePayload(task *asynq.Task) (cycleRescheduleQueuePayload, error) {
	var payload cycleRescheduleQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func decodeCycleRefreshQueuePayload(task *asynq.Task) (cycleRefreshQueuePayload, error) {
	var payload cycleRefreshQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func (s *sSysPublish) enqueueTelegramTask(ctx context.Context, taskType string, jobId int64, delay time.Duration, unique bool) error {
	return s.enqueueTelegramTaskWithQueue(ctx, taskType, jobId, delay, unique, tgQueueNameDefault)
}

func (s *sSysPublish) enqueueTelegramAttemptTimeout(ctx context.Context, attemptId int64, delay time.Duration) (bool, error) {
	if attemptId <= 0 {
		return false, nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(tgAttemptQueuePayload{AttemptId: attemptId})
	if err != nil {
		return false, err
	}
	options := []asynq.Option{
		asynq.Queue(tgQueueNameAttemptTimeout),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeAttemptTimeout, payload), options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return false, nil
	}
	return err == nil, err
}

func (s *sSysPublish) enqueueTelegramTaskWithQueue(ctx context.Context, taskType string, jobId int64, delay time.Duration, unique bool, queueName string) (resultErr error) {
	ctx = telegramPublishTraceContext(ctx, jobId)
	stage := beginTelegramPublishStage(ctx, "queue.enqueue", g.Map{"taskType": taskType, "targetQueue": queueName, "delayMs": delay.Milliseconds()})
	defer stage.Finish(&resultErr)
	if jobId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(tgQueuePayload{JobId: jobId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, payload)
	if queueName == "" {
		queueName = tgQueueNameDefault
	}
	options := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(10),
		asynq.Timeout(telegramPublishTaskTimeout),
	}
	// Stream 重复通知、Outbox 恢复和频道完成唤醒可能同时尝试入队。
	// 使用资料任务的稳定 ID，保证同一 job 在 Asynq 中只有一个执行实体，
	// 避免多个不同 task ID 同时争抢同一频道租约。
	if taskType == tgTaskTypePublish {
		options = append(options, asynq.TaskID(fmt.Sprintf("tg-publish-job-%d", jobId)))
	}
	if unique {
		options = append(options, asynq.Unique(30*time.Second))
	}
	if delay > 0 {
		if taskType == tgTaskTypePublish && queueName == channelDispatchQueueSend {
			options = append(options, asynq.ProcessAt(telegramPublishScheduleTime(time.Now(), delay)))
		} else {
			options = append(options, asynq.ProcessIn(delay))
		}
	}
	// Queue submission must never hold an HTTP/API request indefinitely when
	// Redis or an Asynq Lua script is slow. Telegram deletion itself remains in
	// the existing worker; this timeout only bounds the enqueue operation.
	enqueueCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	step := beginTelegramPublishStage(ctx, "queue.redis_enqueue", g.Map{"targetQueue": queueName})
	info, err := client.EnqueueContext(enqueueCtx, task, options...)
	step.End(err, g.Map{"duplicate": errors.Is(err, asynq.ErrDuplicateTask), "taskIdConflict": errors.Is(err, asynq.ErrTaskIDConflict)})
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	// 稳定 TaskID 可能残留在 Redis 的 pending 集合中，但对应任务已经丢失。
	// 不能把冲突当成成功，否则频道队首会永久 pending；使用唯一兜底 ID 重入，
	// 执行端仍以 job 状态和频道租约幂等校验，避免重复发送。
	if errors.Is(err, asynq.ErrTaskIDConflict) && taskType == tgTaskTypePublish {
		// 稳定 ID 已被 Redis 中的旧任务占用时，先清除数据库里的旧引用。
		// 否则恢复器会反复检查同一个失效 ID，任务虽然使用 fallback 入队，
		// 但数据库仍看起来像旧任务未恢复，导致频道继续延迟重入。
		_, clearErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", jobId).Where("asynq_task_id", fmt.Sprintf("tg-publish-job-%d", jobId)).
			Data(g.Map{"asynq_task_id": "", "updated_at": time.Now()}).Update()
		if clearErr != nil {
			g.Log().Warningf(ctx, "清理TG任务稳定TaskID引用失败 jobId:%d err:%+v", jobId, clearErr)
		}
		fallbackTaskID := fmt.Sprintf("tg-publish-job-%d-fallback-%d", jobId, time.Now().UnixNano())
		fallbackOptions := []asynq.Option{
			asynq.Queue(queueName),
			asynq.MaxRetry(10),
			asynq.Timeout(telegramPublishTaskTimeout),
			asynq.TaskID(fallbackTaskID),
		}
		if unique {
			fallbackOptions = append(fallbackOptions, asynq.Unique(30*time.Second))
		}
		if delay > 0 {
			if queueName == channelDispatchQueueSend {
				fallbackOptions = append(fallbackOptions, asynq.ProcessAt(telegramPublishScheduleTime(time.Now(), delay)))
			} else {
				fallbackOptions = append(fallbackOptions, asynq.ProcessIn(delay))
			}
		}
		g.Log().Warningf(ctx, "TG任务稳定TaskID冲突，使用兜底任务重入 jobId:%d fallbackTaskId:%s", jobId, fallbackTaskID)
		info, err = client.EnqueueContext(enqueueCtx, task, fallbackOptions...)
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
	}
	if err != nil {
		return gerror.Wrapf(err, "Asynq任务入队失败 taskType:%s queue:%s jobId:%d delay:%s", taskType, queueName, jobId, delay)
	}
	// 新删除通道独立于发送；删除任务不能覆盖发送任务的队列身份，否则恢复检查会认错执行者。
	if taskType == tgTaskTypeCleanup && queueName != tgQueueNameCleanup {
		return nil
	}
	step = beginTelegramPublishStage(ctx, "queue.save_task_id", g.Map{"enqueuedTaskId": info.ID, "targetQueue": queueName})
	_, saveErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
		"asynq_task_id": info.ID,
		"queue_name":    queueName,
		"updated_at":    time.Now(),
	}).Update()
	step.End(saveErr)
	return nil
}

func (s *sSysPublish) telegramQueueClient(ctx context.Context) (*asynq.Client, error) {
	s.tgQueueMu.Lock()
	defer s.tgQueueMu.Unlock()
	if s.tgQueueClient != nil {
		return s.tgQueueClient, nil
	}
	client := asynq.NewClient(telegramQueueRedisOpt(ctx))
	s.tgQueueClient = client
	return client, nil
}

func telegramQueueRedisOpt(ctx context.Context) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
}

func telegramQueueRetryDelay(n int, err error, task *asynq.Task) time.Duration {
	var retryErr *tgRetryAfterError
	if errors.As(err, &retryErr) && retryErr.after > 0 {
		return retryErr.after + time.Duration(n)*time.Second
	}
	var collectRetryErr *collectMediaRetryError
	if errors.As(err, &collectRetryErr) && collectRetryErr.delay > 0 {
		return collectRetryErr.delay + time.Duration(n)*time.Second
	}
	var collectProcessRetryErr *collectProcessRetryError
	if errors.As(err, &collectProcessRetryErr) && collectProcessRetryErr.delay > 0 {
		return collectProcessRetryErr.delay + time.Duration(n)*time.Second
	}
	return asynq.DefaultRetryDelayFunc(n, err, task)
}

func decodeTelegramQueuePayload(task *asynq.Task) (tgQueuePayload, error) {
	var payload tgQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析TG队列任务失败: %w", err)
	}
	if payload.JobId <= 0 {
		return payload, fmt.Errorf("TG队列任务缺少jobId")
	}
	return payload, nil
}

func decodeImportQueuePayload(task *asynq.Task) (importQueuePayload, error) {
	var payload importQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析导入队列任务失败: %w", err)
	}
	if payload.Id <= 0 && payload.RunId <= 0 {
		return payload, fmt.Errorf("导入队列任务缺少id")
	}
	return payload, nil
}

func decodeTgMessageRepairQueuePayload(task *asynq.Task) (tgMessageRepairQueuePayload, error) {
	var payload tgMessageRepairQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析TG消息修复队列任务失败: %w", err)
	}
	if payload.RunId <= 0 {
		return payload, fmt.Errorf("TG消息修复队列任务缺少runId")
	}
	return payload, nil
}

func decodeImportMatchQueuePayload(task *asynq.Task) (importMatchQueuePayload, error) {
	var payload importMatchQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析导入TG匹配队列任务失败: %w", err)
	}
	if payload.MatchRunId <= 0 {
		return payload, fmt.Errorf("导入TG匹配队列任务缺少matchRunId")
	}
	return payload, nil
}

func decodeProfileDownQueuePayload(task *asynq.Task) (profileDownQueuePayload, error) {
	var payload profileDownQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析资料下架队列任务失败: %w", err)
	}
	payload.ProfileIds = uniqueIds(payload.ProfileIds)
	if payload.TenantId <= 0 || len(payload.ProfileIds) == 0 {
		return payload, fmt.Errorf("资料下架队列任务缺少tenantId或profileIds")
	}
	return payload, nil
}
