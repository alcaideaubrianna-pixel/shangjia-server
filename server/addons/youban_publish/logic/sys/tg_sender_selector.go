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

// telegramChannelSenderBotIdForProfile spreads independent profile jobs across
// the configured Bots while keeping one profile stable on the same Bot. A
// failed or rate-limited Bot is handled by nextTelegramChannelBotId.
func telegramChannelSenderBotIdForProfile(channel telegramJobChannel, profileId int64) (int64, error) {
	botIds := make([]int64, 0, len(decodeBotIds(channel.BotIdJson)))
	for _, botId := range decodeBotIds(channel.BotIdJson) {
		if botId > 0 {
			botIds = append(botIds, botId)
		}
	}
	if len(botIds) == 0 {
		return 0, gerror.New("目标频道未配置可用推送BOT")
	}
	if profileId <= 0 {
		return botIds[0], nil
	}
	return botIds[int(profileId%int64(len(botIds)))], nil
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
