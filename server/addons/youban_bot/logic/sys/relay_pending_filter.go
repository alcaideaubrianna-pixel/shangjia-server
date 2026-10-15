package sys

import (
	"context"
	"encoding/json"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

	"github.com/go-telegram/bot/models"
)

func relayPendingFilter(ctx context.Context, cfg *entity.YoubanBotRelayConfig, rows []*entity.YoubanBotRelayDelivery) (string, error) {
	var rules []*entity.YoubanBotRelayRule
	if err := dao.YoubanBotRelayRule.Ctx(ctx).Where(dao.YoubanBotRelayRule.Columns().BotId, cfg.BotId).Scan(&rules); err != nil {
		return "", err
	}
	for _, row := range rows {
		var msg models.Message
		if err := json.Unmarshal([]byte(row.RawJson), &msg); err != nil {
			return "", err
		}
		if reason := relayFilterReason(cfg, rules, &msg, row.TargetChatId); reason != "" {
			return reason, nil
		}
	}
	first := rows[0]
	if first.MediaGroupId != "" {
		var members []*entity.YoubanBotRelayDelivery
		err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(do.YoubanBotRelayDelivery{BotId: first.BotId, SourceChatId: first.SourceChatId, MediaGroupId: first.MediaGroupId, TargetChatId: first.TargetChatId, Kind: consts.RelayKindChat}).Scan(&members)
		if err != nil {
			return "", err
		}
		for _, m := range members {
			if m.Status == consts.RelayCancelled && m.ErrorMessage != "" {
				return "媒体组包含已拦截消息，整组未传递。", nil
			}
			var msg models.Message
			if err := json.Unmarshal([]byte(m.RawJson), &msg); err != nil {
				return "", err
			}
			if reason := relayFilterReason(cfg, rules, &msg, m.TargetChatId); reason != "" {
				return reason, nil
			}
		}
	}
	return "", nil
}
