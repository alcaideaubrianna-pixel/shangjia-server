package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const tgTaskTypeCollectChannelPurge = "youban_publish:collect:channel_purge"

type collectChannelPurgeQueuePayload struct {
	TenantID  int64  `json:"tenantId"`
	AccountID int64  `json:"accountId"`
	RuleID    int64  `json:"ruleId"`
	BotID     int64  `json:"botId"`
	ChannelID string `json:"channelId"`
}

func (s *sSysPublish) enqueueCollectChannelPurge(ctx context.Context, payload collectChannelPurgeQueuePayload, delay time.Duration) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	options := []asynq.Option{asynq.Queue(tgQueueNameBackground), asynq.MaxRetry(10), asynq.Timeout(30 * time.Minute), asynq.Unique(30 * time.Second)}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectChannelPurge, body), options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func decodeCollectChannelPurgeQueuePayload(task *asynq.Task) (collectChannelPurgeQueuePayload, error) {
	var payload collectChannelPurgeQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	if payload.TenantID <= 0 || payload.AccountID <= 0 || payload.RuleID <= 0 || payload.ChannelID == "" {
		return payload, fmt.Errorf("频道下架任务参数不完整")
	}
	return payload, nil
}
