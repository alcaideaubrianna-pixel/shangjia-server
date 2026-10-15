package sys

import (
	"context"
	"encoding/json"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
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
			_, err := dao.YoubanBotFeature.Ctx(ctx).WherePri(old.Id).Data(do.YoubanBotFeature{Status: in.Status}).Update()
			return gerror.Wrap(err, "更新双向插件状态失败")
		}
		current := &sysin.FeatureModel{ConfigValues: featureConfigValues(normalizeFeatureConfigJson(consts.RelayFeatureKey, old.ConfigJson)), ConfigSchema: featureConfigSchema(consts.RelayFeatureKey)}
		settings, _, err := relaySettingsFromFeature(in.ConfigJson)
		if err != nil {
			return err
		}
		if err := relayLockBot(ctx, settings.BotId); err != nil {
			return err
		}
		var locked *entity.YoubanBotRelayConfig
		if err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, settings.BotId).LockUpdate().Scan(&locked); err != nil {
			return err
		}
		incoming := featureConfigValues(in.ConfigJson)
		current.ConfigValues["botId"] = incoming["botId"]
		if err := s.relayFeatureSchema(ctx, current); err != nil {
			return err
		}
		if incoming["revision"] == nil || incoming["revision"] == "" || incoming["revision"] != current.ConfigValues["revision"] {
			return gerror.New("配置已变化或尚未加载，请重新选择Bot后再保存")
		}
		if err := s.saveRelayFeatureSettings(ctx, in.ConfigJson); err != nil {
			return err
		}

		_, err = dao.YoubanBotFeature.Ctx(ctx).WherePri(old.Id).Data(data).Update()
		return gerror.Wrap(err, "保存双向机器人插件失败")
	})
}

func relayFeatureJSON(model *sysin.FeatureModel) {
	raw, _ := json.Marshal(model.ConfigValues)
	model.ConfigJson = string(raw)
}
