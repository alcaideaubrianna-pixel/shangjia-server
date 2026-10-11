package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/errors/gerror"
	"time"
)

func (s *sSysPublish) prepareRelayMedia(ctx context.Context, media *telegramMediaItem, purpose string) ([]*telegramSentMessage, error) {
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
