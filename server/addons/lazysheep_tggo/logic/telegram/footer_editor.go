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
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/service"
)

type footerEditScope string

const (
	footerEditScopeGlobal  footerEditScope = "global"
	footerEditScopeBinding footerEditScope = "binding"
)

type footerEditSession struct {
	BotKey          string
	ChatID          int64
	UserID          int64
	Action          footerEditAction
	Scope           footerEditScope
	Draft           string
	PanelMessageID  int
	PromptMessageID int
}

type footerEditAction string

const (
	footerEditActionAdd  footerEditAction = "add"
	footerEditActionEdit footerEditAction = "edit"
)

var footerEditSessions sync.Map

func init() {
	RegisterMessageHandler(&footerEditInputHandler{})
	RegisterCallbackHandler(&footerEditorCallback{})
}

type footerEditInputHandler struct{}

func (h *footerEditInputHandler) Key() string              { return "footer_edit_input" }
func (h *footerEditInputHandler) Pattern() string          { return "" }
func (h *footerEditInputHandler) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *footerEditInputHandler) Description() string      { return "页脚内容输入" }
func (h *footerEditInputHandler) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil || strings.TrimSpace(msg.Text) == "" {
		return false
	}
	_, ok := getFooterEditSession(msg.Chat.ID, footerMessageUserID(msg))
	return ok
}
func (h *footerEditInputHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	session, ok := getFooterEditSession(msg.Chat.ID, footerMessageUserID(msg))
	if !ok {
		return nil
	}
	if session.BotKey != currentBotKey(ctx) {
		clearFooterEditSession(msg.Chat.ID, session.UserID)
		return nil
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		_, err := sendFooterComposePrompt(ctx, b, msg.Chat.ID, session.UserID, session.Action, session.Scope, session.PanelMessageID, msg.MessageThreadID, "内容不能为空，请重新发送。")
		return err
	}
	session.Draft = telegramMessageHTML(msg)
	storeFooterEditSession(session)
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
	if session.PromptMessageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: session.PromptMessageID})
	}
	if session.PanelMessageID > 0 {
		return openFooterDraftPanel(ctx, b, session, session.PanelMessageID)
	}
	return sendFooterDraftPanel(ctx, b, session)
}

type footerEditorCallback struct{}

func (h *footerEditorCallback) Key() string              { return "footer_editor" }
func (h *footerEditorCallback) Pattern() string          { return "footer:" }
func (h *footerEditorCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *footerEditorCallback) Description() string      { return "页脚编辑面板" }
func (h *footerEditorCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	parts := strings.Split(update.CallbackQuery.Data, ":")
	if len(parts) < 2 {
		return nil
	}
	action := parts[1]
	chatID := callbackChatID(update.CallbackQuery)
	userID := update.CallbackQuery.From.ID
	switch action {
	case "add", "edit":
		act := footerEditAction(action)
		scope := defaultFooterEditScope(ctx, currentBotKey(ctx), chatID)
		panelMessageID := callbackMessageID(update.CallbackQuery)
		if err := startFooterEdit(ctx, currentBotKey(ctx), chatID, userID, act, scope, "", panelMessageID, 0); err != nil {
			return replyCallback(ctx, b, update, fmt.Sprintf("启动编辑失败：%v", err))
		}
		if err := openFooterInputPanel(ctx, b, currentBotKey(ctx), chatID, userID, act, panelMessageID); err != nil {
			return err
		}
		if err := showFooterPrompt(ctx, b, update, act, ""); err != nil {
			return err
		}
		return nil
	case "settings":
		clearFooterEditSession(chatID, userID)
		return openBindingPluginSettingsPanel(ctx, b, update)
	case "input_back":
		clearFooterEditSession(chatID, userID)
		return openFooterEditorPanel(ctx, b, currentBotKey(ctx), chatID, userID, callbackMessageID(update.CallbackQuery))
	case "draft_back":
		session, ok := getFooterEditSession(chatID, userID)
		if !ok {
			return openFooterEditorPanel(ctx, b, currentBotKey(ctx), chatID, userID, callbackMessageID(update.CallbackQuery))
		}
		session.Draft = ""
		storeFooterEditSession(session)
		if err := openFooterInputPanel(ctx, b, session.BotKey, chatID, userID, session.Action, callbackMessageID(update.CallbackQuery)); err != nil {
			return err
		}
		if err := showFooterPrompt(ctx, b, update, session.Action, session.Scope); err != nil {
			return err
		}
		return nil
	case "save":
		if len(parts) < 3 {
			return replyCallback(ctx, b, update, "缺少保存范围。")
		}
		scope := footerEditScope(parts[2])
		if scope != footerEditScopeGlobal && scope != footerEditScopeBinding {
			return replyCallback(ctx, b, update, "未知保存范围。")
		}
		return saveFooterDraft(ctx, b, update, scope)
	case "scope":
		if len(parts) < 3 {
			return replyCallback(ctx, b, update, "缺少编辑范围。")
		}
		scope := footerEditScope(parts[2])
		if scope != footerEditScopeGlobal && scope != footerEditScopeBinding {
			return replyCallback(ctx, b, update, "未知编辑范围。")
		}
		session, ok := getFooterEditSession(chatID, userID)
		if !ok {
			return replyCallback(ctx, b, update, "编辑会话已失效，请重新打开页脚页面。")
		}
		if strings.TrimSpace(session.Draft) != "" {
			return saveFooterDraft(ctx, b, update, scope)
		}
		return replyCallback(ctx, b, update, "请先输入页脚内容。")
	case "cancel":
		if session, ok := getFooterEditSession(chatID, userID); ok && session.PromptMessageID > 0 {
			_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: session.PromptMessageID})
		}
		clearFooterEditSession(chatID, userID)
		return openFooterEditorPanel(ctx, b, currentBotKey(ctx), chatID, userID, callbackMessageID(update.CallbackQuery))
	case "back":
		clearFooterEditSession(chatID, userID)
		if err := openFooterEditorPanel(ctx, b, currentBotKey(ctx), chatID, userID, callbackMessageID(update.CallbackQuery)); err != nil {
			return err
		}
		return nil
	}
	return replyCallback(ctx, b, update, "未知页脚操作。")
}

func openFooterEditorPanel(ctx context.Context, b *bot.Bot, botKey string, chatID int64, userID int64, messageID int) error {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	binding := findBindingByChat(state, botKey, chatID)
	text := buildFooterEditorText(state, binding)
	keyboard := buildFooterEditorKeyboard(state, binding)
	if keyboard == nil {
		return nil
	}
	if messageID > 0 {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        text,
			ReplyMarkup: keyboard,
		})
		return err
	}
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

func openFooterInputPanel(ctx context.Context, b *bot.Bot, botKey string, chatID int64, userID int64, action footerEditAction, messageID int) error {
	text := "新增页脚"
	if action == footerEditActionEdit {
		text = "编辑页脚"
	}
	text += "\n\n请直接回复机器人刚发送的输入提示，发送新的页脚内容。"
	cancelText := "取消新建"
	if action == footerEditActionEdit {
		cancelText = "取消编辑"
	}
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "返回", CallbackData: "footer:input_back"}},
		{{Text: cancelText, CallbackData: "footer:cancel"}},
	}}
	if messageID > 0 {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: keyboard})
		return err
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: keyboard})
	return err
}

func showFooterPrompt(ctx context.Context, b *bot.Bot, update *models.Update, action footerEditAction, scope footerEditScope) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	chatID := callbackChatID(update.CallbackQuery)
	promptMessageID, err := sendFooterComposePrompt(ctx, b, chatID, update.CallbackQuery.From.ID, action, scope, callbackMessageID(update.CallbackQuery), callbackMessageThreadID(update.CallbackQuery), "")
	if err != nil {
		return err
	}
	if session, ok := getFooterEditSession(chatID, update.CallbackQuery.From.ID); ok {
		session.PromptMessageID = promptMessageID
		storeFooterEditSession(session)
	}
	return replyCallback(ctx, b, update, "请直接回复这条消息并发送内容。")
}

func sendFooterComposePrompt(ctx context.Context, b *bot.Bot, chatID int64, userID int64, action footerEditAction, scope footerEditScope, panelMessageID int, messageThreadID int, hint string) (int, error) {
	if chatID == 0 || userID == 0 {
		return 0, fmt.Errorf("缺少编辑上下文")
	}
	text := footerComposeText(ctx, b, currentBotKey(ctx), chatID, action, scope)
	if strings.TrimSpace(hint) != "" {
		text += "\n\n" + strings.TrimSpace(hint)
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:              chatID,
		MessageThreadID:     messageThreadID,
		Text:                text,
		ReplyMarkup:         &models.ForceReply{ForceReply: true, Selective: true, InputFieldPlaceholder: "直接发送页脚内容"},
		DisableNotification: false,
	})
	if err != nil || sent == nil {
		return 0, err
	}
	if session, ok := getFooterEditSession(chatID, userID); ok {
		session.PanelMessageID = panelMessageID
		session.PromptMessageID = sent.ID
		storeFooterEditSession(session)
	}
	return sent.ID, nil
}

func buildFooterEditorText(state *model.State, binding *model.BindingRecord) string {
	globalText := ""
	if state != nil && state.Plugins != nil {
		if cfg := state.Plugins["footer"]; cfg != nil {
			globalText = settingString(cfg.Settings, "footerText", "")
		}
	}
	bindingText := "未单独设置，当前跟随全局。"
	if binding != nil {
		if v := settingString(binding.PluginState, "footer.text", ""); v != "" {
			bindingText = v
		}
	}
	lines := []string{
		"自定义底部",
		"",
		"点击新增或编辑后，直接发送新的页脚内容。",
		"",
		"全局页脚：" + displayFooterText(globalText),
		"当前频道：" + displayFooterText(bindingText),
	}
	if binding != nil {
		lines = append(lines, "", "当前会话可直接编辑当前频道覆盖。")
	}
	return strings.Join(lines, "\n")
}

func buildFooterEditorKeyboard(state *model.State, binding *model.BindingRecord) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, 2)
	actionText := "新增页脚"
	if footerConfigured(state, binding) {
		actionText = "编辑页脚"
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: actionText, CallbackData: footerActionCallback(actionText)},
	})
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "返回配置设置", CallbackData: "footer:settings"},
	})
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "取消", CallbackData: "footer:settings"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func footerActionCallback(actionText string) string {
	if actionText == "编辑页脚" {
		return "footer:edit"
	}
	return "footer:add"
}

func footerConfigured(state *model.State, binding *model.BindingRecord) bool {
	if binding != nil && strings.TrimSpace(settingString(binding.PluginState, "footer.text", "")) != "" {
		return true
	}
	if state != nil && state.Plugins != nil {
		if cfg := state.Plugins["footer"]; cfg != nil && strings.TrimSpace(settingString(cfg.Settings, "footerText", "")) != "" {
			return true
		}
	}
	return false
}

func footerComposeText(ctx context.Context, b *bot.Bot, botKey string, chatID int64, action footerEditAction, scope footerEditScope) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return "请直接发送页脚内容。"
	}
	binding := findBindingByChat(state, botKey, chatID)
	header := "新增页脚"
	if action == footerEditActionEdit {
		header = "编辑页脚"
	}
	target := "待选择保存范围"
	current := ""
	if scope == footerEditScopeBinding {
		target = "当前频道"
		if binding != nil {
			current = settingString(binding.PluginState, "footer.text", "")
		}
	} else if state != nil && state.Plugins != nil {
		if cfg := state.Plugins["footer"]; cfg != nil {
			current = settingString(cfg.Settings, "footerText", "")
		}
	}
	lines := []string{
		header + " / " + target,
		"",
		"请直接回复这条消息并发送新的页脚内容。",
	}
	if action == footerEditActionEdit {
		lines = append(lines, "", "当前内容："+displayFooterText(current))
	}
	return strings.Join(lines, "\n")
}

func defaultFooterEditScope(ctx context.Context, botKey string, chatID int64) footerEditScope {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err == nil && findBindingByChat(state, botKey, chatID) != nil {
		return footerEditScopeBinding
	}
	return footerEditScopeGlobal
}

func startFooterEdit(ctx context.Context, botKey string, chatID int64, userID int64, action footerEditAction, scope footerEditScope, draft string, panelMessageID int, promptMessageID int) error {
	if botKey == "" || chatID == 0 || userID == 0 {
		return fmt.Errorf("缺少编辑上下文")
	}
	storeFooterEditSession(footerEditSession{
		BotKey:          botKey,
		ChatID:          chatID,
		UserID:          userID,
		Action:          action,
		Scope:           scope,
		Draft:           strings.TrimSpace(draft),
		PanelMessageID:  panelMessageID,
		PromptMessageID: promptMessageID,
	})
	return nil
}

func storeFooterEditSession(session footerEditSession) {
	footerEditSessions.Store(footerSessionKey(session.ChatID, session.UserID), session)
	footerEditSessions.Store(footerSessionKey(session.ChatID, 0), session)
}

func getFooterEditSession(chatID, userID int64) (footerEditSession, bool) {
	raw, ok := footerEditSessions.Load(footerSessionKey(chatID, userID))
	if !ok && userID != 0 {
		raw, ok = footerEditSessions.Load(footerSessionKey(chatID, 0))
	}
	if !ok {
		return footerEditSession{}, false
	}
	session, ok := raw.(footerEditSession)
	return session, ok
}

func clearFooterEditSession(chatID, userID int64) {
	footerEditSessions.Delete(footerSessionKey(chatID, userID))
	footerEditSessions.Delete(footerSessionKey(chatID, 0))
}

func footerSessionKey(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
}

func footerMessageUserID(msg *models.Message) int64 {
	if msg == nil || msg.From == nil {
		return 0
	}
	return msg.From.ID
}

func updateGlobalFooter(state *model.State, text string) error {
	if state == nil {
		return fmt.Errorf("状态不存在")
	}
	if state.Plugins == nil {
		state.Plugins = model.DefaultPluginConfigs()
	}
	plugin := state.Plugins["footer"]
	if plugin == nil {
		plugin = model.DefaultPluginConfigs()["footer"]
		state.Plugins["footer"] = plugin
	}
	if plugin.Settings == nil {
		plugin.Settings = map[string]any{}
	}
	plugin.Settings["footerText"] = strings.TrimSpace(text)
	return nil
}

func updateBindingFooter(state *model.State, botKey string, chatID int64, text string) error {
	if state == nil {
		return fmt.Errorf("状态不存在")
	}
	binding := findBindingByChat(state, botKey, chatID)
	if binding == nil {
		return fmt.Errorf("未找到当前会话的绑定关系")
	}
	if binding.PluginState == nil {
		binding.PluginState = map[string]any{}
	}
	binding.PluginState["footer.text"] = strings.TrimSpace(text)
	binding.PluginState["footer.useFooter"] = true
	return nil
}

func displayFooterText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "未配置"
	}
	return text
}

func callbackMessageID(callback *models.CallbackQuery) int {
	if callback == nil {
		return 0
	}
	if callback.Message.Message != nil {
		return callback.Message.Message.ID
	}
	return 0
}

func callbackMessageThreadID(callback *models.CallbackQuery) int {
	if callback == nil {
		return 0
	}
	if callback.Message.Message != nil {
		return callback.Message.Message.MessageThreadID
	}
	return 0
}
