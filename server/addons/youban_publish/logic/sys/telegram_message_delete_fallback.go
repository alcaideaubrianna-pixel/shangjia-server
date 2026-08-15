package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

const (
	telegramDeleteFallbackPriority = -100
	telegramDeleteFallbackInterval = 2 * time.Minute
)

type telegramDeleteFallbackRetryError struct {
	cause error
	delay time.Duration
}

func (e *telegramDeleteFallbackRetryError) Error() string                        { return e.cause.Error() }
func (e *telegramDeleteFallbackRetryError) Unwrap() error                        { return e.cause }
func (e *telegramDeleteFallbackRetryError) AccountTaskRetryDelay() time.Duration { return e.delay }

func (s *sSysPublish) enqueueTelegramMessageDeleteFallback(ctx context.Context, job telegramJobRecord, reason string, cause error) {
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil || channel.TgAccountId <= 0 {
		s.appendTelegramJobLog(ctx, job, "delete_fallback", "failed", "协议号删除兜底不可用：频道未绑定有效协议号")
		return
	}
	nextRunAt := telegramDeleteFallbackNextRunAt(ctx, channel.TgAccountId, job.Id)
	wait := time.Until(nextRunAt)
	maxAttempts := telegramDeleteFallbackConfigInt(ctx, "maxAttempts", 5, 1, 10)
	_, err = collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: job.TenantId, AccountID: channel.TgAccountId,
		TaskType: collectorin.AccountTaskTypeMessageDeleteFallback,
		TaskKey:  fmt.Sprintf("message-delete-fallback:%d", job.Id),
		Priority: telegramDeleteFallbackPriority, MaxAttempts: maxAttempts, NextRunAt: &nextRunAt,
	})
	if err != nil {
		observeTelegramDeleteFallback(ctx, "enqueue_failed", channel.TgAccountId, 0)
		g.Log().Warningf(ctx, "协议号删除兜底任务提交失败 tgAccountId:%d jobId:%d channelId:%d err:%+v", channel.TgAccountId, job.Id, job.ChannelId, err)
		s.appendTelegramJobLog(ctx, job, "delete_fallback", "failed", "提交协议号删除兜底任务失败："+err.Error())
		return
	}
	observeTelegramDeleteFallback(ctx, "queued", channel.TgAccountId, int64(telegramDeleteMessageCount(ctx, job.Id)))
	observeTelegramDeleteFallbackWait(ctx, "queue", channel.TgAccountId, wait)
	g.Log().Infof(ctx, "协议号删除兜底任务已排队 tgAccountId:%d jobId:%d channelId:%d wait:%s nextRunAt:%s", channel.TgAccountId, job.Id, job.ChannelId, wait.Round(time.Second), nextRunAt.Format(time.RFC3339))
	message := fmt.Sprintf("%s，Bot删除失败，已提交协议号低优先级删除任务，预计执行时间:%s", reason, nextRunAt.Format("2006-01-02 15:04:05"))
	if cause != nil {
		message += "，原因：" + cause.Error()
	}
	s.appendTelegramJobLog(ctx, job, "delete_fallback", "queued", message)
}

func telegramDeleteFallbackNextRunAt(ctx context.Context, tgAccountID, jobID int64) time.Time {
	now := time.Now()
	base := now
	value, err := g.DB().Model("hg_tg_collector_account_task").Safe().Ctx(ctx).
		Fields("MAX(next_run_at) AS next_run_at").
		Where("account_id", tgAccountID).
		Where("task_type", collectorin.AccountTaskTypeMessageDeleteFallback).
		WhereIn("status", []string{collectorin.AccountTaskStatusPending, collectorin.AccountTaskStatusFailedRetry, collectorin.AccountTaskStatusProcessing}).
		One()
	if err == nil && !value.IsEmpty() {
		if latest := value["next_run_at"].GTime(); latest != nil && latest.Time.After(base) {
			base = latest.Time
		}
	}
	intervalSeconds := telegramDeleteFallbackConfigInt(ctx, "intervalSeconds", int(telegramDeleteFallbackInterval/time.Second), 10, 3600)
	jitterSeconds := telegramDeleteFallbackConfigInt(ctx, "jitterSeconds", 30, 0, 300)
	jitter := time.Duration(0)
	if jitterSeconds > 0 {
		jitter = time.Duration(jobID%int64(jitterSeconds+1)) * time.Second
	}
	return base.Add(time.Duration(intervalSeconds)*time.Second + jitter)
}

func telegramDeleteFallbackConfigInt(ctx context.Context, key string, fallback, minimum, maximum int) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.telegramDeleteFallback."+key, fallback).Int()
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func (s *sSysPublish) handleMessageDeleteFallbackAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) error {
	const prefix = "message-delete-fallback:"
	if task == nil || !strings.HasPrefix(task.TaskKey, prefix) {
		return gerror.New("协议号删除兜底任务参数无效")
	}
	jobID, err := strconv.ParseInt(strings.TrimPrefix(task.TaskKey, prefix), 10, 64)
	if err != nil || jobID <= 0 {
		return gerror.New("协议号删除兜底任务ID无效")
	}
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if channel.TgAccountId != task.AccountID {
		return gerror.New("协议号删除兜底任务与频道绑定账号不一致")
	}
	observeTelegramDeleteFallback(ctx, "started", task.AccountID, 0)
	g.Log().Infof(ctx, "协议号删除兜底开始执行 tgAccountId:%d taskId:%d jobId:%d attempt:%d/%d", task.AccountID, task.ID, jobID, task.AttemptCount, task.MaxAttempts)
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	inputChannel, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		return gerror.New("协议号删除兜底仅支持频道和超级群")
	}
	messages, err := s.telegramJobDeleteFallbackMessages(ctx, jobID)
	if err != nil || len(messages) == 0 {
		return err
	}
	for _, batch := range telegramDeleteMessageBatches(messages, telegramDeleteMessagesMaxItems) {
		ids := make([]int, 0, len(batch.messages))
		for _, item := range batch.messages {
			ids = append(ids, int(item.MessageId))
		}
		_, deleteErr := client.API().ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: inputChannel.ChannelID, AccessHash: inputChannel.AccessHash}, ID: ids,
		})
		if deleteErr != nil {
			if delay, flood := tgerr.AsFloodWait(deleteErr); flood {
				buffer := time.Duration(telegramDeleteFallbackConfigInt(ctx, "floodWaitBufferSeconds", 5, 1, 60)) * time.Second
				retryAfter := delay + buffer
				observeTelegramDeleteFallback(ctx, "flood_wait", task.AccountID, 0)
				observeTelegramDeleteFallbackWait(ctx, "flood_wait", task.AccountID, delay)
				g.Log().Warningf(ctx, "协议号删除触发Telegram限流 tgAccountId:%d taskId:%d jobId:%d wait:%s retryAfter:%s attempt:%d/%d err:%+v", task.AccountID, task.ID, jobID, delay, retryAfter, task.AttemptCount, task.MaxAttempts, deleteErr)
				return &telegramDeleteFallbackRetryError{cause: gerror.Wrap(deleteErr, "协议号删除触发Telegram限流"), delay: retryAfter}
			}
			if telegramDeleteFallbackPermanentError(deleteErr) {
				observeTelegramDeleteFallback(ctx, "permanent_failed", task.AccountID, 0)
				g.Log().Warningf(ctx, "协议号删除永久失败 tgAccountId:%d taskId:%d jobId:%d attempt:%d/%d err:%+v", task.AccountID, task.ID, jobID, task.AttemptCount, task.MaxAttempts, deleteErr)
				s.appendTelegramJobLog(ctx, job, "delete_fallback", "failed", "协议号没有删除频道消息的权限："+deleteErr.Error())
				return nil
			}
			retryDelay := telegramDeleteFallbackRetryDelay(task.AttemptCount)
			result := "retry"
			if task.AttemptCount >= task.MaxAttempts {
				result = "dead"
			}
			observeTelegramDeleteFallback(ctx, result, task.AccountID, 0)
			g.Log().Warningf(ctx, "协议号删除执行失败 tgAccountId:%d taskId:%d jobId:%d retryAfter:%s attempt:%d/%d err:%+v", task.AccountID, task.ID, jobID, retryDelay, task.AttemptCount, task.MaxAttempts, deleteErr)
			return &telegramDeleteFallbackRetryError{
				cause: gerror.Wrap(deleteErr, "协议号删除旧消息失败"),
				delay: retryDelay,
			}
		}
		if err = markTelegramMessagesDeleted(ctx, batch.messages); err != nil {
			return err
		}
	}
	observeTelegramDeleteFallback(ctx, "success", task.AccountID, int64(len(messages)))
	g.Log().Infof(ctx, "协议号删除兜底执行成功 tgAccountId:%d taskId:%d jobId:%d messages:%d", task.AccountID, task.ID, jobID, len(messages))
	s.appendTelegramJobLog(ctx, job, "delete_fallback", "success", fmt.Sprintf("协议号删除旧消息成功，消息数:%d", len(messages)))
	return nil
}

func telegramDeleteMessageCount(ctx context.Context, jobID int64) int {
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobID).
		WhereIn("status", []string{"sent", "undeletable"}).Count()
	if err != nil {
		return 0
	}
	return count
}

func telegramDeleteFallbackPermanentError(err error) bool {
	return tgerr.Is(err, "CHAT_ADMIN_REQUIRED") ||
		tgerr.Is(err, "CHANNEL_PRIVATE") ||
		tgerr.Is(err, "MESSAGE_DELETE_FORBIDDEN") ||
		tgerr.Is(err, "MSG_ID_INVALID")
}

func telegramDeleteFallbackRetryDelay(attempt int) time.Duration {
	delays := []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	if attempt <= 0 {
		return delays[0]
	}
	if attempt > len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt-1]
}

func (s *sSysPublish) telegramJobDeleteFallbackMessages(ctx context.Context, jobID int64) ([]telegramDeleteMessage, error) {
	var rows []telegramDeleteMessage
	err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id,tg_message_id AS message_id").
		Where("job_id", jobID).
		WhereIn("status", []string{"sent", "undeletable"}).
		OrderAsc("id").Scan(&rows)
	return rows, gerror.Wrap(err, "读取协议号待删除消息失败")
}
