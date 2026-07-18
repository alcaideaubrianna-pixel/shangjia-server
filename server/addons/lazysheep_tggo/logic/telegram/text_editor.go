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

type textEditTarget struct {
	Key         string
	PluginKey   string
	SettingKey  string
	Title       string
	Placeholder string
	BackData    string
}

type textEditSession struct {
	BotKey          string
	ChatID          int64
	UserID          int64
	Target          textEditTarget
	Draft           string
	PanelMessageID  int
	PromptMessageID int
}

var textEditSessions sync.Map

func init() {
	RegisterMessageHandler(&textEditInputHandler{})
	RegisterCallbackHandler(&textEditorCallback{})
}

type textEditInputHandler struct{}

func (h *textEditInputHandler) Key() string              { return "text_edit_input" }
func (h *textEditInputHandler) Pattern() string          { return "" }
func (h *textEditInputHandler) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *textEditInputHandler) Description() string      { return "通用文案输入" }
func (h *textEditInputHandler) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil || strings.TrimSpace(msg.Text) == "" {
		return false
	}
	_, ok := getTextEditSession(msg.Chat.ID, textEditMessageUserID(msg))
	return ok
}
func (h *textEditInputHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	session, ok := getTextEditSession(msg.Chat.ID, textEditMessageUserID(msg))
	if !ok {
		return nil
	}
	if session.BotKey != currentBotKey(ctx) {
		clearTextEditSession(msg.Chat.ID, session.UserID)
		return nil
	}
	if strings.TrimSpace(msg.Text) == "" {
		_, err := sendTextEditPrompt(ctx, b, session, msg.MessageThreadID, "内容不能为空，请重新发送。")
		return err
	}
	session.Draft = telegramMessageHTML(msg)
	storeTextEditSession(session)
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
	if session.PromptMessageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: session.PromptMessageID})
	}
	return openTextEditDraftPanel(ctx, b, session)
}

type textEditorCallback struct{}

func (h *textEditorCallback) Key() string              { return "text_editor" }
func (h *textEditorCallback) Pattern() string          { return "textedit:" }
func (h *textEditorCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *textEditorCallback) Description() string      { return "通用文案编辑" }
func (h *textEditorCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	parts := strings.Split(update.CallbackQuery.Data, ":")
	if len(parts) < 2 {
		return nil
	}
	chatID := callbackChatID(update.CallbackQuery)
	userID := update.CallbackQuery.From.ID
	switch parts[1] {
	case "help":
		return startTextEditFromCallback(ctx, b, update, textEditHelpTarget())
	case "save":
		session, ok := getTextEditSession(chatID, userID)
		if !ok || strings.TrimSpace(session.Draft) == "" {
			return replyCallback(ctx, b, update, "请先输入文案内容。")
		}
		return saveTextEditDraft(ctx, b, update, session)
	case "back":
		clearTextEditSession(chatID, userID)
		return openTextEditBack(ctx, b, update, textEditHelpTarget())
	case "cancel":
		if session, ok := getTextEditSession(chatID, userID); ok && session.PromptMessageID > 0 {
			_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: session.PromptMessageID})
		}
		clearTextEditSession(chatID, userID)
		return openTextEditBack(ctx, b, update, textEditHelpTarget())
	}
	return replyCallback(ctx, b, update, "未知文案操作。")
}

func startTextEditFromCallback(ctx context.Context, b *bot.Bot, update *models.Update, target textEditTarget) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	botKey := currentBotKey(ctx)
	userID := update.CallbackQuery.From.ID
	if ok, err := service.SysLazysheepTggo().IsBotAdmin(ctx, botKey, userID); err != nil {
		return err
	} else if !ok {
		return replyCallback(ctx, b, update, "该菜单仅机器人管理员可用。")
	}
	session := textEditSession{
		BotKey:         botKey,
		ChatID:         callbackChatID(update.CallbackQuery),
		UserID:         userID,
		Target:         target,
		PanelMessageID: callbackMessageID(update.CallbackQuery),
	}
	storeTextEditSession(session)
	if err := openTextEditInputPanel(ctx, b, session); err != nil {
		return err
	}
	promptMessageID, err := sendTextEditPrompt(ctx, b, session, callbackMessageThreadID(update.CallbackQuery), "")
	if err != nil {
		return err
	}
	session.PromptMessageID = promptMessageID
	storeTextEditSession(session)
	return replyCallback(ctx, b, update, "请直接回复这条消息并发送文案。")
}

func openTextEditInputPanel(ctx context.Context, b *bot.Bot, session textEditSession) error {
	text := session.Target.Title + "\n\n请直接回复机器人刚发送的输入提示，发送新的文案内容。"
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "返回", CallbackData: "textedit:back"}},
		{{Text: "取消编辑", CallbackData: "textedit:cancel"}},
	}}
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      session.ChatID,
		MessageID:   session.PanelMessageID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

func sendTextEditPrompt(ctx context.Context, b *bot.Bot, session textEditSession, messageThreadID int, hint string) (int, error) {
	text := "编辑" + session.Target.Title + "\n\n请直接回复这条消息并发送新的文案内容。"
	if current := currentTextEditValue(ctx, session.BotKey, session.Target); strings.TrimSpace(current) != "" {
		text += "\n\n当前内容：" + displayFooterText(current)
	}
	if strings.TrimSpace(hint) != "" {
		text += "\n\n" + strings.TrimSpace(hint)
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          session.ChatID,
		MessageThreadID: messageThreadID,
		Text:            text,
		ReplyMarkup: &models.ForceReply{
			ForceReply:            true,
			Selective:             true,
			InputFieldPlaceholder: session.Target.Placeholder,
		},
	})
	if err != nil || sent == nil {
		return 0, err
	}
	return sent.ID, nil
}

func openTextEditDraftPanel(ctx context.Context, b *bot.Bot, session textEditSession) error {
	text := session.Target.Title + "已录入，请确认保存。\n\n" + displayFooterText(session.Draft)
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "保存", CallbackData: "textedit:save"}},
		{{Text: "重新输入", CallbackData: "textedit:back"}},
		{{Text: "取消", CallbackData: "textedit:cancel"}},
	}}
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      session.ChatID,
		MessageID:   session.PanelMessageID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	return err
}

func saveTextEditDraft(ctx context.Context, b *bot.Bot, update *models.Update, session textEditSession) error {
	opCtx := context.WithoutCancel(ctx)
	state, err := service.SysLazysheepTggo().GetState(opCtx)
	if err != nil {
		return err
	}
	if err = updateTextEditValue(state, session.BotKey, session.Target, session.Draft); err != nil {
		return replyCallback(ctx, b, update, fmt.Sprintf("保存失败：%v", err))
	}
	if err = service.SysLazysheepTggo().SaveState(opCtx, state); err != nil {
		return err
	}
	if session.PromptMessageID > 0 {
		_, _ = b.DeleteMessage(opCtx, &bot.DeleteMessageParams{ChatID: session.ChatID, MessageID: session.PromptMessageID})
	}
	clearTextEditSession(session.ChatID, session.UserID)
	if err = openTextEditBack(opCtx, b, update, session.Target); err != nil {
		return err
	}
	return replyCallback(opCtx, b, update, session.Target.Title+"已更新。")
}

func openTextEditBack(ctx context.Context, b *bot.Bot, update *models.Update, target textEditTarget) error {
	if target.BackData == "admin:plugin:help" {
		return editAdminPanel(ctx, b, update, buildAdminPluginDetailText(ctx, currentBotKey(ctx), "help"), buildAdminPluginDetailKeyboard(ctx, currentBotKey(ctx), "help"))
	}
	return nil
}

func textEditHelpTarget() textEditTarget {
	return textEditTarget{
		Key:         "help",
		PluginKey:   "help",
		SettingKey:  "helpText",
		Title:       "帮助文案",
		Placeholder: "发送新的帮助文案",
		BackData:    "admin:plugin:help",
	}
}

func currentTextEditValue(ctx context.Context, botKey string, target textEditTarget) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return ""
	}
	plugin := adminPluginsForBot(state, botKey)[target.PluginKey]
	if plugin == nil {
		return ""
	}
	return settingString(plugin.Settings, target.SettingKey, "")
}

func updateTextEditValue(state *model.State, botKey string, target textEditTarget, text string) error {
	if state == nil {
		return fmt.Errorf("状态不存在")
	}
	botCfg := state.Bots[botKey]
	if botCfg == nil {
		return fmt.Errorf("机器人配置不存在")
	}
	if botCfg.Plugins == nil {
		botCfg.Plugins = cloneAdminPlugins(state.Plugins)
	}
	plugin := botCfg.Plugins[target.PluginKey]
	if plugin == nil {
		if def := model.DefaultPluginConfigs()[target.PluginKey]; def != nil {
			botCfg.Plugins[target.PluginKey] = cloneAdminPlugins(map[string]*model.PluginConfig{target.PluginKey: def})[target.PluginKey]
			plugin = botCfg.Plugins[target.PluginKey]
		}
	}
	if plugin == nil {
		return fmt.Errorf("插件不存在")
	}
	if plugin.Settings == nil {
		plugin.Settings = map[string]any{}
	}
	plugin.Settings[target.SettingKey] = strings.TrimSpace(text)
	return nil
}

func storeTextEditSession(session textEditSession) {
	textEditSessions.Store(textEditSessionKey(session.ChatID, session.UserID), session)
	textEditSessions.Store(textEditSessionKey(session.ChatID, 0), session)
}

func getTextEditSession(chatID, userID int64) (textEditSession, bool) {
	raw, ok := textEditSessions.Load(textEditSessionKey(chatID, userID))
	if !ok && userID != 0 {
		raw, ok = textEditSessions.Load(textEditSessionKey(chatID, 0))
	}
	if !ok {
		return textEditSession{}, false
	}
	session, ok := raw.(textEditSession)
	return session, ok
}

func clearTextEditSession(chatID, userID int64) {
	textEditSessions.Delete(textEditSessionKey(chatID, userID))
	textEditSessions.Delete(textEditSessionKey(chatID, 0))
}

func textEditSessionKey(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
}

func textEditMessageUserID(msg *models.Message) int64 {
	if msg == nil || msg.From == nil {
		return 0
	}
	return msg.From.ID
}
