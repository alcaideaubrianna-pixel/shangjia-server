package sys

import (
	"context"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) AdminRelaySwitch(ctx context.Context, in *sysin.RelaySwitchInp) error {
	if err := in.Filter(); err != nil {
		return err
	}
	cfg, err := relayConfig(ctx, in.BotId)
	if err != nil {
		return err
	}
	if cfg == nil {
		return gerror.New("请先保存该Bot的双向设置")
	}
	_, err = dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).Data(do.YoubanBotRelayConfig{Enabled: in.Enabled}).Update()
	return gerror.Wrap(err, "切换双向机器人状态失败")
}
