package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

func telegramChannelSenderBotId(channel telegramJobChannel) (int64, error) {
	botId := firstPositiveId(decodeBotIds(channel.BotIdJson))
	if botId <= 0 {
		return 0, gerror.New("目标频道未配置可用推送BOT")
	}
	return botId, nil
}

func (s *sSysPublish) nextTelegramChannelBotId(ctx context.Context, job telegramJobRecord) (int64, error) {
	if job.ChannelId <= 0 || job.BotId <= 0 {
		return 0, nil
	}
	channel, err := s.currentTelegramJobChannel(ctx, job)
	if err != nil {
		return 0, err
	}
	if channel.Status != 1 {
		return 0, nil
	}
	botIds := decodeBotIds(channel.BotIdJson)
	foundCurrent := false
	for _, botId := range botIds {
		if botId <= 0 {
			continue
		}
		if foundCurrent {
			return botId, nil
		}
		if botId == job.BotId {
			foundCurrent = true
		}
	}
	return 0, nil
}
