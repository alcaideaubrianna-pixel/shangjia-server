package sys

import (
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/hibiken/asynq"
	"time"
)

func (s *sSysPublish) prepareRelayMedia(ctx context.Context, media *telegramMediaItem, purpose string) ([]*telegramSentMessage, error) {
	if media == nil {
		return nil, gerror.New("relay 媒体为空")
	}
	if ready, err := relayReadyMedia(ctx, media.TenantId, media.Id, purpose, "original"); err != nil || ready != nil {
		return nil, err
	}
	if err := s.enqueueRelayMedia(ctx, media, purpose); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if ready, err := relayReadyMedia(ctx, media.TenantId, media.Id, purpose, "original"); err != nil {
			return nil, err
		} else if ready != nil {
			return []*telegramSentMessage{{MessageId: ready.MessageId, TgFileId: ready.RelayChatId}}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return nil, gerror.New("relay 媒体上传等待超时")
}

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
	_, err := s.uploadRelayMediaNow(ctx, &payload.Media, payload.Purpose)
	return err
}
