package sys

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

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
	jobID, err := dequeueTelegramChannelHead(ctx, channelID)
	if err != nil {
		g.Log().Warning(ctx, "频道队列取头失败", g.Map{"event": "channel_queue_error", "stage": "dequeue", "channelId": channelID, "error": err.Error()})
		return false, err
	}
	if jobID <= 0 {
		return false, err
	}
	job, loadErr := s.telegramJobById(ctx, jobID)
	if loadErr != nil {
		g.Log().Warning(ctx, "频道队列任务加载失败", g.Map{"event": "channel_queue_error", "stage": "load", "channelId": channelID, "jobId": jobID, "error": loadErr.Error()})
		_ = enqueueTelegramChannelJob(ctx, channelID, jobID, time.Now().Add(time.Second).UnixMilli())
		return true, loadErr
	}
	g.Log().Info(ctx, "频道队列取出任务", g.Map{"event": "channel_queue_dequeue", "channelId": channelID, "jobId": jobID, "profileId": job.ProfileId, "operationNo": job.OperationNo, "queue": telegramChannelQueueKey(channelID)})
	if err = s.SendTelegramJob(ctx, jobID); err != nil {
		// Keep the job available for the next attempt. A bounded delay avoids a
		// hot loop when the channel lease or Telegram API is temporarily busy.
		hold := time.Now().Add(time.Second).UnixMilli()
		_ = enqueueTelegramChannelJob(ctx, channelID, jobID, hold)
		g.Log().Warning(ctx, "频道队列任务发送失败，已重入队", g.Map{"event": "channel_queue_requeue", "stage": "send", "channelId": channelID, "jobId": jobID, "profileId": job.ProfileId, "score": hold, "error": err.Error()})
		return true, err
	}
	// SendTelegramJob may return nil when another sender owns the lease. Do
	// not lose the popped item in that case; only terminal states are acked.
	if job, loadErr := s.telegramJobById(ctx, jobID); loadErr == nil {
		switch job.Status {
		case telegramJobStatusSent, "failed", "superseded":
			g.Log().Info(ctx, "频道队列任务确认完成", g.Map{"event": "channel_queue_ack", "channelId": channelID, "jobId": jobID, "profileId": job.ProfileId, "status": job.Status})
			return true, nil
		default:
			hold := time.Now().Add(time.Second).UnixMilli()
			_ = enqueueTelegramChannelJob(ctx, channelID, jobID, hold)
			reason := job.LastDispatchError
			if strings.Contains(reason, "媒体仍在异步处理中") {
				g.Log().Info(ctx, "频道队列任务等待媒体就绪", g.Map{"event": "channel_queue_media_not_ready", "channelId": channelID, "jobId": jobID, "profileId": job.ProfileId, "score": hold})
			}
			g.Log().Warning(ctx, "频道队列任务未完成，已重入队", g.Map{"event": "channel_queue_requeue", "stage": "post_send", "channelId": channelID, "jobId": jobID, "profileId": job.ProfileId, "status": job.Status, "score": hold, "reason": reason})
		}
	} else {
		hold := time.Now().Add(time.Second).UnixMilli()
		_ = enqueueTelegramChannelJob(ctx, channelID, jobID, hold)
		g.Log().Warning(ctx, "频道队列任务状态读取失败，已重入队", g.Map{"event": "channel_queue_requeue", "stage": "reload", "channelId": channelID, "jobId": jobID, "score": hold, "error": loadErr.Error()})
	}
	return true, nil
}
