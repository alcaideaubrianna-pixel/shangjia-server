package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/errors/gerror"
	"time"
)

func (s *sSysPublish) uploadRelayMediaNow(ctx context.Context, media *telegramMediaItem, purpose string) ([]*telegramSentMessage, error) {
	channel, err := chooseRelayChannel(ctx, media.TenantId)
	if err != nil || channel == nil {
		return nil, err
	}
	token, err := claimRelayMedia(ctx, media.TenantId, media.Id, purpose, "original", 10*time.Minute)
	if err != nil || token == "" {
		return nil, err
	}
	markRelayChannelUse(ctx, channel.Id, 1)
	defer markRelayChannelUse(ctx, channel.Id, -1)
	client, err := s.TelegramClient(ctx, channel.TgAccountId)
	if err != nil {
		markRelayChannelFailure(ctx, channel.Id, err, time.Minute)
		_ = failRelayMedia(ctx, media.TenantId, media.Id, purpose, "original", token, err)
		return nil, err
	}
	peer, err := s.collectMediaInputPeer(ctx, media.TenantId, channel.TgAccountId, client, channel.TargetChatId)
	if err != nil {
		markRelayChannelFailure(ctx, channel.Id, err, time.Minute)
		_ = failRelayMedia(ctx, media.TenantId, media.Id, purpose, "original", token, err)
		return nil, err
	}
	item := *media
	item.AntiScanEnabled = false
	item.ForceUpload = false
	messages, err := s.sendMessageTemplateWithTgClient(ctx, client, peer, "", []*telegramMediaItem{&item}, nil, channel.TgAccountId, fmt.Sprintf("relay:%d", media.Id))
	if err != nil || len(messages) == 0 {
		if err == nil {
			err = gerror.New("relay 上传未返回消息")
		}
		markRelayChannelFailure(ctx, channel.Id, err, time.Minute)
		_ = failRelayMedia(ctx, media.TenantId, media.Id, purpose, "original", token, err)
		return nil, err
	}
	ref := relayMediaRef{RelayChannelId: channel.Id, RelayChatId: channel.TargetChatId, MessageId: messages[0].MessageId}
	if err = completeRelayMedia(ctx, media.TenantId, media.Id, purpose, "original", token, ref); err != nil {
		return nil, err
	}
	messages[0].TgFileId = channel.TargetChatId
	return messages, nil
}

func (s *sSysPublish) uploadRelayMediaSetNow(ctx context.Context, media []*telegramMediaItem, purpose string) ([]*telegramSentMessage, error) {
	if len(media) == 0 || media[0] == nil || media[0].TenantId <= 0 {
		return nil, gerror.New("relay 媒体组参数无效")
	}
	channel, err := chooseRelayChannel(ctx, media[0].TenantId)
	if err != nil || channel == nil {
		return nil, err
	}
	client, err := s.TelegramClient(ctx, channel.TgAccountId)
	if err != nil {
		markRelayChannelFailure(ctx, channel.Id, err, time.Minute)
		return nil, err
	}
	peer, err := s.collectMediaInputPeer(ctx, media[0].TenantId, channel.TgAccountId, client, channel.TargetChatId)
	if err != nil {
		markRelayChannelFailure(ctx, channel.Id, err, time.Minute)
		return nil, err
	}
	for _, item := range media {
		if item == nil || item.TenantId != media[0].TenantId {
			return nil, gerror.New("relay 媒体组租户不一致")
		}
	}
	markRelayChannelUse(ctx, channel.Id, len(media))
	defer markRelayChannelUse(ctx, channel.Id, -len(media))
	items := make([]*telegramMediaItem, 0, len(media))
	tokens := make([]string, 0, len(media))
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		for i, token := range tokens {
			// Successful completion has already cleared the token; only unfinished claims match.
			_ = failRelayMedia(cleanupCtx, media[i].TenantId, media[i].Id, purpose, "original", token, gerror.New("中转媒体组准备结束，释放未完成租约"))
		}
	}()
	for _, source := range media {
		token, claimErr := claimRelayMedia(ctx, source.TenantId, source.Id, purpose, "original", 10*time.Minute)
		if claimErr != nil || token == "" {
			return nil, claimErr
		}
		clone := *source
		clone.AntiScanEnabled = false
		clone.ForceUpload = false
		items = append(items, &clone)
		tokens = append(tokens, token)
	}
	messages, err := s.sendMessageTemplateWithTgClient(ctx, client, peer, "", items, nil, channel.TgAccountId, fmt.Sprintf("relay-group:%d", media[0].Id))
	if err != nil || len(messages) != len(media) {
		if err == nil {
			err = gerror.New("relay 媒体组上传结果不完整")
		}
		markRelayChannelFailure(ctx, channel.Id, err, time.Minute)
		return nil, err
	}
	for i, message := range messages {
		if err = completeRelayMedia(ctx, media[i].TenantId, media[i].Id, purpose, "original", tokens[i], relayMediaRef{RelayChannelId: channel.Id, RelayChatId: channel.TargetChatId, MessageId: message.MessageId}); err != nil {
			return nil, err
		}
		message.TgFileId = channel.TargetChatId
	}
	return messages, nil
}
