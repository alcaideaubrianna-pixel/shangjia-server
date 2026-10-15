package sys

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
)

type relayFeature struct{}

func (relayFeature) Key() string         { return consts.RelayFeatureKey }
func (relayFeature) Command() string     { return "" }
func (relayFeature) Description() string { return "双向机器人" }
func (relayFeature) Handle(ctx context.Context, s *sSysBot, fc *botFeatureContext) (bool, error) {
	return s.handleRelayUpdate(ctx, fc.BotId, &models.Update{Message: fc.Msg})
}
func (relayFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "botId", Label: "应用到Bot", Component: "select", Default: 0, Placeholder: "保存将把当前表单应用到所选Bot"},
		{Field: "enabled", Label: "双向功能", Component: "switch", Default: 0},
		{Field: "admins", Label: "添加管理员", Component: "textarea", Default: "", Placeholder: "每行一个用户ID或@用户名；@用户名需先到此Bot私聊发送 /start 确认身份。默认管理员为首次保存配置的后台账号，其TG账号需已绑定；所有管理员需先私聊此Bot才能接收通知"},
		{Field: "keywords", Label: "屏蔽词", Component: "textarea", Default: "", Placeholder: "每行一个关键词；保存后替换所选Bot的关键词列表"},
		{Field: "bindings", Label: "双向列表", Component: "textarea", Default: "", Placeholder: "每行一对对象，空格分隔，如：@alice @bob；使用群ID时也用空格。保存后替换所选Bot的绑定列表"},
		{Field: "blacklistUsers", Label: "黑名单用户", Component: "textarea", Default: "", Placeholder: "每行一个用户ID或@用户名；保存后替换所选Bot的用户黑名单"},
		{Field: "blacklistGroups", Label: "黑名单群聊", Component: "textarea", Default: "", Placeholder: "每行一个群ID或@群用户名；保存后替换所选Bot的群黑名单"},
		{Field: "blockLinks", Label: "屏蔽跳转", Component: "switch", Default: 0},
		{Field: "auditDatabase", Label: "保存聊天记录", Component: "switch", Default: 1},
		{Field: "auditGroupId", Label: "审计转发群", Component: "input", Default: "", Placeholder: "可留空；仅支持开启话题的超级群ID，Bot需有管理话题权限"},
	}
}

// Gateway interception runs before other interactive features can consume relay messages.
// The existing Bot binding is reused; no second polling client or Bot token is introduced.
type relayGatewayFeature struct{ bot *sSysBot }

func (f *relayGatewayFeature) Key() string   { return consts.RelayFeatureKey }
func (f *relayGatewayFeature) Priority() int { return 1000 }
func (f *relayGatewayFeature) Menus(ctx context.Context, bc gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	for _, binding := range bc.Bindings {
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
			cfg, err := f.bot.relayIdentity(ctx, binding.ReferenceID)
			if err == nil && cfg != nil {
				items = []gatewayservice.MenuItem{{Command: "start", Description: "开始双向聊天", Order: 0}, {Command: "help", Description: "双向管理帮助（仅管理员）", Order: 1}, {Command: "id", Description: "查看聊天ID", Order: 2}}
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
				cfg, err := f.bot.relayIdentity(ctx, binding.ReferenceID)
				if err == nil && cfg != nil {
					return true, f.bot.relayExpiredButton(ctx, binding.ReferenceID, update.CallbackQuery)
				}
			}
			handled, err := f.bot.handleRelayUpdate(ctx, binding.ReferenceID, update)
			if handled && err != nil && update != nil && update.Message != nil {
				return true, f.bot.relayInteractionError(ctx, binding.ReferenceID, update.Message, err)
			}
			return handled, err
		}
	}
	return false, nil
}

func relayConfigLines(value interface{}) []string {
	text, _ := value.(string)
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == '，' })
	out := make([]string, 0, len(lines))
	for _, v := range lines {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func relaySettingsFromFeature(configJSON string) (*sysin.RelaySettingsInp, []string, error) {
	values := featureConfigValues(configJSON)
	number := func(key string) int {
		v := values[key]
		if n, ok := v.(float64); ok {
			return int(n)
		}
		if n, ok := v.(int); ok {
			return n
		}
		if str, ok := v.(string); ok {
			n, _ := strconv.Atoi(str)
			return n
		}
		return 0
	}
	in := &sysin.RelaySettingsInp{RelayBotInp: sysin.RelayBotInp{BotId: int64(number("botId"))}, Enabled: number("enabled"), BlockLinks: number("blockLinks"), AuditDatabase: number("auditDatabase"), Admins: relayConfigLines(values["admins"])}
	in.AuditGroupId, _ = values["auditGroupId"].(string)
	if in.BotId == 0 {
		return nil, nil, gerror.New("请选择需要配置的Bot")
	}
	if err := in.Filter(); err != nil {
		return nil, nil, err
	}
	words := relayConfigLines(values["keywords"])
	for _, word := range words {
		r := &sysin.RelayRuleInp{RelayBotInp: in.RelayBotInp, Kind: consts.RelayRuleKeyword, Value: word}
		if err := r.Filter(); err != nil {
			return nil, nil, err
		}
	}
	if len(words) > 500 {
		return nil, nil, gerror.New("最多配置500个屏蔽词")
	}
	return in, words, nil
}

func (s *sSysBot) relayFeatureSchema(ctx context.Context, model *sysin.FeatureModel) error {
	bots, err := s.enabledAndDisabledRelayBots(ctx)
	if err != nil {
		return err
	}
	for _, field := range model.ConfigSchema {
		if field.Field == "botId" {
			for _, bot := range bots {
				if bot.IsDefault == 1 {
					continue
				}
				field.Options = append(field.Options, &sysin.FeatureConfigOption{Label: bot.BotName + " @" + bot.BotUsername, Value: bot.Id})
			}
		}
	}
	// Read back the selected Bot's authoritative settings so chat commands are visible here.
	raw, _ := json.Marshal(model.ConfigValues)
	in, _, err := relaySettingsFromFeature(string(raw))
	if err != nil {
		return nil
	}
	cfg, err := s.AdminRelaySettings(ctx, &in.RelayBotInp)
	if err != nil {
		return err
	}
	model.ConfigValues["enabled"] = cfg.Enabled
	model.ConfigValues["blockLinks"] = cfg.BlockLinks
	model.ConfigValues["auditDatabase"] = cfg.AuditDatabase
	model.ConfigValues["auditGroupId"] = cfg.AuditGroupId
	admins := []string{}
	for _, a := range cfg.Admins {
		if a.UserId != "" {
			admins = append(admins, a.UserId)
		} else {
			admins = append(admins, a.Username)
		}
	}
	model.ConfigValues["admins"] = strings.Join(admins, "\n")
	words, err := s.relayKeywords(ctx, in.BotId)
	if err != nil {
		return err
	}
	model.ConfigValues["keywords"] = strings.Join(words, "\n")
	if err := s.relayFeatureRelations(ctx, in.BotId, model); err != nil {
		return err
	}
	delete(model.ConfigValues, "revision")
	model.ConfigValues["loadedBotId"] = in.BotId
	snapshot, _ := json.Marshal(model.ConfigValues)
	model.ConfigValues["revision"] = fmt.Sprintf("%x", sha256.Sum256(snapshot))
	relayFeatureJSON(model)
	return nil
}
