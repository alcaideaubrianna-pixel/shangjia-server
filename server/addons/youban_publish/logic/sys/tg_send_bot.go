package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
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

func (s *sSysPublish) telegramCleanupJobBotToken(ctx context.Context, botId int64, tenantId int64) (string, error) {
	if botId <= 0 {
		return s.telegramJobBotToken(ctx, botId, tenantId)
	}
	var bot *sysin.BotModel
	mod := g.DB().Model(publishBotTable).Safe().Ctx(ctx).Where("id", botId)
	if tenantId > 0 {
		mod = mod.Where("tenant_id IN(0, ?)", tenantId)
	}
	if err := mod.Scan(&bot); err != nil {
		return "", gerror.Wrap(err, "读取历史Bot配置失败")
	}
	if bot == nil || bot.Id <= 0 {
		return "", gerror.New("Bot配置不存在")
	}
	if strings.TrimSpace(bot.BotToken) == "" {
		return "", gerror.New("历史Bot Token未配置")
	}
	return strings.TrimSpace(bot.BotToken), nil
}
