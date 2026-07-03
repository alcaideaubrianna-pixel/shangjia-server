package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysPublish) telegramJobBotToken(ctx context.Context, botId int64, tenantId int64) (string, error) {
	if botId > 0 {
		bot, err := s.getBotById(ctx, botId, tenantId)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(bot.BotToken) == "" {
			return "", gerror.New("Bot Token未配置")
		}
		return strings.TrimSpace(bot.BotToken), nil
	}
	bots, err := s.enabledBots(ctx, tenantId)
	if err != nil {
		return "", err
	}
	if len(bots) == 0 && tenantId > 0 {
		bots, err = s.enabledBots(ctx, 0)
		if err != nil {
			return "", err
		}
	}
	if len(bots) == 0 || bots[0] == nil || strings.TrimSpace(bots[0].BotToken) == "" {
		return "", gerror.New("未配置可用Bot")
	}
	return strings.TrimSpace(bots[0].BotToken), nil
}
