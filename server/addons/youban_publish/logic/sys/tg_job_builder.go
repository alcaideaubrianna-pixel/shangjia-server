package sys

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

var errNoTelegramPublishChannels = errors.New("未配置可推送频道")

func (s *sSysPublish) telegramJobChannels(ctx context.Context, task gdb.Record, targetChannelIds ...[]int64) ([]telegramJobChannel, error) {
	var channelIds []int64
	if len(targetChannelIds) > 0 {
		channelIds = uniqueIds(targetChannelIds[0])
		if len(channelIds) == 0 {
			return nil, errNoTelegramPublishChannels
		}
	} else {
		var err error
		channelIds, err = s.profileChannelIdsOrDefaults(ctx, task["tenant_id"].Int64(), task["account_id"].Int64(), task["profile_id"].Int64())
		if err != nil {
			return nil, err
		}
	}
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", task["tenant_id"].Int64()).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at")
	if len(channelIds) > 0 {
		mod = mod.WhereIn("id", channelIds)
	}
	var channels []telegramJobChannel
	if err := mod.Fields("id,target_chat_id,bot_id_json").OrderAsc("id").Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取TG推送频道失败")
	}
	if len(channels) == 0 {
		return nil, errNoTelegramPublishChannels
	}
	return channels, nil
}

func decodeInt64JSON(raw string) []int64 {
	var ids []int64
	_ = json.Unmarshal([]byte(raw), &ids)
	return uniqueIds(ids)
}

func firstPositiveId(ids []int64) int64 {
	for _, id := range ids {
		if id > 0 {
			return id
		}
	}
	return 0
}

func defaultCycleDays(days int) int {
	if days <= 0 {
		return 4
	}
	return days
}

type telegramJobChannel struct {
	Id           int64  `json:"id"`
	TargetChatId string `json:"targetChatId"`
	BotIdJson    string `json:"botIdJson"`
}
