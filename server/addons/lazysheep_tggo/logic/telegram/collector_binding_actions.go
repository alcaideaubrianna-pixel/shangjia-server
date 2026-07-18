package telegram

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/service"
)

const collectorRevealLinksStateKey = "collector.revealInBot"
const collectorMergeVerifyGroupStateKey = "collector.mergeVerifyInGroup"
const collectorAutoPullStateKey = "collector.autoPull"
const collectorAutoPullStoppedAtKey = "collector.autoPullStoppedAt"
const collectorAutoPullStopReasonKey = "collector.autoPullStopReason"

func init() {
	RegisterBindingPluginAction("collector", "reveal_links", handleCollectorRevealLinks)
	RegisterBindingPluginAction("collector", "auto_pull", handleCollectorAutoPull)
	RegisterBindingPluginAction("collector", "merge_verify_group", handleCollectorMergeVerifyGroup)
}

func collectorRevealLinksEnabled(plugins map[string]*model.PluginConfig, bindingState map[string]any) bool {
	if bindingState != nil {
		if v, ok := bindingState[collectorRevealLinksStateKey].(bool); ok {
			return v
		}
	}
	if cfg := plugins["collector"]; cfg != nil && cfg.Settings != nil {
		if v, ok := cfg.Settings["revealInBot"].(bool); ok {
			return v
		}
	}
	return false
}

func collectorAutoPullEnabled(plugins map[string]*model.PluginConfig, bindingState map[string]any) bool {
	if bindingState != nil {
		if v, ok := bindingState[collectorAutoPullStateKey].(bool); ok {
			return v
		}
	}
	return false
}

func collectorMergeVerifyGroupEnabled(plugins map[string]*model.PluginConfig, bindingState map[string]any) bool {
	if bindingState != nil {
		if v, ok := bindingState[collectorMergeVerifyGroupStateKey].(bool); ok {
			return v
		}
	}
	return true
}

func collectorRevealLinksStatus(binding *model.BindingRecord) string {
	if binding == nil || binding.PluginState == nil {
		return "跟随全局"
	}
	if v, ok := binding.PluginState[collectorRevealLinksStateKey].(bool); ok {
		return boolText(v, "开启", "关闭")
	}
	return "跟随全局"
}

func handleCollectorRevealLinks(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil || action.Binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	next := !collectorRevealLinksEnabled(pluginConfigsForState(action.State, action.Binding.BotKey), action.Binding.PluginState)
	if action.Binding.PluginState == nil {
		action.Binding.PluginState = map[string]any{}
	}
	action.Binding.PluginState[collectorRevealLinksStateKey] = next
	if err := service.SysLazysheepTggo().SaveState(ctx, action.State); err != nil {
		return err
	}
	return refreshBindingConfigPanel(ctx, b, update, fmt.Sprintf("验证/位置机器人查看已%s。", boolText(next, "开启", "关闭")))
}

func handleCollectorAutoPull(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil || action.Binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	if action.Binding.PluginState == nil {
		action.Binding.PluginState = map[string]any{}
	}
	next := !collectorAutoPullEnabled(pluginConfigsForState(action.State, action.Binding.BotKey), action.Binding.PluginState)
	action.Binding.PluginState[collectorAutoPullStateKey] = next
	if next {
		delete(action.Binding.PluginState, collectorAutoPullStoppedAtKey)
		delete(action.Binding.PluginState, collectorAutoPullStopReasonKey)
	}
	if err := service.SysLazysheepTggo().SaveState(ctx, action.State); err != nil {
		return err
	}
	return refreshBindingConfigPanel(ctx, b, update, fmt.Sprintf("自动拉取已%s。", boolText(next, "开启", "关闭")))
}

func handleCollectorMergeVerifyGroup(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil || action.Binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	if action.Binding.PluginState == nil {
		action.Binding.PluginState = map[string]any{}
	}
	next := !collectorMergeVerifyGroupEnabled(pluginConfigsForState(action.State, action.Binding.BotKey), action.Binding.PluginState)
	action.Binding.PluginState[collectorMergeVerifyGroupStateKey] = next
	if err := service.SysLazysheepTggo().SaveState(ctx, action.State); err != nil {
		return err
	}
	return refreshBindingConfigPanel(ctx, b, update, fmt.Sprintf("验证视频合并已%s。", boolText(next, "开启", "关闭")))
}

func openBindingDefaultPanel(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	chatID := callbackChatID(update.CallbackQuery)
	if chatID == 0 {
		return replyCallback(ctx, b, update, "无法识别当前会话。")
	}
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, currentBotKey(ctx), chatID)
	if binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	isAdmin, _ := service.SysLazysheepTggo().IsBotAdmin(ctx, currentBotKey(ctx), update.CallbackQuery.From.ID)
	keyboard := buildBindingDefaultKeyboard(state, binding, isAdmin)
	if keyboard == nil {
		return replyCallback(ctx, b, update, "暂无可配置项。")
	}
	if update.CallbackQuery.Message.Message != nil {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			Text:        buildBindingConfigTextWithState(state, binding),
			ReplyMarkup: keyboard,
		})
		if err != nil {
			return err
		}
	}
	return replyCallback(ctx, b, update, "已恢复默认显示。")
}

func buildBindingDefaultKeyboard(state *model.State, binding *model.BindingRecord, isAdmin bool) *models.InlineKeyboardMarkup {
	if state == nil || binding == nil {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, 0, 8)
	if panel := buildBindingPluginKeyboard(state, binding, isAdmin, "panel"); panel != nil {
		rows = append(rows, panel.InlineKeyboard...)
	}
	if config := buildBindingPluginKeyboard(state, binding, isAdmin, "config"); config != nil {
		rows = append(rows, config.InlineKeyboard...)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回绑定配置", CallbackData: "binding:panel:back"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func pluginConfigsForState(state *model.State, botKey string) map[string]*model.PluginConfig {
	if state == nil {
		return model.DefaultPluginConfigs()
	}
	if botKey != "" {
		if cfg := state.Bots[botKey]; cfg != nil && cfg.Plugins != nil {
			return cfg.Plugins
		}
	}
	return state.Plugins
}

func collectorRevealLinksSettingText(binding *model.BindingRecord) string {
	if binding == nil {
		return "验证/位置机器人查看：跟随全局"
	}
	return "验证/位置机器人查看：" + collectorRevealLinksStatus(binding)
}

func collectorRevealLinksStateText(plugins map[string]*model.PluginConfig, binding *model.BindingRecord) string {
	return collectorRevealLinksSettingText(binding)
}

func collectorRevealLinksStateFromState(state *model.State, botKey string, binding *model.BindingRecord) bool {
	return collectorRevealLinksEnabled(pluginConfigsForState(state, botKey), binding.PluginState)
}

func buildBindingConfigTextWithState(state *model.State, binding *model.BindingRecord) string {
	if binding == nil {
		return "绑定配置"
	}
	footer := "关闭"
	if pluginStateBool(binding.PluginState, "footer.useFooter", true) {
		footer = "开启"
	}
	reveal := "开启"
	if state != nil {
		reveal = boolText(collectorRevealLinksStateFromState(state, binding.BotKey, binding), "开启", "关闭")
	}
	autoPull := boolText(collectorAutoPullEnabled(pluginConfigsForState(state, binding.BotKey), binding.PluginState), "开启", "关闭")
	mergeVerify := boolText(collectorMergeVerifyGroupEnabled(pluginConfigsForState(state, binding.BotKey), binding.PluginState), "开启", "关闭")
	autoPullDetail := ""
	if autoPull == "关闭" && binding.PluginState != nil {
		reason, _ := binding.PluginState[collectorAutoPullStopReasonKey].(string)
		stoppedAt, _ := binding.PluginState[collectorAutoPullStoppedAtKey].(string)
		if reason != "" || stoppedAt != "" {
			autoPullDetail = fmt.Sprintf("\n自动关闭原因：%s\n自动关闭时间：%s", reason, stoppedAt)
		}
	}
	return fmt.Sprintf("绑定配置\n\n当前链接：%s\n验证/位置机器人查看：%s\n自定义底部：%s\n自动拉取：%s\n验证视频合并：%s%s\n\n下面这些按钮都来自插件配置。", binding.SourceURL, reveal, footer, autoPull, mergeVerify, autoPullDetail)
}
