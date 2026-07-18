// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/lazysheep_tggo/model"
)

type contextKey string

const (
	botKeyContextKey           contextKey = "lazysheep_tggo_bot_key"
	callbackAnsweredContextKey contextKey = "lazysheep_tggo_callback_answered"
)

type MessageHandler interface {
	Key() string
	Pattern() string
	MatchType() bot.MatchType
	Description() string
	Handle(ctx context.Context, b *bot.Bot, update *models.Update) error
}

type MessageMatcher interface {
	Match(update *models.Update) bool
}

type CallbackHandler interface {
	Key() string
	Pattern() string
	MatchType() bot.MatchType
	Description() string
	Handle(ctx context.Context, b *bot.Bot, update *models.Update) error
}

type TriggerType string

const (
	TriggerStart      TriggerType = "start"
	TriggerMenuButton TriggerType = "menu_button"
)

type PluginRequest struct {
	Trigger TriggerType
	BotKey  string
	Text    string
	Update  *models.Update
}

func WithBotKey(ctx context.Context, botKey string) context.Context {
	return context.WithValue(ctx, botKeyContextKey, botKey)
}

func CurrentBotKey(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(botKeyContextKey).(string); ok {
		return v
	}
	return ""
}

func WithCallbackAnswered(ctx context.Context) context.Context {
	return context.WithValue(ctx, callbackAnsweredContextKey, true)
}

func CallbackAnswered(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(callbackAnsweredContextKey).(bool)
	return v
}

type BotPlugin interface {
	Key() string
	Handle(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, plugins map[string]*model.PluginConfig) (bool, error)
}

var (
	messageHandlers  = make([]MessageHandler, 0, 8)
	callbackHandlers = make([]CallbackHandler, 0, 8)
	botPlugins       = make([]BotPlugin, 0, 8)
	handlerMu        sync.RWMutex
)

func RegisterMessageHandler(h MessageHandler) {
	handlerMu.Lock()
	defer handlerMu.Unlock()
	messageHandlers = append(messageHandlers, h)
}

func RegisterCallbackHandler(h CallbackHandler) {
	handlerMu.Lock()
	defer handlerMu.Unlock()
	callbackHandlers = append(callbackHandlers, h)
}

func RegisterBotPlugin(p BotPlugin) {
	handlerMu.Lock()
	defer handlerMu.Unlock()
	botPlugins = append(botPlugins, p)
}

func MessageHandlers() []MessageHandler {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	out := make([]MessageHandler, len(messageHandlers))
	copy(out, messageHandlers)
	return out
}

func CallbackHandlers() []CallbackHandler {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	out := make([]CallbackHandler, len(callbackHandlers))
	copy(out, callbackHandlers)
	return out
}

func BotPlugins() []BotPlugin {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	out := make([]BotPlugin, len(botPlugins))
	copy(out, botPlugins)
	return out
}

func startPayload(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/start") {
		return ""
	}
	payload := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	return strings.TrimSpace(payload)
}
