package sys

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// telegramChannelQueueEnabled is deliberately opt-in while the legacy Asynq
// transport remains live. This prevents two consumers from sending the same
// job during migration.
func telegramChannelQueueEnabled() bool {
	v := strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED"))
	b, _ := strconv.ParseBool(v)
	return b
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
	if err != nil || jobID <= 0 {
		return false, err
	}
	if err = s.SendTelegramJob(ctx, jobID); err != nil {
		// Keep the job available for the next attempt. A bounded delay avoids a
		// hot loop when the channel lease or Telegram API is temporarily busy.
		_ = enqueueTelegramChannelJob(ctx, channelID, jobID, time.Now().Add(time.Second).UnixMilli())
		return true, err
	}
	// SendTelegramJob may return nil when another sender owns the lease. Do
	// not lose the popped item in that case; only terminal states are acked.
	if job, loadErr := s.telegramJobById(ctx, jobID); loadErr == nil {
		switch job.Status {
		case telegramJobStatusSent, "failed", "superseded":
			return true, nil
		default:
			_ = enqueueTelegramChannelJob(ctx, channelID, jobID, time.Now().Add(time.Second).UnixMilli())
		}
	} else {
		_ = enqueueTelegramChannelJob(ctx, channelID, jobID, time.Now().Add(time.Second).UnixMilli())
	}
	return true, nil
}
