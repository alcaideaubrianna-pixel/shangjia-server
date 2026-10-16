package sys

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
)

// Deletion is addressed by exact channel/message IDs. Never use whole-job
// deletion for a duplicate: it would also remove the canonical publication.
func (s *sSysPublish) enqueueTelegramDuplicateMessages(ctx context.Context, job telegramJobRecord, messages []*telegramSentMessage) error {
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if message == nil || message.MessageId <= 0 {
			continue
		}
		_, err := s.enqueueTelegramAutoDeleteFallback(ctx, &autoDeleteChannel{
			Id: job.ChannelId, TenantId: job.TenantId, TgAccountId: channel.TgAccountId, TargetChatId: job.TargetChatId,
		}, int(message.MessageId), nil)
		if err != nil {
			return err
		}
		g.Log().Warningf(ctx, "TG迟到重复消息已提交精确删除 jobId:%d channelId:%d messageId:%d", job.Id, job.ChannelId, message.MessageId)
	}
	return nil
}
