package sys

import (
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/hibiken/asynq"
	"time"
)

type relayMediaTaskPayload struct {
	Media   telegramMediaItem `json:"media"`
	Purpose string            `json:"purpose"`
}

func (s *sSysPublish) enqueueRelayMedia(ctx context.Context, media *telegramMediaItem, purpose string) error {
	if media == nil {
		return gerror.New("relay 媒体为空")
	}
	body, err := json.Marshal(relayMediaTaskPayload{Media: *media, Purpose: purpose})
	if err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeRelayMedia, body), asynq.Queue(tgQueueNameRelay), asynq.MaxRetry(5), asynq.Timeout(10*time.Minute))
	return err
}

func (s *sSysPublish) handleRelayMediaTask(ctx context.Context, task *asynq.Task) error {
	var payload relayMediaTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	_, err := s.prepareRelayMedia(ctx, &payload.Media, payload.Purpose)
	return err
}
