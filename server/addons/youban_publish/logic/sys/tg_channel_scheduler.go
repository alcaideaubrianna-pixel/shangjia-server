package sys

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	tgDispatchStatusIdle               = "idle"
	tgDispatchStatusQueued             = "queued"
	tgDispatchStatusProcessing         = "processing"
	tgDispatchStatusDone               = "done"
	tgSchedulerChannelCacheTTL         = time.Second
	defaultTelegramSchedulerRunTimeout = 10 * time.Second
)

func (s *sSysPublish) runTelegramChannelScheduler(ctx context.Context) {
	g.Log().Info(ctx, "TG频道调度器已启动")
	time.Sleep(time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTelegramSchedulerTick(ctx, g.Cfg().MustGet(ctx, "youbanPublish.queue.schedulerBatchSize", 50).Int())
		}
	}
}

func (s *sSysPublish) runTelegramSchedulerTick(ctx context.Context, limit int) {
	tickCtx, cancel := context.WithTimeout(ctx, telegramSchedulerRunTimeout(ctx))
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Warningf(ctx, "TG频道调度单轮异常，下一轮继续：%v", recovered)
		}
	}()
	if err := s.dispatchTelegramDueJobs(tickCtx, limit); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}
		g.Log().Warningf(ctx, "调度TG频道推送任务失败：%+v", err)
	}
}

func telegramSchedulerRunTimeout(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.queue.schedulerRunTimeoutSeconds", int(defaultTelegramSchedulerRunTimeout/time.Second)).Int()
	if seconds < 2 {
		seconds = 2
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func (s *sSysPublish) scheduleTelegramJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.scheduleTelegramJobWithOptions(ctx, jobId, delay, true)
}

func (s *sSysPublish) scheduleTelegramJobWithOptions(ctx context.Context, jobId int64, delay time.Duration, dispatch bool) error {
	if jobId <= 0 {
		return nil
	}
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	priority := s.telegramJobPriority(job)
	queueName := telegramQueueNameByPriority(priority)
	data := g.Map{
		"priority":            priority,
		"queue_name":          queueName,
		"dispatch_status":     tgDispatchStatusIdle,
		"last_dispatch_error": "",
		"updated_at":          gtime.Now(),
	}
	if delay > 0 {
		data["next_retry_at"] = gtime.Now().Add(delay)
	} else if job.Status == "pending" {
		data["next_retry_at"] = nil
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "标记TG任务待调度失败")
	}
	if shouldInvalidateTelegramSchedulerChannelCache(job, delay) {
		s.invalidateTelegramSchedulerChannelCache(ctx, job.ChannelId, job.TargetChatId)
	}
	if dispatch && delay <= 0 {
		return s.dispatchTelegramDueJobs(ctx, 10)
	}
	return nil
}

func shouldInvalidateTelegramSchedulerChannelCache(job telegramJobRecord, delay time.Duration) bool {
	return delay <= 0 && isTelegramUrgentJob(job)
}

func (s *sSysPublish) dispatchTelegramDueJobs(ctx context.Context, limit int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if limit <= 0 {
		limit = 50
	}
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	lock := hglock.NewConfig(5*time.Second, 100*time.Millisecond).Mutex("youban_publish:tg_scheduler")
	if err := lock.TryLock(lockCtx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取TG调度器锁失败")
	}
	if err := s.resetStaleTelegramDispatchJobs(ctx); err != nil {
		s.releaseTelegramChannelLease(context.Background(), lock)
		return err
	}
	jobs, err := s.telegramSchedulerCandidates(ctx, limit)
	s.releaseTelegramChannelLease(context.Background(), lock)
	if err != nil {
		return err
	}
	usedChannels := make(map[string]struct{})
	blockedChannels := make(map[string]struct{})
	dispatched := 0
	for _, job := range jobs {
		if dispatched >= limit {
			break
		}
		channelKey := telegramSchedulerChannelKey(job)
		if _, ok := usedChannels[channelKey]; ok {
			continue
		}
		if _, ok := blockedChannels[channelKey]; ok {
			continue
		}
		busy, err := s.telegramChannelHasActiveDispatch(ctx, job)
		if err != nil {
			return err
		}
		if busy {
			if channelKey != "" {
				blockedChannels[channelKey] = struct{}{}
			}
			continue
		}
		waitingChannelOrder, err := s.telegramChannelHasEarlierActiveJob(ctx, job)
		if err != nil {
			return err
		}
		if waitingChannelOrder {
			if channelKey != "" {
				blockedChannels[channelKey] = struct{}{}
			}
			continue
		}
		waitingPrevious, err := s.collectTelegramJobHasPreviousActive(ctx, job)
		if err != nil {
			return err
		}
		if waitingPrevious {
			continue
		}
		queued, err := s.dispatchTelegramJob(ctx, job)
		if err != nil {
			return err
		}
		if queued {
			usedChannels[channelKey] = struct{}{}
			dispatched++
		}
	}
	return nil
}

type telegramSchedulerChannel struct {
	ChannelId    int64  `orm:"channel_id"`
	TargetChatId string `orm:"target_chat_id"`
}

func (s *sSysPublish) telegramSchedulerCandidates(ctx context.Context, limit int) ([]telegramJobRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	candidateLimit := limit * 8
	if candidateLimit < 100 {
		candidateLimit = 100
	}
	if candidateLimit > 1000 {
		candidateLimit = 1000
	}
	now := gtime.Now()
	bulk, err := s.telegramSchedulerCandidatesByPriority(ctx, now, telegramSchedulerPriorityBulk, candidateLimit)
	if err != nil {
		return nil, err
	}
	urgent, err := s.telegramSchedulerCandidatesByPriority(ctx, now, telegramSchedulerPriorityUrgent, candidateLimit)
	if err != nil {
		return nil, err
	}
	normal, err := s.telegramSchedulerCandidatesByPriority(ctx, now, telegramSchedulerPriorityNormal, candidateLimit)
	if err != nil {
		return nil, err
	}
	return mergeTelegramSchedulerCandidates(urgent, normal, bulk, limit), nil
}

const (
	telegramSchedulerPriorityUrgent = iota + 1
	telegramSchedulerPriorityNormal
	telegramSchedulerPriorityBulk
)

func (s *sSysPublish) telegramSchedulerCandidatesByPriority(ctx context.Context, now *gtime.Time, bucket int, limit int) ([]telegramJobRecord, error) {
	priorityExpr := `CASE
		WHEN LOWER(TRIM(j.operation_no)) LIKE 'profile:%' THEN 10
		WHEN j.priority > 0 AND j.priority <> 100 THEN j.priority
		WHEN LOWER(TRIM(j.operation_no)) LIKE 'full_push:%' OR LOWER(TRIM(j.operation_no)) LIKE 'cycle_batch:%' THEN 90
		ELSE 50
	END`
	priorityCondition := "(" + priorityExpr + ") >= 90"
	switch bucket {
	case telegramSchedulerPriorityUrgent:
		priorityCondition = "(" + priorityExpr + ") <= 10"
	case telegramSchedulerPriorityNormal:
		priorityCondition = "(" + priorityExpr + ") > 10 AND (" + priorityExpr + ") < 90"
	}
	mod := g.DB().Model(publishTgJobTable + " j").Safe().Ctx(ctx).Unscoped()
	if bucket == telegramSchedulerPriorityBulk {
		mod = mod.Where("(LOWER(TRIM(j.operation_no)) LIKE 'full_push:%' OR LOWER(TRIM(j.operation_no)) LIKE 'cycle_batch:%')")
	} else {
		mod = mod.Where(telegramSchedulerCollectPredecessorCondition())
	}
	var jobs []telegramJobRecord
	err := mod.
		Fields("j.*").
		WhereIn("j.status", []string{"pending", "failed_retry", "unknown"}).
		Where("(j.next_retry_at IS NULL OR j.next_retry_at <= ?)", now).
		Where("(j.dispatch_status = ? OR j.dispatch_status = '')", tgDispatchStatusIdle).
		Where(priorityCondition).
		OrderAsc("j.created_at").OrderAsc("j.id").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrapf(err, "读取TG待调度任务失败 bucket:%d", bucket)
	}
	return jobs, nil
}

func mergeTelegramSchedulerCandidates(urgent, normal, bulk []telegramJobRecord, limit int) []telegramJobRecord {
	if limit <= 0 {
		return nil
	}
	result := make([]telegramJobRecord, 0, len(urgent)+len(normal)+len(bulk))
	urgentLimit := len(urgent)
	if len(bulk) > 0 && urgentLimit >= limit {
		urgentLimit = limit - 1
	}
	result = append(result, urgent[:urgentLimit]...)
	if len(bulk) > 0 && urgentLimit < limit {
		result = append(result, bulk[0])
		bulk = bulk[1:]
	}
	normalIndex, bulkIndex := 0, 0
	for normalIndex < len(normal) || bulkIndex < len(bulk) {
		for i := 0; i < 4 && normalIndex < len(normal); i++ {
			result = append(result, normal[normalIndex])
			normalIndex++
		}
		if bulkIndex < len(bulk) {
			result = append(result, bulk[bulkIndex])
			bulkIndex++
		}
	}
	if len(result) > urgentLimit+limit*9 {
		result = result[:urgentLimit+limit*9]
	}
	return result
}

func telegramSchedulerChannelCacheKey(channel telegramSchedulerChannel) string {
	if channel.ChannelId > 0 {
		return "youban_publish:tg_scheduler:candidates:channel:" + strconv.FormatInt(channel.ChannelId, 10)
	}
	return "youban_publish:tg_scheduler:candidates:chat:" + normalizeTelegramChannelChatID(channel.TargetChatId)
}

func (s *sSysPublish) invalidateTelegramSchedulerChannelCache(ctx context.Context, channelId int64, targetChatId string) {
	channel := telegramSchedulerChannel{ChannelId: channelId, TargetChatId: targetChatId}
	if channel.ChannelId <= 0 && strings.TrimSpace(channel.TargetChatId) == "" {
		return
	}
	if _, err := cache.Instance().Remove(ctx, telegramSchedulerChannelCacheKey(channel)); err != nil {
		g.Log().Warningf(ctx, "清理TG频道调度候选缓存失败 channelId:%d chat:%s err:%+v", channelId, targetChatId, err)
	}
}

func telegramSchedulerCollectPredecessorCondition() string {
	return `(j.collect_source_id <= 0 OR j.collect_source_message_id <= 0 OR j.collect_source_chat_id = '' OR NOT EXISTS (
		SELECT 1
		FROM ` + publishTgJobTable + ` pj
		WHERE pj.id <> j.id
		  AND pj.collect_source_id = j.collect_source_id
		  AND pj.collect_source_chat_id = j.collect_source_chat_id
		  AND pj.collect_source_message_id > 0
		  AND pj.collect_source_message_id < j.collect_source_message_id
		  AND (
			pj.status IN ('pending', 'sending', 'unknown')
			OR (pj.status = 'failed_retry' AND (pj.next_retry_at IS NULL OR pj.next_retry_at <= NOW()))
		  )
		  AND ((j.channel_id > 0 AND pj.channel_id = j.channel_id) OR (j.channel_id <= 0 AND pj.target_chat_id = j.target_chat_id))
	))`
}

func (s *sSysPublish) telegramChannelHasActiveDispatch(ctx context.Context, job telegramJobRecord) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	channelKey := telegramSchedulerChannelKey(job)
	if channelKey == "" {
		return false, nil
	}
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Unscoped().
		Where("j.id <> ?", job.Id).
		WhereIn("j.status", []string{"sending", "pending", "failed_retry", "unknown"}).
		Where("(j.dispatch_status IN (?, ?) OR j.status = ?)", tgDispatchStatusQueued, tgDispatchStatusProcessing, "sending")
	if isTelegramUrgentJob(job) {
		mod = mod.Where("(j.status = ? OR j.dispatch_status = ? OR (j.dispatch_status = ? AND j.priority <= ?))", "sending", tgDispatchStatusProcessing, tgDispatchStatusQueued, tgJobPriorityUrgent)
	}
	if job.ChannelId > 0 {
		mod = mod.Where("j.channel_id", job.ChannelId)
	} else {
		mod = mod.Where("j.target_chat_id", job.TargetChatId)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查TG频道调度状态失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) dispatchTelegramJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	priority := s.telegramJobPriority(job)
	queueName := telegramQueueNameByPriority(priority)
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		Data(g.Map{
			"priority":            priority,
			"queue_name":          queueName,
			"dispatch_status":     tgDispatchStatusQueued,
			"dispatched_at":       gtime.Now(),
			"dispatch_count":      gdb.Raw("dispatch_count + 1"),
			"last_dispatch_error": "",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return false, gerror.Wrap(err, "锁定TG任务调度状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	if err = s.enqueueTelegramTaskWithQueue(ctx, tgTaskTypePublish, job.Id, 0, true, queueName); err != nil {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"last_dispatch_error": err.Error(),
			"updated_at":          gtime.Now(),
		}).Update()
		return false, gerror.Wrap(err, "TG任务入队失败")
	}
	return true, nil
}

func (s *sSysPublish) resetStaleTelegramDispatchJobs(ctx context.Context) error {
	timeoutSeconds := g.Cfg().MustGet(ctx, "youbanPublish.queue.dispatchTimeoutSeconds", 300).Int()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("dispatch_status", tgDispatchStatusQueued).
		Where("dispatched_at <= ?", gtime.Now().Add(-time.Duration(timeoutSeconds)*time.Second)).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"last_dispatch_error": "调度后长时间未被消费，已重新进入待调度队列",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "恢复超时TG调度任务失败")
	}
	return nil
}

func (s *sSysPublish) telegramJobPriority(job telegramJobRecord) int {
	return telegramJobPriorityValue(job)
}

func isTelegramUrgentJob(job telegramJobRecord) bool {
	return telegramJobPriorityValue(job) <= tgJobPriorityUrgent
}

func telegramJobPriorityValue(job telegramJobRecord) int {
	operationNo := strings.ToLower(strings.TrimSpace(job.OperationNo))
	if strings.HasPrefix(operationNo, "profile:") {
		return tgJobPriorityUrgent
	}
	if job.Priority > 0 && job.Priority != 100 {
		return job.Priority
	}
	if strings.HasPrefix(operationNo, "full_push:") || strings.HasPrefix(operationNo, "cycle_batch:") {
		return tgJobPriorityBulk
	}
	return tgJobPriorityDefault
}

func telegramQueueNameByPriority(priority int) string {
	switch {
	case priority <= tgJobPriorityUrgent:
		return tgQueueNameUrgent
	case priority >= tgJobPriorityBulk:
		return tgQueueNameBulk
	default:
		return tgQueueNameDefault
	}
}

func telegramSchedulerChannelKey(job telegramJobRecord) string {
	if job.ChannelId > 0 {
		return "channel:" + strconv.FormatInt(job.ChannelId, 10)
	}
	chatId := strings.TrimSpace(normalizeTelegramChannelChatID(job.TargetChatId))
	if chatId == "" {
		return ""
	}
	return "chat:" + chatId
}
