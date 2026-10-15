package sys

import (
	"context"
	"encoding/json"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// Keep feature metadata and per-Bot configuration atomic. Toggling the global
// plugin switch must not overwrite newer settings changed by Telegram commands.
func (s *sSysBot) persistRelayFeature(ctx context.Context, in *sysin.FeatureSaveInp, data g.Map) error {
	return dao.YoubanBotFeature.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := dao.YoubanBotFeature.Columns()
		var old *botFeatureRow
		if err := dao.YoubanBotFeature.Ctx(ctx).Where(c.FeatureKey, consts.RelayFeatureKey).LockUpdate().Scan(&old); err != nil {
			return err
		}
		if old == nil {
			return gerror.New("双向机器人插件尚未注册，请刷新插件列表")
		}
		current := &sysin.FeatureModel{ConfigValues: featureConfigValues(normalizeFeatureConfigJson(consts.RelayFeatureKey, old.ConfigJson)), ConfigSchema: featureConfigSchema(consts.RelayFeatureKey)}
		if err := s.relayFeatureSchema(ctx, current); err != nil {
			return err
		}
		toggleOnly := old.Status != data[c.Status] && (relayFeatureConfigEqual(in.ConfigJson, old.ConfigJson) || relayFeatureConfigEqual(in.ConfigJson, current.ConfigJson))
		if toggleOnly {
			data[c.ConfigJson] = old.ConfigJson
		} else if err := s.saveRelayFeatureSettings(ctx, in.ConfigJson); err != nil {
			return err
		}
		_, err := dao.YoubanBotFeature.Ctx(ctx).WherePri(old.Id).Data(data).Update()
		return gerror.Wrap(err, "保存双向机器人插件失败")
	})
}

func relayFeatureJSON(model *sysin.FeatureModel) {
	raw, _ := json.Marshal(model.ConfigValues)
	model.ConfigJson = string(raw)
}

func relayFeatureConfigEqual(left, right string) bool {
	a, _ := json.Marshal(featureConfigValues(left))
	b, _ := json.Marshal(featureConfigValues(right))
	return string(a) == string(b)
}
