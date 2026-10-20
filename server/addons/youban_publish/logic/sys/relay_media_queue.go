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
	if ready, err := relayReadyMedia(ctx, media.TenantId, media.Id, purpose, "original"); err != nil {
		return nil, err
	} else if ready != nil {
		return []*telegramSentMessage{{MessageId: ready.MessageId, TgFileId: ready.RelayChatId}}, nil
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

type relayMediaGroupTaskPayload struct {
	Media   []*telegramMediaItem `json:"media"`
	Purpose string               `json:"purpose"`
}

func (s *sSysPublish) prepareRelayMediaSet(ctx context.Context, media []*telegramMediaItem, purpose string) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, gerror.New("relay 媒体组为空")
	}
	readyRefs := make([]*telegramSentMessage, len(media))
	allReady := true
	for i, item := range media {
		ref, err := relayReadyMedia(ctx, item.TenantId, item.Id, purpose, "original")
		if err != nil {
			return nil, err
		}
		if ref == nil {
			allReady = false
			break
		}
		readyRefs[i] = &telegramSentMessage{MessageId: ref.MessageId, TgFileId: ref.RelayChatId}
	}
	if allReady {
		return readyRefs, nil
	}
	body, err := json.Marshal(relayMediaGroupTaskPayload{Media: media, Purpose: purpose})
	if err != nil {
		return nil, err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeRelayMediaGroup, body), asynq.Queue(tgQueueNameRelay), asynq.MaxRetry(5), asynq.Timeout(10*time.Minute)); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		refs := make([]*telegramSentMessage, len(media))
		ready := true
		for i, item := range media {
			ref, readErr := relayReadyMedia(ctx, item.TenantId, item.Id, purpose, "original")
			if readErr != nil {
				return nil, readErr
			}
			if ref == nil {
				ready = false
				break
			}
			refs[i] = &telegramSentMessage{MessageId: ref.MessageId, TgFileId: ref.RelayChatId}
		}
		if ready {
			return refs, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return nil, gerror.New("relay 媒体组上传等待超时")
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

func (s *sSysPublish) handleRelayMediaGroupTask(ctx context.Context, task *asynq.Task) error {
	var payload relayMediaGroupTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	_, err := s.uploadRelayMediaSetNow(ctx, payload.Media, payload.Purpose)
	return err
}
