package sys

import "github.com/gogf/gf/v2/errors/gerror"

func telegramChannelSenderBotId(channel telegramJobChannel) (int64, error) {
	botId := firstPositiveId(decodeBotIds(channel.BotIdJson))
	if botId <= 0 {
		return 0, gerror.New("目标频道未配置可用推送BOT")
	}
	return botId, nil
}
