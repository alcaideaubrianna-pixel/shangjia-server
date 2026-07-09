package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	tgDispatchStatusIdle       = "idle"
	tgDispatchStatusQueued     = "queued"
	tgDispatchStatusProcessing = "processing"
	tgDispatchStatusDone       = "done"
)

func (s *sSysPublish) runTelegramChannelScheduler(ctx context.Context) {
	time.Sleep(time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.dispatchTelegramDueJobs(ctx, g.Cfg().MustGet(ctx, "youbanPublish.queue.schedulerBatchSize", 50).Int()); err != nil {
				g.Log().Warningf(ctx, "调度TG频道推送任务失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) scheduleTelegramJob(ctx context.Context, jobId int64, delay time.Duration) error {
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
	if delay <= 0 {
		return s.dispatchTelegramDueJobs(ctx, 10)
	}
	return nil
}

func (s *sSysPublish) dispatchTelegramDueJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 50
	}
	if err := ensureCollectTelegramOrderColumns(ctx); err != nil {
		return err
	}
	lock := hglock.NewConfig(5*time.Second, 100*time.Millisecond).Mutex("youban_publish:tg_scheduler")
	if err := lock.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取TG调度器锁失败")
	}
	defer s.releaseTelegramChannelLease(ctx, lock)
	if err := s.resetStaleTelegramDispatchJobs(ctx); err != nil {
		return err
	}
	jobs, err := s.telegramSchedulerCandidates(ctx, limit*6)
	if err != nil {
		return err
	}
	usedChannels := make(map[string]struct{})
	dispatched := 0
	for _, job := range jobs {
		if dispatched >= limit {
			break
		}
		channelKey := telegramSchedulerChannelKey(job)
		if _, ok := usedChannels[channelKey]; ok {
			continue
		}
		busy, err := s.telegramChannelHasActiveDispatch(ctx, job)
		if err != nil {
			return err
		}
		if busy {
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

func (s *sSysPublish) telegramSchedulerCandidates(ctx context.Context, limit int) ([]telegramJobRecord, error) {
	if limit <= 0 {
		limit = 200
	}
	var jobs []telegramJobRecord
	now := gtime.Now()
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		OrderAsc("priority").OrderAsc("collect_source_id").OrderAsc("collect_source_chat_id").OrderAsc("collect_source_message_id").OrderAsc("id").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG待调度任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) telegramChannelHasActiveDispatch(ctx context.Context, job telegramJobRecord) (bool, error) {
	channelKey := telegramSchedulerChannelKey(job)
	if channelKey == "" {
		return false, nil
	}
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id <> ?", job.Id).
		WhereIn("status", []string{"sending", "pending", "failed_retry"}).
		Where("(dispatch_status IN (?, ?) OR status = ?)", tgDispatchStatusQueued, tgDispatchStatusProcessing, "sending")
	if job.ChannelId > 0 {
		mod = mod.Where("channel_id", job.ChannelId)
	} else {
		mod = mod.Where("target_chat_id", job.TargetChatId)
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
		WhereIn("status", []string{"pending", "failed_retry"}).
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
		WhereIn("status", []string{"pending", "failed_retry"}).
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
	if job.Priority > 0 && job.Priority != 100 {
		return job.Priority
	}
	operationNo := strings.ToLower(strings.TrimSpace(job.OperationNo))
	if strings.HasPrefix(operationNo, "full_push:") {
		return tgJobPriorityBulk
	}
	if job.CycleEnabled == 1 && job.NextCycleAt != nil {
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
