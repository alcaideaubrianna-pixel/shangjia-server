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
	AllRules     bool    `json:"allRules,omitempty"`
	OperationID  string  `json:"operationId,omitempty"`
	NotifyChatID string  `json:"notifyChatId,omitempty"`
	ProfileIDs   []int64 `json:"profileIds,omitempty"`
	JobIDs       []int64 `json:"jobIds,omitempty"`
	MessageCount int     `json:"messageCount,omitempty"`
	Title        string  `json:"title,omitempty"`
	Link         string  `json:"link,omitempty"`
	Suffix       string  `json:"suffix,omitempty"`
	TenantID     int64   `json:"tenantId"`
	AccountID    int64   `json:"accountId"`
	RuleID       int64   `json:"ruleId"`
	BotID        int64   `json:"botId"`
	ChannelID    string  `json:"channelId"`
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
	options := []asynq.Option{asynq.Queue(tgQueueNameBackground), asynq.MaxRetry(10), asynq.Timeout(30 * time.Minute), asynq.TaskID(fmt.Sprintf("collect-channel-purge:%d:%d:%s", payload.TenantID, payload.AccountID, payload.ChannelID))}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectChannelPurge, body), options...)
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return fmt.Errorf("该频道删除任务正在执行，请勿重复提交")
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
