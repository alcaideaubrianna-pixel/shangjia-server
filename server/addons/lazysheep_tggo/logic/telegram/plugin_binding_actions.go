// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/service"
)

func init() {
	RegisterBindingPluginAction("collector", "verify_link", handleCollectorVerifyLink)
	RegisterBindingPluginAction("collector", "location_link", handleCollectorLocationLink)
	RegisterBindingPluginAction("footer", "toggle", handleFooterToggle)
	RegisterBindingPluginAction("footer", "edit", handleFooterEdit)
	RegisterMessageHandler(&footerCommand{})
	RegisterMessageHandler(&bindingQuickConfigCommand{})
	RegisterCallbackHandler(&bindingPluginActionCallback{})
	RegisterCallbackHandler(&bindingConfigCallback{})
}

type BindingPluginActionContext struct {
	BotKey    string
	ChatID    int64
	UserID    int64
	MessageID int
	State     *model.State
	Binding   *model.BindingRecord
}

type BindingPluginActionHandler func(context.Context, *bot.Bot, *models.Update, *BindingPluginActionContext) error

var bindingPluginActionHandlers = map[string]BindingPluginActionHandler{}

func RegisterBindingPluginAction(pluginKey, actionKey string, handler BindingPluginActionHandler) {
	if strings.TrimSpace(pluginKey) == "" || strings.TrimSpace(actionKey) == "" || handler == nil {
		return
	}
	bindingPluginActionHandlers[pluginKey+":"+actionKey] = handler
}

type bindingQuickConfigCommand struct{}

func (h *bindingQuickConfigCommand) Key() string              { return "binding_quick_config" }
func (h *bindingQuickConfigCommand) Pattern() string          { return "" }
func (h *bindingQuickConfigCommand) MatchType() bot.MatchType { return bot.MatchTypeContains }
func (h *bindingQuickConfigCommand) Description() string      { return "绑定配置快捷命令" }
func (h *bindingQuickConfigCommand) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil {
		return false
	}
	_, ok := parseBindingQuickConfigCommand(msg.Text)
	return ok
}
func (h *bindingQuickConfigCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	action, ok := parseBindingQuickConfigCommand(msg.Text)
	if !ok {
		return nil
	}
	userID := userIDFromMessage(msg)
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	botKey := currentBotKey(ctx)
	cfg := state.Bots[botKey]
	if cfg == nil || cfg.MemberId == 0 || cfg.MemberId != userID {
		sent, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "只有机器人创建者可以修改绑定配置。",
		})
		if sendErr == nil && sent != nil {
			deleteMessageLater(b, msg.Chat.ID, sent.ID)
		}
		return sendErr
	}
	binding := findBindingByChat(state, botKey, msg.Chat.ID)
	if binding == nil {
		sent, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "当前会话还没有绑定配置。",
		})
		if sendErr == nil && sent != nil {
			deleteMessageLater(b, msg.Chat.ID, sent.ID)
		}
		return sendErr
	}
	text := applyBindingQuickConfig(action, binding)
	if err = service.SysLazysheepTggo().SaveState(ctx, state); err != nil {
		return err
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        text + "\n\n" + buildBindingQuickConfigSummary(state, binding),
		ReplyMarkup: buildBindingQuickConfigKeyboard(),
	})
	if err == nil && sent != nil {
		deleteMessageLater(b, msg.Chat.ID, sent.ID)
	}
	return err
}

type footerCommand struct{}

func (h *footerCommand) Key() string              { return "footer" }
func (h *footerCommand) Pattern() string          { return "footer" }
func (h *footerCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *footerCommand) Description() string      { return "编辑内容底栏" }
func (h *footerCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "footer") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	return openFooterEditorPanel(ctx, b, currentBotKey(ctx), msg.Chat.ID, userIDFromMessage(msg), 0)
}

type bindingPluginActionCallback struct{}

func (h *bindingPluginActionCallback) Key() string              { return "binding_plugin_action" }
func (h *bindingPluginActionCallback) Pattern() string          { return "plugin:" }
func (h *bindingPluginActionCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *bindingPluginActionCallback) Description() string      { return "绑定插件动作" }
func (h *bindingPluginActionCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	parts := strings.Split(update.CallbackQuery.Data, ":")
	if len(parts) < 3 {
		return nil
	}
	if err := replyCallback(ctx, b, update, "正在更新配置..."); err != nil {
		return err
	}
	ctx = WithCallbackAnswered(ctx)
	pluginKey := parts[1]
	actionKey := parts[2]
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	chatID := callbackChatID(update.CallbackQuery)
	binding := findBindingByChat(state, currentBotKey(ctx), chatID)
	if binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	if binding.PluginState == nil {
		binding.PluginState = map[string]any{}
	}
	handler := bindingPluginActionHandlers[pluginKey+":"+actionKey]
	if handler == nil {
		return replyCallback(ctx, b, update, "该插件动作暂未实现。")
	}
	return handler(ctx, b, update, &BindingPluginActionContext{
		BotKey:    currentBotKey(ctx),
		ChatID:    chatID,
		UserID:    update.CallbackQuery.From.ID,
		MessageID: callbackMessageID(update.CallbackQuery),
		State:     state,
		Binding:   binding,
	})
}

type bindingConfigCallback struct{}

func (h *bindingConfigCallback) Key() string              { return "binding_config" }
func (h *bindingConfigCallback) Pattern() string          { return "binding:" }
func (h *bindingConfigCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *bindingConfigCallback) Description() string      { return "绑定配置面板" }
func (h *bindingConfigCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	parts := strings.Split(update.CallbackQuery.Data, ":")
	if len(parts) < 3 {
		return nil
	}
	action := parts[1]
	chatID := callbackChatID(update.CallbackQuery)
	if chatID == 0 {
		return replyCallback(ctx, b, update, "无法识别当前会话。")
	}
	switch action {
	case "footer":
		if err := toggleBindingFooter(ctx, chatID, currentBotKey(ctx)); err != nil {
			return replyCallback(ctx, b, update, fmt.Sprintf("切换页脚失败：%v", err))
		}
		if err := refreshBindingConfigPanel(ctx, b, update, "页脚已更新。"); err != nil {
			return err
		}
		return nil
	case "refresh":
		return refreshBindingConfigPanel(ctx, b, update, "配置已刷新。")
	case "config":
		return openBindingPluginSettingsPanel(ctx, b, update)
	case "default":
		return openBindingDefaultPanel(ctx, b, update)
	case "panel":
		return refreshBindingConfigPanel(ctx, b, update, "已返回绑定配置。")
	case "pull":
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "请在当前会话输入 /pull 或 /pull 10 立即采集。",
			ShowAlert:       false,
		})
		return err
	}
	return replyCallback(ctx, b, update, "未知配置操作。")
}

func sendBindingPluginActions(ctx context.Context, b *bot.Bot, botKey string, chatID int64, userID int64) error {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, botKey, chatID)
	if binding == nil {
		return nil
	}
	isAdmin, err := service.SysLazysheepTggo().IsBotAdmin(ctx, botKey, userID)
	if err != nil {
		isAdmin = false
	}
	keyboard := buildBindingPluginKeyboard(state, binding, isAdmin, "panel")
	if keyboard == nil {
		return nil
	}
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "已启用的插件动作：",
		ReplyMarkup: keyboard,
	})
	return err
}

func sendBindingConfigPanel(ctx context.Context, b *bot.Bot, botKey string, chatID int64, userID int64) error {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, botKey, chatID)
	if binding == nil {
		return nil
	}
	isAdmin, err := service.SysLazysheepTggo().IsBotAdmin(ctx, botKey, userID)
	if err != nil {
		isAdmin = false
	}
	keyboard := buildBindingConfigKeyboard(state, binding, isAdmin)
	if keyboard == nil {
		return nil
	}
	text := buildBindingConfigTextWithState(state, binding)
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	if err == nil && sent != nil {
		deleteMessageLater(b, chatID, sent.ID)
	}
	return err
}

func sendBindingQuickConfigKeyboard(ctx context.Context, b *bot.Bot, chatID int64) error {
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "快捷配置已打开。点击底部按钮即可修改，只有机器人创建者可以操作。",
		ReplyMarkup: buildBindingQuickConfigKeyboard(),
	})
	if err == nil && sent != nil {
		deleteMessageLater(b, chatID, sent.ID)
	}
	return err
}

func buildBindingQuickConfigKeyboard() *models.ReplyKeyboardMarkup {
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{{Text: "机器人查看开"}, {Text: "机器人查看关"}},
			{{Text: "页脚开"}, {Text: "页脚关"}},
			{{Text: "自动拉取开"}, {Text: "自动拉取关"}},
		},
		ResizeKeyboard:        true,
		InputFieldPlaceholder: "选择配置或直接输入：自动拉取开",
	}
}

func buildBindingPluginKeyboard(state *model.State, binding *model.BindingRecord, isAdmin bool, placement string) *models.InlineKeyboardMarkup {
	if state == nil || binding == nil {
		return nil
	}
	plugins := state.Plugins
	if botCfg := state.Bots[binding.BotKey]; botCfg != nil && botCfg.Plugins != nil {
		plugins = botCfg.Plugins
	}
	rows := make([][]models.InlineKeyboardButton, 0)
	for _, plugin := range plugins {
		if plugin == nil || !plugin.Enabled || !plugin.VisibleInBinding || len(plugin.BindingActions) == 0 {
			continue
		}
		for _, action := range plugin.BindingActions {
			if plugin.Key == "collector" && (action.Key == "showVerifyLink" || action.Key == "showLocationLink") {
				continue
			}
			if !action.Visible {
				continue
			}
			if action.AdminOnly && !isAdmin {
				continue
			}
			if bindingActionPlacement(action) != placement {
				continue
			}
			label := resolveBindingActionLabel(plugin, action, binding)
			if label == "" {
				continue
			}
			callback := strings.TrimSpace(action.Callback)
			if callback == "" {
				callback = fmt.Sprintf("%s:%s", plugin.Key, action.Key)
			}
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         label,
				CallbackData: fmt.Sprintf("plugin:%s", callback),
			}})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildBindingConfigKeyboard(state *model.State, binding *model.BindingRecord, isAdmin bool) *models.InlineKeyboardMarkup {
	if state == nil || binding == nil {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, 0, 6)
	pluginKeyboard := buildBindingPluginKeyboard(state, binding, isAdmin, "panel")
	if pluginKeyboard != nil {
		rows = append(rows, pluginKeyboard.InlineKeyboard...)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "恢复默认", CallbackData: "binding:default:view"},
		{Text: "刷新配置", CallbackData: "binding:refresh:now"},
		{Text: "配置设置", CallbackData: "binding:config:settings"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func openBindingPluginSettingsPanel(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	chatID := callbackChatID(update.CallbackQuery)
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, currentBotKey(ctx), chatID)
	if binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	isAdmin, _ := service.SysLazysheepTggo().IsBotAdmin(ctx, currentBotKey(ctx), update.CallbackQuery.From.ID)
	keyboard := buildBindingPluginSettingsKeyboard(state, binding, isAdmin)
	if keyboard == nil {
		return replyCallback(ctx, b, update, "暂无插件配置项。")
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        buildBindingPluginSettingsText(binding),
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return err
	}
	if sent != nil {
		deleteMessageLater(b, chatID, sent.ID)
	}
	return replyCallback(ctx, b, update, "已打开配置设置。")
}

func buildBindingPluginSettingsKeyboard(state *model.State, binding *model.BindingRecord, isAdmin bool) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, 4)
	pluginKeyboard := buildBindingPluginKeyboard(state, binding, isAdmin, "config")
	if pluginKeyboard != nil {
		rows = append(rows, pluginKeyboard.InlineKeyboard...)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "返回绑定配置", CallbackData: "binding:panel:back"},
	})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "刷新配置", CallbackData: "binding:config:settings"}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "取消", CallbackData: "binding:panel:back"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildBindingPluginSettingsText(binding *model.BindingRecord) string {
	if binding == nil {
		return "配置设置"
	}
	return fmt.Sprintf("配置设置\n\n当前链接：%s\n\n这里显示当前已启用插件提供的配置入口。", binding.SourceURL)
}

func buildBindingConfigText(binding *model.BindingRecord) string {
	if binding == nil {
		return "绑定配置"
	}
	footer := "关闭"
	if pluginStateBool(binding.PluginState, "footer.useFooter", true) {
		footer = "开启"
	}
	reveal := boolText(collectorRevealLinksEnabled(nil, binding.PluginState), "开启", "关闭")
	autoPull := boolText(collectorAutoPullEnabled(nil, binding.PluginState), "开启", "关闭")
	return fmt.Sprintf("绑定配置\n\n当前链接：%s\n验证/位置机器人查看：%s\n自定义底部：%s\n自动拉取：%s\n\n下面这些按钮都来自插件配置。", binding.SourceURL, reveal, footer, autoPull)
}

func toggleBindingFooter(ctx context.Context, chatID int64, botKey string) error {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, botKey, chatID)
	if binding == nil {
		return fmt.Errorf("未找到当前绑定")
	}
	if binding.PluginState == nil {
		binding.PluginState = map[string]any{}
	}
	next := !pluginStateBool(binding.PluginState, "footer.useFooter", true)
	binding.PluginState["footer.useFooter"] = next
	return service.SysLazysheepTggo().SaveState(ctx, state)
}

func handleFooterToggle(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil || action.Binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	next := !pluginStateBool(action.Binding.PluginState, "footer.useFooter", true)
	action.Binding.PluginState["footer.useFooter"] = next
	if err := service.SysLazysheepTggo().SaveState(ctx, action.State); err != nil {
		return err
	}
	return refreshBindingConfigPanel(ctx, b, update, fmt.Sprintf("页脚已%s。", boolText(next, "开启", "关闭")))
}

func handleFooterEdit(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil {
		return replyCallback(ctx, b, update, "缺少配置上下文。")
	}
	return openFooterEditorPanel(ctx, b, action.BotKey, action.ChatID, action.UserID, action.MessageID)
}

func handleCollectorVerifyLink(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil || action.Binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	next := !action.Binding.VerifyEnabled
	action.Binding.VerifyEnabled = next
	if err := service.SysLazysheepTggo().SaveState(ctx, action.State); err != nil {
		return err
	}
	return refreshBindingConfigPanel(ctx, b, update, fmt.Sprintf("查看验证已%s。", boolText(next, "开启", "关闭")))
}

func handleCollectorLocationLink(ctx context.Context, b *bot.Bot, update *models.Update, action *BindingPluginActionContext) error {
	if action == nil || action.Binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	next := !action.Binding.LocationEnabled
	action.Binding.LocationEnabled = next
	if err := service.SysLazysheepTggo().SaveState(ctx, action.State); err != nil {
		return err
	}
	return refreshBindingConfigPanel(ctx, b, update, fmt.Sprintf("查看位置已%s。", boolText(next, "开启", "关闭")))
}

func refreshBindingConfigPanel(ctx context.Context, b *bot.Bot, update *models.Update, alert string) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	chatID := callbackChatID(update.CallbackQuery)
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, currentBotKey(ctx), chatID)
	if binding == nil {
		return replyCallback(ctx, b, update, "绑定关系不存在。")
	}
	isAdmin, _ := service.SysLazysheepTggo().IsBotAdmin(ctx, currentBotKey(ctx), update.CallbackQuery.From.ID)
	keyboard := buildBindingConfigKeyboard(state, binding, isAdmin)
	if keyboard == nil {
		return replyCallback(ctx, b, update, "暂无可配置项。")
	}
	if update.CallbackQuery.Message.Message != nil {
		if _, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			Text:        buildBindingConfigTextWithState(state, binding),
			ReplyMarkup: keyboard,
		}); err != nil {
			if isTelegramMessageNotModified(err) {
				return replyCallback(ctx, b, update, alert)
			}
			g.Log().Warningf(ctx, "刷新绑定配置面板失败 bot:%s chat:%d err:%+v", currentBotKey(ctx), chatID, err)
			return replyCallback(ctx, b, update, alert+" 配置已保存，但面板刷新失败，请点“刷新配置”。")
		}
	}
	return replyCallback(ctx, b, update, alert)
}

func isTelegramMessageNotModified(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

func callbackChatID(callback *models.CallbackQuery) int64 {
	if callback == nil {
		return 0
	}
	if callback.Message.Message != nil {
		return callback.Message.Message.Chat.ID
	}
	if callback.Message.InaccessibleMessage != nil {
		return callback.Message.InaccessibleMessage.Chat.ID
	}
	return 0
}

func resolveBindingActionLabel(plugin *model.PluginConfig, action model.PluginBindingAction, binding *model.BindingRecord) string {
	pluginKey := ""
	if plugin != nil {
		pluginKey = plugin.Key
	}
	switch pluginKey {
	case "footer":
		if action.Key == "useFooter" {
			enabled := pluginStateBool(binding.PluginState, "footer.useFooter", boolDefault(action.Default, true))
			return fmt.Sprintf("%s：%s", action.Label, boolText(enabled, "开", "关"))
		}
	case "collector":
		if action.Key == "showVerifyLink" {
			return fmt.Sprintf("%s：%s", action.Label, boolText(binding.VerifyEnabled, "开", "关"))
		}
		if action.Key == "showLocationLink" {
			return fmt.Sprintf("%s：%s", action.Label, boolText(binding.LocationEnabled, "开", "关"))
		}
		if action.Key == "revealInBot" {
			return fmt.Sprintf("%s：%s", action.Label, boolText(collectorRevealLinksEnabled(map[string]*model.PluginConfig{pluginKey: plugin}, binding.PluginState), "开", "关"))
		}
		if action.Key == "mergeVerifyInGroup" {
			return fmt.Sprintf("%s：%s", action.Label, boolText(collectorMergeVerifyGroupEnabled(map[string]*model.PluginConfig{pluginKey: plugin}, binding.PluginState), "开", "关"))
		}
		if action.Key == "autoPull" {
			return fmt.Sprintf("%s：%s", action.Label, boolText(collectorAutoPullEnabled(nil, binding.PluginState), "开", "关"))
		}
	}
	return action.Label
}

type bindingQuickConfigAction struct {
	Key     string
	Enable  bool
	Message string
}

func parseBindingQuickConfigCommand(text string) (bindingQuickConfigAction, bool) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "：", ":"))
	text = strings.TrimPrefix(text, "/")
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "=", "")
	text = strings.ReplaceAll(text, ":", "")
	replacer := strings.NewReplacer("开启", "开", "打开", "开", "启用", "开", "关闭", "关", "禁用", "关")
	text = replacer.Replace(text)
	switch text {
	case "机器人查看开", "验证位置机器人查看开":
		return bindingQuickConfigAction{Key: "reveal", Enable: true, Message: "验证/位置机器人查看已开启。"}, true
	case "机器人查看关", "验证位置机器人查看关":
		return bindingQuickConfigAction{Key: "reveal", Enable: false, Message: "验证/位置机器人查看已关闭。"}, true
	case "页脚开", "自定义底部开":
		return bindingQuickConfigAction{Key: "footer", Enable: true, Message: "页脚已开启。"}, true
	case "页脚关", "自定义底部关":
		return bindingQuickConfigAction{Key: "footer", Enable: false, Message: "页脚已关闭。"}, true
	case "自动拉取开":
		return bindingQuickConfigAction{Key: "auto_pull", Enable: true, Message: "自动拉取已开启。"}, true
	case "自动拉取关":
		return bindingQuickConfigAction{Key: "auto_pull", Enable: false, Message: "自动拉取已关闭。"}, true
	}
	return bindingQuickConfigAction{}, false
}

func applyBindingQuickConfig(action bindingQuickConfigAction, binding *model.BindingRecord) string {
	if binding.PluginState == nil {
		binding.PluginState = map[string]any{}
	}
	switch action.Key {
	case "reveal":
		binding.PluginState[collectorRevealLinksStateKey] = action.Enable
	case "footer":
		binding.PluginState["footer.useFooter"] = action.Enable
	case "auto_pull":
		binding.PluginState[collectorAutoPullStateKey] = action.Enable
	}
	return action.Message
}

func buildBindingQuickConfigSummary(state *model.State, binding *model.BindingRecord) string {
	if binding == nil {
		return ""
	}
	reveal := boolText(collectorRevealLinksStateFromState(state, binding.BotKey, binding), "开", "关")
	footer := boolText(pluginStateBool(binding.PluginState, "footer.useFooter", true), "开", "关")
	autoPull := boolText(collectorAutoPullEnabled(pluginConfigsForState(state, binding.BotKey), binding.PluginState), "开", "关")
	return fmt.Sprintf("当前：机器人查看%s｜页脚%s｜自动拉取%s", reveal, footer, autoPull)
}

func isBotCreator(state *model.State, botKey string, userID int64) bool {
	if state == nil || strings.TrimSpace(botKey) == "" || userID == 0 {
		return false
	}
	cfg := state.Bots[botKey]
	return cfg != nil && cfg.MemberId == userID
}

func bindingActionPlacement(action model.PluginBindingAction) string {
	placement := strings.TrimSpace(action.Placement)
	if placement != "" {
		return placement
	}
	if action.Kind == "button" {
		return "config"
	}
	return "panel"
}

func findBindingByChat(state *model.State, botKey string, chatID int64) *model.BindingRecord {
	if state == nil {
		return nil
	}
	for _, item := range state.Bindings {
		if item == nil || item.BotKey != botKey {
			continue
		}
		if item.ReviewChatID == chatID || item.PublishChatID == chatID {
			return item
		}
	}
	return nil
}

func pluginStateBool(state map[string]any, key string, fallback bool) bool {
	if state == nil {
		return fallback
	}
	if v, ok := state[key].(bool); ok {
		return v
	}
	return fallback
}

func boolDefault(v any, fallback bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

func boolText(v bool, yesText, noText string) string {
	if v {
		return yesText
	}
	return noText
}
