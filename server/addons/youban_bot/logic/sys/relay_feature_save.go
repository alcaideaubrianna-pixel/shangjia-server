package sys

import (
	"context"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// Keep feature metadata and per-Bot configuration atomic. Toggling the global
// plugin switch must not overwrite newer settings changed by Telegram commands.
func (s *sSysBot) persistRelayFeature(ctx context.Context, in *sysin.FeatureSaveInp, data g.Map) error {
	data[dao.YoubanBotFeature.Columns().Command] = ""
	return dao.YoubanBotFeature.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := dao.YoubanBotFeature.Columns()
		var old *botFeatureRow
		if err := dao.YoubanBotFeature.Ctx(ctx).Where(c.FeatureKey, consts.RelayFeatureKey).LockUpdate().Scan(&old); err != nil {
			return err
		}
		if old == nil {
			return gerror.New("双向机器人插件尚未注册，请刷新插件列表")
		}
		if in.StatusOnly {
			if in.Status != 1 && in.Status != 2 {
				return gerror.New("插件状态不合法")
			}
			_, err := dao.YoubanBotFeature.Ctx(ctx).WherePri(old.Id).Data(do.YoubanBotFeature{Status: in.Status, ConfigJson: "{}", Command: ""}).Update()
			return gerror.Wrap(err, "更新双向插件状态失败")
		}
		// Global metadata never writes account-owned business settings.
		data[c.ConfigJson] = "{}"
		_, err := dao.YoubanBotFeature.Ctx(ctx).WherePri(old.Id).Data(data).Update()
		return gerror.Wrap(err, "保存双向机器人插件失败")
	})
}
