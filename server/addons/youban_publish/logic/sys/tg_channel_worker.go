package sys

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/utility/dbtime"
)

// telegramChannelBlockReason describes why a queue item cannot make progress.
// It is deliberately separate from the persisted job status: the status tells
// us what the sender is doing, while this value tells the channel scheduler
// whether it may yield the head.
type telegramChannelBlockReason string

const (
	telegramChannelBlockNone          telegramChannelBlockReason = ""
	telegramChannelBlockMedia         telegramChannelBlockReason = "media_not_ready"
	telegramChannelBlockAccount       telegramChannelBlockReason = "account_fallback"
	telegramChannelBlockUnknownResult telegramChannelBlockReason = "unknown_result"
)

type telegramChannelQueueDecision struct {
	BlockReason telegramChannelBlockReason
	YieldHead   bool
	Delay       time.Duration
}

func telegramChannelEffectivePriority(job telegramJobRecord, mediaReady ...bool) int {
	priority := job.Priority
	if priority <= 0 {
		priority = tgJobPriorityBulk
	}
	decision := telegramChannelQueueDecisionFor(job)
	if priority > tgJobPriorityHistory {
		priority = tgJobPriorityHistory
	}
	if decision.YieldHead && !(len(mediaReady) > 0 && mediaReady[0]) {
		return tgJobPriorityHistory
	}
	return priority
}

func telegramChannelPriorityAfterFailure(priority, previousRetryCount, retryCount int) int {
	if priority <= 0 {
		priority = tgJobPriorityBulk
	}
	if priority > tgJobPriorityHistory {
		priority = tgJobPriorityHistory
	}
	if retryCount <= previousRetryCount || retryCount <= 0 || retryCount%3 != 0 || priority >= tgJobPriorityHistory {
		return priority
	}
	// Higher numbers are lower priority. Keep the downgrade bounded at the
	// existing history lane so a failed job can yield without becoming invalid.
	if next := priority + 5; next < tgJobPriorityHistory {
		return next
	}
	return tgJobPriorityHistory
}

func telegramChannelQueueDecisionFor(job telegramJobRecord) telegramChannelQueueDecision {
	reason := strings.TrimSpace(job.LastDispatchError)
	switch {
	case telegramChannelJobHasUnknownResult(job):
		// An in-flight or uncertain send always preserves the channel barrier,
		// even if an earlier media-wait error remains on the record.
		return telegramChannelQueueDecision{BlockReason: telegramChannelBlockUnknownResult, Delay: 30 * time.Second}
	case strings.Contains(reason, "awaiting_account_fallback") || strings.Contains(reason, "账号降级"):
		return telegramChannelQueueDecision{BlockReason: telegramChannelBlockAccount, Delay: 30 * time.Second}
	case strings.Contains(reason, "媒体仍在异步处理中"):
		return telegramChannelQueueDecision{BlockReason: telegramChannelBlockMedia, YieldHead: true, Delay: 10 * time.Second}
	default:
		return telegramChannelQueueDecision{BlockReason: telegramChannelBlockNone, Delay: time.Second}
	}
}

func telegramChannelJobHasUnknownResult(job telegramJobRecord) bool {
	if job.Status == telegramJobStatusSending || job.Status == telegramJobStatusConfirming || job.Status == "reconciling" {
		return true
	}
	reason := strings.ToLower(strings.TrimSpace(job.LastDispatchError))
	for _, marker := range []string{
		"context canceled", "context deadline exceeded", "timeout", "超时",
		"等待webhook", "等待对账", "协议号对账", "网络错误", "network error",
	} {
		if strings.Contains(reason, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// telegramChannelQueueEnabled defaults to the channel queue transport. Set the
// environment variable to false for an emergency rollback to Asynq.
func telegramChannelQueueEnabled() bool {
	v := strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED"))
	if v == "" {
		return true
	}
	b, _ := strconv.ParseBool(v)
	if strings.EqualFold(v, "false") || v == "0" {
		return false
	}
	return b || !strings.EqualFold(v, "false")
}

// processTelegramChannelQueueHead claims and executes one logical channel
// queue item. The existing SendTelegramJob remains the single send/lease
// implementation; this adapter only supplies ordering and requeue semantics.
// It is not called until YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED is enabled.
func (s *sSysPublish) processTelegramChannelQueueHead(ctx context.Context, channelID int64) (bool, error) {
	if !telegramChannelQueueEnabled() {
		return false, nil
	}
	dequeueStartedAt := time.Now()
	claim, err := dequeueTelegramChannelHeadWithScore(ctx, channelID)
	if err != nil {
		g.Log().Warning(ctx, "频道队列取头失败", g.Map{"event": "channel_queue_error", "stage": "dequeue", "channelId": channelID, "error": err.Error()})
		return false, err
	}
	jobID := claim.JobID
	if jobID <= 0 {
		return false, nil
	}
	job, loadErr := s.telegramJobById(ctx, jobID)
	if loadErr != nil {
		g.Log().Warning(ctx, "频道队列任务加载失败", g.Map{"event": "channel_queue_error", "stage": "load", "channelId": channelID, "jobId": jobID, "error": loadErr.Error()})
		if enqueueErr := enqueueTelegramChannelJob(ctx, channelID, jobID, tgJobPriorityDefault, time.Now().Add(time.Second)); enqueueErr != nil {
			return true, errors.Join(loadErr, enqueueErr)
		}
		return true, loadErr
	}
	// A stale media-wait error is not proof that media is still unavailable.
	// Check the existing readiness gate before choosing the migration lane.
	mediaReady := false
	if telegramChannelQueueDecisionFor(job).YieldHead {
		mediaReady, _ = s.profileMediaReady(ctx, job.ProfileId)
	}
	priority := telegramChannelEffectivePriority(job, mediaReady)
	if telegramChannelQueueNeedsMigration(claim.Score, priority, claim.Urgent, claim.History) {
		// Historical members use the old score format. Reinsert the same job
		// with the current lane/score before sending; this preserves the job and
		// lets a newly clicked manual job stay ahead on the next pass.
		if migrateErr := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, time.Now()); migrateErr != nil {
			g.Log().Warning(ctx, "频道队列旧成员迁移失败，保留任务待恢复", g.Map{
				"event": "channel_queue_migration_failed", "channelId": channelID,
				"jobId": jobID, "score": claim.Score, "priority": priority,
				"error": migrateErr.Error(),
			})
			return true, migrateErr
		}
		g.Log().Info(ctx, "频道队列旧成员已重算排序", g.Map{
			"event": "channel_queue_migrated", "channelId": channelID,
			"jobId": jobID, "oldScore": claim.Score,
			"newScore": telegramChannelQueueScore(priority, time.Now(), jobID),
			"priority": priority, "urgent": priority <= tgJobPriorityInteractive,
		})
		return true, nil
	}
	queueName := telegramChannelQueueKey(channelID)
	if claim.Urgent {
		queueName = telegramChannelUrgentQueueKey(channelID)
	} else if claim.History {
		queueName = telegramChannelHistoryQueueKey(channelID)
	}
	g.Log().Info(ctx, "频道队列取出任务", g.Map{
		"event":       "channel_queue_dequeue",
		"channelId":   channelID,
		"jobId":       jobID,
		"profileId":   job.ProfileId,
		"operationNo": job.OperationNo,
		"priority":    priority,
		"queue":       queueName,
		"score":       telegramChannelQueueScore(priority, time.Now(), jobID),
		"durationMs":  time.Since(dequeueStartedAt).Milliseconds(),
		"reason":      "head_claimed",
	})
	retryCountBeforeSend := job.RetryCount
	priorityBeforeSend := job.Priority
	if err = s.SendTelegramJob(ctx, jobID); err != nil {
		// Keep the job available for the next attempt. A bounded delay avoids a
		// hot loop when the channel lease or Telegram API is temporarily busy.
		hold := time.Now().Add(time.Second).UnixMilli()
		if refreshed, reloadErr := s.telegramJobById(ctx, jobID); reloadErr == nil {
			job = refreshed
		}
		priority := job.Priority
		if priority <= 0 {
			priority = tgJobPriorityBulk
		}
		// Downgrade only when this send actually advanced the retry counter.
		newPriority := priority
		if job.RetryCount > retryCountBeforeSend && !isTelegramChannelJobTerminal(job) && telegramChannelQueueDecisionFor(job).BlockReason == telegramChannelBlockNone {
			newPriority = telegramChannelPriorityAfterFailure(priorityBeforeSend, retryCountBeforeSend, job.RetryCount)
		}
		if newPriority != priority {
			result, updateErr := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("id", jobID).Where("retry_count", job.RetryCount).Where("priority", priorityBeforeSend).Data(g.Map{"priority": newPriority, "updated_at": gtime.Now()}).Update()
			if updateErr != nil {
				g.Log().Warning(ctx, "频道队列任务优先级更新失败", g.Map{"jobId": jobID, "error": updateErr.Error()})
			} else if affected, rowsErr := result.RowsAffected(); rowsErr == nil && affected > 0 {
				job.Priority = newPriority
			}
		}
		priority = telegramChannelEffectivePriority(job, mediaReady)
		score := telegramChannelQueueScore(priority, time.UnixMilli(hold), jobID)
		if isTelegramChannelJobTerminal(job) {
			g.Log().Info(ctx, "频道队列任务发送失败后已收口", g.Map{"event": "channel_queue_ack", "channelId": channelID, "jobId": jobID, "status": job.Status, "reason": "terminal_after_send_error"})
			return true, nil
		}
		if enqueueErr := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, time.UnixMilli(hold)); enqueueErr != nil {
			return true, gerror.Wrap(enqueueErr, "频道队列发送失败后重入队失败")
		}
		g.Log().Warning(ctx, "频道队列任务发送失败，已重入队", g.Map{
			"event":       "channel_queue_requeue",
			"stage":       "send",
			"channelId":   channelID,
			"jobId":       jobID,
			"profileId":   job.ProfileId,
			"operationNo": job.OperationNo,
			"priority":    priority,
			"queue":       queueName,
			"score":       score,
			"durationMs":  time.Since(dequeueStartedAt).Milliseconds(),
			"reason":      telegramPublishLogError(err),
		})
		return true, err
	}
	// SendTelegramJob may return nil when another sender owns the lease. Do
	// not lose the popped item in that case; only terminal states are acked.
	if job, loadErr := s.telegramJobById(ctx, jobID); loadErr == nil {
		switch job.Status {
		case telegramJobStatusSent, "failed", "superseded":
			g.Log().Info(ctx, "频道队列任务确认完成", g.Map{
				"event":       "channel_queue_ack",
				"channelId":   channelID,
				"jobId":       jobID,
				"profileId":   job.ProfileId,
				"operationNo": job.OperationNo,
				"priority":    priority,
				"queue":       queueName,
				"score":       telegramChannelQueueScore(priority, time.Now(), jobID),
				"durationMs":  time.Since(dequeueStartedAt).Milliseconds(),
				"status":      job.Status,
				"reason":      "terminal",
			})
			return true, nil
		default:
			hold, reason := telegramChannelJobRequeueAt(job)
			decision := telegramChannelQueueDecisionFor(job)
			priority := telegramChannelEffectivePriority(job, mediaReady)
			if strings.Contains(reason, "媒体仍在异步处理中") {
				g.Log().Info(ctx, "频道队列任务等待媒体就绪", g.Map{"event": "channel_queue_media_not_ready", "channelId": channelID, "jobId": jobID, "profileId": job.ProfileId, "score": hold})
			}
			requeueScore := telegramChannelQueueScore(priority, time.UnixMilli(hold), jobID)
			if isTelegramChannelJobTerminal(job) {
				g.Log().Info(ctx, "频道队列任务已收口", g.Map{"event": "channel_queue_ack", "channelId": channelID, "jobId": jobID, "status": job.Status, "reason": "terminal_after_reload"})
				return true, nil
			}
			if enqueueErr := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, time.UnixMilli(hold)); enqueueErr != nil {
				return true, gerror.Wrap(enqueueErr, "频道队列未完成任务重入队失败")
			}
			g.Log().Warning(ctx, "频道队列任务未完成，已重入队", g.Map{
				"event":           "channel_queue_requeue",
				"stage":           "post_send",
				"channelId":       channelID,
				"jobId":           jobID,
				"profileId":       job.ProfileId,
				"operationNo":     job.OperationNo,
				"priority":        priority,
				"queue":           queueName,
				"status":          job.Status,
				"score":           requeueScore,
				"durationMs":      time.Since(dequeueStartedAt).Milliseconds(),
				"reason":          reason,
				"blockReason":     decision.BlockReason,
				"yieldHead":       decision.YieldHead,
				"recoveryDelayMs": time.Until(time.UnixMilli(hold)).Milliseconds(),
			})
		}
	} else {
		hold := time.Now().Add(time.Second).UnixMilli()
		if enqueueErr := enqueueTelegramChannelJob(ctx, channelID, jobID, tgJobPriorityDefault, time.UnixMilli(hold)); enqueueErr != nil {
			return true, gerror.Wrap(enqueueErr, "频道队列状态读取失败后重入队失败")
		}
		g.Log().Warning(ctx, "频道队列任务状态读取失败，已重入队", g.Map{"event": "channel_queue_requeue", "stage": "reload", "channelId": channelID, "jobId": jobID, "score": hold, "error": loadErr.Error()})
	}
	return true, nil
}

func telegramChannelJobRequeueAt(job telegramJobRecord) (int64, string) {
	now := time.Now()
	reason := strings.TrimSpace(job.LastDispatchError)
	switch job.Status {
	case telegramJobStatusRetryWait:
		if reason == "" {
			reason = "任务等待下一次重试"
		}
		if retryAt := dbtime.FromTimestamp(job.NextRetryAt); retryAt != nil && retryAt.Time.After(now) {
			return retryAt.TimestampMilli(), reason
		}
		return now.Add(time.Second).UnixMilli(), reason
	case telegramJobStatusPending:
		if reason == "" {
			reason = "任务仍待发送"
		}
		if retryAt := dbtime.FromTimestamp(job.NextRetryAt); retryAt != nil && retryAt.Time.After(now) {
			return retryAt.TimestampMilli(), reason
		}
		return now.Add(3 * time.Second).UnixMilli(), reason
	case telegramJobStatusSending, telegramJobStatusConfirming, "reconciling":
		if reason == "" {
			reason = "任务正在发送或等待对账"
		}
		// An in-flight or uncertain Telegram attempt is owned by the
		// watchdog/reconciliation path. Keep a delayed Redis member only as a
		// liveness fallback; never hot-loop it as a normal send retry.
		return now.Add(30 * time.Second).UnixMilli(), reason
	default:
		if reason == "" {
			reason = "任务状态未收口"
		}
		return now.Add(3 * time.Second).UnixMilli(), reason
	}
}

func isTelegramChannelJobTerminal(job telegramJobRecord) bool {
	switch job.Status {
	case telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded:
		return true
	default:
		return false
	}
}
