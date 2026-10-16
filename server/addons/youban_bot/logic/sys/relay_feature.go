package sys

import (
	"context"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	"github.com/go-telegram/bot/models"
)

type relayFeature struct{}

func (relayFeature) Key() string         { return consts.RelayFeatureKey }
func (relayFeature) Command() string     { return "" }
func (relayFeature) Description() string { return "双向机器人" }
func (relayFeature) Handle(ctx context.Context, s *sSysBot, fc *botFeatureContext) (bool, error) {
	return s.handleRelayUpdate(ctx, fc.BotId, &models.Update{Message: fc.Msg})
}
func (relayFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{}

}

// Gateway interception runs before other interactive features can consume relay messages.
// The existing Bot binding is reused; no second polling client or Bot token is introduced.
type relayGatewayFeature struct{ bot *sSysBot }

func (f *relayGatewayFeature) Key() string   { return consts.RelayFeatureKey }
func (f *relayGatewayFeature) Priority() int { return 1000 }
func (f *relayGatewayFeature) Menus(ctx context.Context, bc gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	for _, binding := range bc.Bindings {
		if binding.Owner == relayTwoWayBotOwner {
			cfg, err := f.bot.relaySourceIdentity(ctx, binding.ReferenceID)
			if err != nil || cfg == nil || cfg.Enabled != 1 {
				return gatewayservice.FeatureMenus{}, err
			}
			return gatewayservice.FeatureMenus{Managed: true, Items: []gatewayservice.MenuItem{{Command: "start", Description: "开始双向聊天", Order: 0}, {Command: "help", Description: "双向管理帮助（仅管理员）", Order: 1}, {Command: "id", Description: "查看聊天ID", Order: 2}}}, nil
		}
		if binding.Owner == "youban_bot" {
			items := make([]gatewayservice.MenuItem, 0)
			for i, feature := range botFeatures {
				if feature == nil || !f.bot.featureMenuVisible(ctx, feature.Key()) {
					continue
				}
				cmd := strings.TrimPrefix(f.bot.featureCommand(ctx, feature), "/")
				if cmd != "" {
					items = append(items, gatewayservice.MenuItem{Command: cmd, Description: f.bot.featureDescription(ctx, feature), Order: i})
				}
			}
			return gatewayservice.FeatureMenus{Managed: true, Items: items}, nil
		}
	}
	return gatewayservice.FeatureMenus{}, nil
}
func (f *relayGatewayFeature) HandleUpdate(ctx context.Context, bc gatewayservice.BotContext, update *models.Update) (bool, error) {
	for _, binding := range bc.Bindings {
		if binding.Owner == "youban_bot" {
			if update != nil && update.CallbackQuery != nil {
				if handled, err := f.bot.handleRelayManagementCallback(ctx, binding.ReferenceID, update.CallbackQuery); handled || err != nil {
					return handled, err
				}
			}
			return false, nil
		}
		if binding.Owner == relayTwoWayBotOwner {
			// The dedicated two-way provider owns the original chat/topic flow.
			// This extension may consume only its explicit management commands;
			// ordinary messages must continue to the original provider.
			if update != nil && update.Message != nil && !relayGatewayCommand(update.Message.Text) {
				return false, nil
			}
			cfg, err := f.bot.relaySourceIdentity(ctx, binding.ReferenceID)
			if err != nil || cfg == nil || cfg.Enabled != 1 {
				return false, err
			}
			handled, err := f.bot.handleRelaySourceUpdate(ctx, binding.ReferenceID, update)
			if handled && err != nil && update != nil && update.Message != nil {
				return true, f.bot.relayInteractionError(ctx, binding.ReferenceID, update.Message, err)
			}
			return handled, err
		}
	}
	return false, nil
}

func relayGatewayCommand(text string) bool {
	command := parseRelayCommand(text)
	if command.Name == "/help" || command.Name == "/id" {
		return true
	}
	for _, name := range []string{
		"绑定", "解绑", "确认解绑", "绑定列表", "添加管理员", "删除管理员", "管理员列表",
		"屏蔽", "删除屏蔽", "屏蔽用户", "解除屏蔽用户", "屏蔽群", "解除屏蔽群",
		"屏蔽列表", "黑名单", "屏蔽跳转", "当前设置", "私聊用户", "群聊列表",
		"审计设置", "聊天存档", "转发群", "关闭转发",
	} {
		if command.Name == name {
			return true
		}
	}
	return false
}
