package sys

import (
	"context"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
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

func (f *relayGatewayFeature) Key() string { return consts.RelayFeatureKey }

// Run before the dedicated two-way UI feature so pending relay usernames can
// be activated by /start. Unbound messages return false and keep the original
// two-way provider/feature flow unchanged. Gateway priorities are descending.
func (f *relayGatewayFeature) Priority() int { return 150 }
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
			if update != nil && update.Message != nil && isOriginalTwoWayMenuInput(update.Message.Text) {
				return false, nil
			}
			if update != nil && update.Message != nil && !relayGatewayCommand(update.Message.Text) {
				bound, boundErr := f.bot.relayGatewayEndpointBound(ctx, binding.ReferenceID, update.Message)
				if boundErr != nil || !bound {
					return false, boundErr
				}
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

func isOriginalTwoWayMenuInput(text string) bool {
	return strings.TrimSpace(text) == "双向聊天"
}

// relayGatewayEndpointBound keeps the original two-way provider in control of
// unbound chats while allowing configured relay endpoints to use the extension.
// Username endpoints are accepted for /start activation; subsequent messages
// use the resolved Telegram chat ID. Group endpoints use the chat ID directly.
func (s *sSysBot) relayGatewayEndpointBound(ctx context.Context, botID int64, msg *models.Message) (bool, error) {
	if msg == nil || msg.Chat.ID == 0 {
		return false, nil
	}
	c := dao.YoubanBotRelayEndpoint.Columns()
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	var endpoint struct{ Id int64 }
	if err := dao.YoubanBotRelayEndpoint.Ctx(ctx).
		Where(c.BotId, botID).
		Where(c.ChatId, chatID).
		Scan(&endpoint); err != nil {
		return false, err
	}
	if endpoint.Id > 0 {
		return true, nil
	}
	if msg.From == nil || msg.From.Username == "" || msg.Chat.Type != models.ChatTypePrivate {
		return false, nil
	}
	if err := dao.YoubanBotRelayEndpoint.Ctx(ctx).
		Where(do.YoubanBotRelayEndpoint{BotId: botID, ChatId: "@" + strings.ToLower(msg.From.Username)}).
		Scan(&endpoint); err != nil {
		return false, err
	}
	return endpoint.Id > 0, nil
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
