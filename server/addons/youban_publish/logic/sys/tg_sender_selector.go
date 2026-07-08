package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
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
	value, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", job.ChannelId).
		Where("tenant_id", job.TenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		Fields("bot_id_json").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取频道备用BOT失败")
	}
	botIds := decodeBotIds(value.String())
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
