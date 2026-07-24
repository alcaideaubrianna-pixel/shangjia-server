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
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
)

type menuButton struct {
	Text      string `json:"text"`
	Action    string `json:"action"`
	Value     string `json:"value"`
	AdminOnly bool   `json:"adminOnly"`
}

func init() {
	RegisterBotPlugin(&welcomePlugin{})
	RegisterBotPlugin(&menuPlugin{})
}

func Dispatch(ctx context.Context, b *bot.Bot, req *PluginRequest) error {
	_, err := DispatchHandled(ctx, b, req)
	return err
}

func DispatchHandled(ctx context.Context, b *bot.Bot, req *PluginRequest) (bool, error) {
	if req == nil {
		return false, nil
	}
	plugins := currentBotPlugins(ctx)
	for _, p := range BotPlugins() {
		cfg := plugins[p.Key()]
		if p.Key() == "rights" && isRightsPayload(req.Text) {
			if cfg == nil {
				cfg = model.DefaultPluginConfigs()["rights"]
			}
		} else if p.Key() == "profile" && isMemberInvitePayload(req.Text) {
			if cfg == nil {
				cfg = model.DefaultPluginConfigs()["profile"]
			}
		} else if cfg == nil || !cfg.Enabled {
			continue
		}
		handled, err := p.Handle(ctx, b, req, cfg, plugins)
		if err != nil {
			return false, err
		}
		if handled {
			return true, nil
		}
	}
	if req.Trigger == TriggerMenuButton {
		return false, nil
	}
	return false, nil
}

type welcomePlugin struct{}

func (p *welcomePlugin) Key() string { return "welcome" }

func (p *welcomePlugin) Handle(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, plugins map[string]*model.PluginConfig) (bool, error) {
	if req.Trigger != TriggerStart || req.Update == nil || req.Update.Message == nil || req.Update.Message.From == nil {
		return false, nil
	}
	if startPayload(req.Text) != "" {
		return false, nil
	}
	_ = service.SysLazysheepTggo().TouchUser(ctx, &sysin.TouchUserInp{
		TelegramID:   req.Update.Message.From.ID,
		BotKey:       req.BotKey,
		Username:     req.Update.Message.From.Username,
		FirstName:    req.Update.Message.From.FirstName,
		LastName:     req.Update.Message.From.LastName,
		LanguageCode: req.Update.Message.From.LanguageCode,
		IsBot:        req.Update.Message.From.IsBot,
	})
	isAdmin, _ := service.SysLazysheepTggo().IsBotAdmin(ctx, req.BotKey, req.Update.Message.From.ID)
	role := currentBotRole(ctx)
	settings := cfg.Settings
	text := settingString(settings, "welcomeText", "欢迎使用<b>懒羊羊TGGo</b>")
	params := &bot.SendMessageParams{
		ChatID:    req.Update.Message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if menuCfg := plugins["menu"]; menuCfg != nil && menuCfg.Enabled {
		params.ReplyMarkup = buildReplyKeyboard(menuCfg.Settings, plugins, isAdmin, role)
	}
	if params.ReplyMarkup == nil {
		g.Log().Infof(ctx, "Telegram /start 未生成底部按钮 bot:%s", req.BotKey)
	}
	_, err := b.SendMessage(ctx, params)
	return true, err
}

type menuPlugin struct{}

func (p *menuPlugin) Key() string { return "menu" }

func (p *menuPlugin) Handle(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, plugins map[string]*model.PluginConfig) (bool, error) {
	if req.Trigger != TriggerMenuButton || req.Update == nil || req.Update.Message == nil {
		return false, nil
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return false, nil
	}
	userID := int64(0)
	if req.Update.Message.From != nil {
		userID = req.Update.Message.From.ID
	}
	isAdmin, _ := service.SysLazysheepTggo().IsBotAdmin(ctx, req.BotKey, userID)
	for _, row := range settingButtons(cfg.Settings, currentBotRole(ctx)) {
		for _, item := range row {
			if item.Text != text {
				continue
			}
			if strings.TrimSpace(item.Text) == "创建机器人" && item.Action == "reply" {
				return true, openBotCreateSession(ctx, b, req)
			}
			if item.AdminOnly && !isAdmin {
				return true, executeMenuButton(ctx, b, req, menuButton{Text: item.Text, Action: "reply", Value: "该菜单仅管理员可用。"})
			}
			return true, executeMenuButton(ctx, b, req, item)
		}
	}
	return false, nil
}

func executeMenuButton(ctx context.Context, b *bot.Bot, req *PluginRequest, item menuButton) error {
	reply := strings.TrimSpace(item.Value)
	switch item.Action {
	case "url":
		if reply == "" {
			reply = "链接未配置"
		}
	case "plugin":
		if reply != "" && dispatchMenuButtonShortcut(ctx, b, req, reply) {
			return nil
		}
		if reply == "" {
			reply = "插件功能未配置"
		}
	case "reply":
	default:
		if reply == "" {
			reply = "按钮未配置"
		}
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: req.Update.Message.Chat.ID,
		Text:   reply,
	})
	return err
}

func dispatchMenuButtonShortcut(ctx context.Context, b *bot.Bot, req *PluginRequest, text string) bool {
	update := cloneUpdateWithText(req.Update, text)
	if update == nil || update.Message == nil {
		return false
	}
	for _, handler := range MessageHandlers() {
		if handler.Key() == "menu_button" {
			continue
		}
		if !menuShortcutMatchesHandler(text, handler) {
			continue
		}
		if err := handler.Handle(ctx, b, update); err != nil {
			g.Log().Warningf(ctx, "Telegram 菜单快捷入口执行失败 bot:%s handler:%s err:%+v", req.BotKey, handler.Key(), err)
		}
		return true
	}
	return false
}

func cloneUpdateWithText(update *models.Update, text string) *models.Update {
	if update == nil || update.Message == nil {
		return nil
	}
	cloned := *update
	msg := *update.Message
	msg.Text = text
	msg.Entities = commandEntitiesForText(text)
	cloned.Message = &msg
	if update.EditedMessage != nil {
		edited := *update.EditedMessage
		edited.Text = text
		edited.Entities = commandEntitiesForText(text)
		cloned.EditedMessage = &edited
	}
	if update.ChannelPost != nil {
		post := *update.ChannelPost
		post.Text = text
		post.Entities = commandEntitiesForText(text)
		cloned.ChannelPost = &post
	}
	if update.EditedChannelPost != nil {
		editedPost := *update.EditedChannelPost
		editedPost.Text = text
		editedPost.Entities = commandEntitiesForText(text)
		cloned.EditedChannelPost = &editedPost
	}
	return &cloned
}

func commandEntitiesForText(text string) []models.MessageEntity {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return nil
	}
	command := text
	if idx := strings.IndexAny(command, " \t"); idx >= 0 {
		command = command[:idx]
	}
	if idx := strings.IndexByte(command, '@'); idx >= 0 {
		command = command[:idx]
	}
	if len(command) <= 1 {
		return nil
	}
	return []models.MessageEntity{{
		Type:   models.MessageEntityTypeBotCommand,
		Offset: 0,
		Length: len(command),
	}}
}

func menuShortcutMatchesHandler(text string, handler MessageHandler) bool {
	text = strings.TrimSpace(text)
	if text == "" || handler == nil {
		return false
	}
	pattern := strings.TrimSpace(handler.Pattern())
	switch handler.MatchType() {
	case bot.MatchTypeExact:
		return text == pattern || text == handler.Key()
	case bot.MatchTypePrefix:
		return strings.HasPrefix(text, pattern)
	case bot.MatchTypeContains:
		return strings.Contains(text, pattern)
	case bot.MatchTypeCommand, bot.MatchTypeCommandStartOnly:
		if !strings.HasPrefix(text, "/") {
			return false
		}
		first := strings.TrimSpace(text)
		if idx := strings.IndexAny(first, " \t"); idx >= 0 {
			first = first[:idx]
		}
		first = strings.TrimPrefix(first, "/")
		if idx := strings.IndexByte(first, '@'); idx >= 0 {
			first = first[:idx]
		}
		return first == pattern
	default:
		return false
	}
}

func buildReplyKeyboard(settings map[string]any, plugins map[string]*model.PluginConfig, isAdmin bool, role string) *models.ReplyKeyboardMarkup {
	rows := settingButtons(settings, role)
	if settingBool(settings, "showPluginCommands", true) {
		commandRow := make([]menuButton, 0)
		for _, key := range sortedAdminPluginKeys(plugins) {
			item := plugins[key]
			if key == "welcome" || key == "menu" || item == nil || !item.Enabled {
				continue
			}
			if !settingBool(item.Settings, "menuVisible", false) {
				continue
			}
			command := settingString(item.Settings, "command", "")
			if command != "" {
				commandRow = append(commandRow, menuButton{Text: command, Action: "plugin", Value: command})
			}
		}
		if len(commandRow) > 0 {
			rows = append(rows, commandRow)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	keyboard := make([][]models.KeyboardButton, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		line := make([]models.KeyboardButton, 0, len(row))
		for _, item := range row {
			if item.Text == "" {
				continue
			}
			if item.AdminOnly && !isAdmin {
				continue
			}
			line = append(line, models.KeyboardButton{Text: item.Text})
		}
		if len(line) > 0 {
			keyboard = append(keyboard, line)
		}
	}
	if len(keyboard) == 0 {
		return nil
	}
	return &models.ReplyKeyboardMarkup{
		Keyboard:       keyboard,
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

func currentBotPlugins(ctx context.Context) map[string]*model.PluginConfig {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return model.DefaultPluginConfigs()
	}
	botKey := currentBotKey(ctx)
	if botKey != "" {
		if cfg := state.Bots[botKey]; cfg != nil && cfg.Plugins != nil {
			return cfg.Plugins
		}
	}
	return state.Plugins
}

func currentBotRole(ctx context.Context) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return "user"
	}
	botKey := currentBotKey(ctx)
	if botKey == "" {
		return "user"
	}
	if cfg := state.Bots[botKey]; cfg != nil && strings.TrimSpace(cfg.Role) != "" {
		return cfg.Role
	}
	return "user"
}

func settingString(settings map[string]any, key, fallback string) string {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func settingBool(settings map[string]any, key string, fallback bool) bool {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(bool); ok {
		return v
	}
	return fallback
}

func hasMountedPlugin(settings map[string]any, key string) bool {
	if settings == nil {
		return false
	}
	raw, ok := settings["mountedPlugins"].([]any)
	if !ok {
		return false
	}
	for _, item := range raw {
		if strings.TrimSpace(fmt.Sprint(item)) == key {
			return true
		}
	}
	return false
}

func settingButtons(settings map[string]any, role string) [][]menuButton {
	if settings == nil {
		return nil
	}
	raw := roleButtons(settings, role)
	rows := make([][]menuButton, 0, len(raw))
	for _, rowRaw := range raw {
		items := toAnySlice(rowRaw)
		row := make([]menuButton, 0, len(items))
		for _, itemRaw := range items {
			itemMap, ok := itemRaw.(map[string]any)
			if !ok {
				continue
			}
			value := strings.TrimSpace(fmt.Sprint(itemMap["value"]))
			action := strings.TrimSpace(fmt.Sprint(itemMap["action"]))
			if action == "plugin" && value == "/pull 配置" {
				value = "管理员配置"
			}
			row = append(row, menuButton{
				Text:      strings.TrimSpace(fmt.Sprint(itemMap["text"])),
				Action:    action,
				Value:     value,
				AdminOnly: settingMapBool(itemMap, "adminOnly", false),
			})
		}
		rows = append(rows, row)
	}
	return rows
}

func roleButtons(settings map[string]any, role string) []any {
	var raw []any
	if strings.TrimSpace(role) == "official" {
		raw = toAnySlice(settings["officialButtons"])
	} else {
		raw = toAnySlice(settings["userButtons"])
	}
	if len(raw) == 0 {
		raw = toAnySlice(settings["buttons"])
	}
	return raw
}

func settingMapBool(settings map[string]any, key string, fallback bool) bool {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(bool); ok {
		return v
	}
	return fallback
}

func toAnySlice(raw any) []any {
	switch v := raw.(type) {
	case []any:
		return v
	case []map[string]any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out
	case [][]map[string]any:
		out := make([]any, 0, len(v))
		for _, row := range v {
			out = append(out, toAnySlice(row))
		}
		return out
	default:
		return nil
	}
}
