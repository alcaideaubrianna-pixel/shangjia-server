package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/library/cache"
)

const monitorChatMapCacheKey = "lazysheep_tggo:monitor:chat_map"

type monitorChatLabel struct {
	Title string `json:"title"`
	Label string `json:"label"`
}

func (s *sLazySheepTGGo) monitorBotLabels(ctx context.Context) map[string]string {
	labels := make(map[string]string)
	state, err := s.GetState(ctx)
	if err != nil || state == nil {
		return labels
	}
	for key, item := range state.Bots {
		if item == nil {
			continue
		}
		label := strings.TrimSpace(item.Username)
		if label != "" {
			label = "@" + strings.TrimPrefix(label, "@")
		}
		if label == "" {
			label = strings.TrimSpace(item.DisplayName)
		}
		if label == "" {
			label = key
		}
		labels[key] = label
	}
	return labels
}

func (s *sLazySheepTGGo) monitorChatLabels(ctx context.Context) map[string]monitorChatLabel {
	val, err := cache.Instance().Get(ctx, monitorChatMapCacheKey)
	if err == nil && !val.IsNil() {
		out := make(map[string]monitorChatLabel)
		_ = val.Scan(&out)
		if len(out) > 0 {
			return out
		}
	}
	out := make(map[string]monitorChatLabel)
	if err = s.ensureChatMapTable(ctx); err != nil {
		return out
	}
	var rows []struct {
		BotKey string `json:"botKey" orm:"bot_key"`
		ChatID int64  `json:"chatId" orm:"chat_id"`
		Title  string `json:"title" orm:"title"`
		Label  string `json:"label" orm:"label"`
	}
	if err = g.DB().Model("hg_addon_lazysheep_tggo_chat_map").Fields("bot_key,chat_id,title,label").Scan(&rows); err != nil {
		return out
	}
	for _, row := range rows {
		label := strings.TrimSpace(row.Label)
		if label == "" {
			label = strings.TrimSpace(row.Title)
		}
		if label == "" {
			label = fmt.Sprintf("%d", row.ChatID)
		}
		out[monitorChatMapKey(row.BotKey, row.ChatID)] = monitorChatLabel{Title: row.Title, Label: label}
	}
	mergeMonitorChatLabelsFromWebhookLog(ctx, out)
	_ = cache.Instance().Set(ctx, monitorChatMapCacheKey, out, 10*time.Minute)
	return out
}

func mergeMonitorChatLabelsFromWebhookLog(ctx context.Context, out map[string]monitorChatLabel) {
	var rows []struct {
		BotKey  string `json:"botKey" orm:"bot_key"`
		Payload string `json:"payload" orm:"payload"`
	}
	if err := g.DB().Model("hg_addon_lazysheep_tggo_webhook_log").
		Fields("bot_key,payload").
		OrderDesc("id").
		Limit(2000).
		Scan(&rows); err != nil {
		return
	}
	for _, row := range rows {
		var update models.Update
		if err := json.Unmarshal([]byte(row.Payload), &update); err != nil {
			continue
		}
		chat, ok := telegramChatFromUpdate(&update)
		if !ok || chat.ID == 0 {
			continue
		}
		key := monitorChatMapKey(row.BotKey, chat.ID)
		if _, ok = out[key]; ok {
			continue
		}
		title := strings.TrimSpace(chat.Title)
		if title == "" {
			title = strings.TrimSpace(strings.TrimSpace(chat.FirstName + " " + chat.LastName))
		}
		label := title
		if label == "" && strings.TrimSpace(chat.Username) != "" {
			label = "@" + strings.TrimPrefix(strings.TrimSpace(chat.Username), "@")
		}
		if label == "" {
			continue
		}
		out[key] = monitorChatLabel{Title: title, Label: label}
	}
}

func monitorChatMapKey(botKey string, chatID int64) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(botKey), chatID)
}

func (s *sLazySheepTGGo) enrichPullMonitorLabels(ctx context.Context, res *sysin.PullMonitorModel) *sysin.PullMonitorModel {
	if res == nil {
		return res
	}
	bots := s.monitorBotLabels(ctx)
	chats := s.monitorChatLabels(ctx)
	state, _ := s.GetState(ctx)
	for _, item := range res.Bindings {
		if item == nil {
			continue
		}
		item.BotName = monitorBotLabel(bots, item.BotKey)
		chat := chats[monitorChatMapKey(item.BotKey, item.ChatID)]
		item.ChatTitle = chat.Title
		item.ChatLabel = monitorChatLabelText(chat, item.ChatID)
		if state != nil && state.Bindings != nil {
			if binding := state.Bindings[item.BindingKey]; binding != nil {
				item.AutoPull = pluginStateBoolValue(binding.PluginState, collectorAutoPullStateKey, true)
				item.AutoPullStoppedAt = pluginSettingString(binding.PluginState, collectorAutoPullStoppedAtKey, "")
				item.AutoPullStopReason = pluginSettingString(binding.PluginState, collectorAutoPullStopReasonKey, "")
			}
		}
	}
	for _, item := range res.Recent {
		if item == nil {
			continue
		}
		item.BotName = monitorBotLabel(bots, item.BotKey)
		chat := chats[monitorChatMapKey(item.BotKey, item.ChatID)]
		item.ChatTitle = chat.Title
		item.ChatLabel = monitorChatLabelText(chat, item.ChatID)
	}
	return res
}

func (s *sLazySheepTGGo) enrichPushQueueMonitorLabels(ctx context.Context, res *sysin.PushQueueMonitorModel) *sysin.PushQueueMonitorModel {
	if res == nil {
		return res
	}
	bots := s.monitorBotLabels(ctx)
	chats := s.monitorChatLabels(ctx)
	for _, item := range res.Channels {
		if item == nil {
			continue
		}
		item.BotName = monitorBotLabel(bots, item.BotKey)
		chat := chats[monitorChatMapKey(item.BotKey, item.ChatID)]
		item.ChatTitle = chat.Title
		item.ChatLabel = monitorChatLabelText(chat, item.ChatID)
	}
	for _, item := range res.Recent {
		if item == nil {
			continue
		}
		item.BotName = monitorBotLabel(bots, item.BotKey)
		chat := chats[monitorChatMapKey(item.BotKey, item.ChatID)]
		item.ChatTitle = chat.Title
		item.ChatLabel = monitorChatLabelText(chat, item.ChatID)
	}
	for _, item := range res.FailedLogs {
		if item == nil {
			continue
		}
		item.BotName = monitorBotLabel(bots, item.BotKey)
		chat := chats[monitorChatMapKey(item.BotKey, item.ChatID)]
		item.ChatTitle = chat.Title
		item.ChatLabel = monitorChatLabelText(chat, item.ChatID)
	}
	return res
}

func monitorBotLabel(items map[string]string, botKey string) string {
	if label := strings.TrimSpace(items[botKey]); label != "" {
		return label
	}
	return botKey
}

func monitorChatLabelText(item monitorChatLabel, chatID int64) string {
	label := strings.TrimSpace(item.Label)
	if label == "" {
		label = fmt.Sprintf("%d", chatID)
	}
	return fmt.Sprintf("%s (%d)", label, chatID)
}
