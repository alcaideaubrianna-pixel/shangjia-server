package sys

import (
	"context"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) enabledAndDisabledRelayBots(ctx context.Context) ([]*sysin.BotModel, error) {
	var rows []*sysin.BotModel
	c := dao.YoubanBotBot.Columns()
	err := dao.YoubanBotBot.Ctx(ctx).Fields(c.Id, c.BotName, c.BotUsername, c.Status).OrderAsc(c.Id).Scan(&rows)
	return rows, gerror.Wrap(err, "读取双向Bot选项失败")
}
func (s *sSysBot) relayKeywords(ctx context.Context, botID int64) ([]string, error) {
	var rows []*entity.YoubanBotRelayRule
	err := dao.YoubanBotRelayRule.Ctx(ctx).Where(do.YoubanBotRelayRule{BotId: botID, Kind: consts.RelayRuleKeyword}).OrderAsc(dao.YoubanBotRelayRule.Columns().Id).Scan(&rows)
	words := make([]string, 0, len(rows))
	for _, r := range rows {
		words = append(words, r.Label)
	}
	return words, gerror.Wrap(err, "读取屏蔽词失败")
}
func (s *sSysBot) saveRelayFeatureSettings(ctx context.Context, configJSON string) error {
	in, words, err := relaySettingsFromFeature(configJSON)
	if err != nil {
		return err
	}
	return dao.YoubanBotRelayConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := s.AdminRelaySaveSettings(ctx, in); err != nil {
			return err
		}
		if _, err := dao.YoubanBotRelayRule.Ctx(ctx).Where(do.YoubanBotRelayRule{BotId: in.BotId, Kind: consts.RelayRuleKeyword}).Delete(); err != nil {
			return err
		}
		for _, word := range words {
			if err := s.AdminRelaySaveRule(ctx, &sysin.RelayRuleInp{RelayBotInp: in.RelayBotInp, Kind: consts.RelayRuleKeyword, Value: word}); err != nil {
				return err
			}
		}
		if err := s.saveRelayFeatureRelations(ctx, in.BotId, featureConfigValues(configJSON)); err != nil {
			return err
		}
		return nil
	})
}
