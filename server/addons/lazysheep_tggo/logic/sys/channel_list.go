// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
)

type channelNoteCount struct {
	BindingID int64 `json:"bindingId" orm:"binding_id"`
	Count     int   `json:"count" orm:"count"`
}

type channelQueueCount struct {
	BindingKey string `json:"bindingKey" orm:"binding_key"`
	ChatID     int64  `json:"chatId" orm:"chat_id"`
	Pending    int    `json:"pending" orm:"pending"`
	Doing      int    `json:"doing" orm:"doing"`
	Retry      int    `json:"retry" orm:"retry"`
	Done       int    `json:"done" orm:"done"`
	Dead       int    `json:"dead" orm:"dead"`
	Unknown    int    `json:"unknown" orm:"unknown"`
	LastError  string `json:"lastError" orm:"last_error"`
}

func (s *sLazySheepTGGo) ChannelList(ctx context.Context, in *lsysin.ChannelListInp) (res *lsysin.ChannelListModel, err error) {
	res = &lsysin.ChannelListModel{List: []*lsysin.ChannelListItem{}}
	state, err := s.GetState(ctx)
	if err != nil {
		return nil, err
	}
	noteCounts, err := s.channelNoteCounts(ctx)
	if err != nil {
		return nil, err
	}
	queueCounts, err := s.channelQueueCounts(ctx)
	if err != nil {
		return nil, err
	}
	keyword := strings.ToLower(strings.TrimSpace(in.Keyword))
	status := strings.TrimSpace(in.Status)
	bots := s.monitorBotLabels(ctx)
	chats := s.monitorChatLabels(ctx)
	for _, binding := range state.Bindings {
		if binding == nil {
			continue
		}
		if in.BotKey != "" && binding.BotKey != in.BotKey {
			continue
		}
		chatID := channelListChatID(binding)
		botCfg := state.Bots[binding.BotKey]
		chat := chats[monitorChatMapKey(binding.BotKey, chatID)]
		queue := queueCounts[channelQueueKey(binding.Key, chatID)]
		item := &lsysin.ChannelListItem{
			BotKey:             binding.BotKey,
			BotName:            monitorBotLabel(bots, binding.BotKey),
			BindingKey:         binding.Key,
			SourceURL:          binding.SourceURL,
			ChatID:             chatID,
			ChatTitle:          chat.Title,
			ChatLabel:          monitorChatLabelText(chat, chatID),
			AddedBy:            channelListAddedBy(binding, botCfg),
			AddedAt:            formatGTime(binding.CreatedAt),
			UpdatedAt:          formatGTime(binding.UpdatedAt),
			LastPullID:         binding.LastPullID,
			LastCursor:         binding.LastCursor,
			AutoPull:           pluginStateBoolValue(binding.PluginState, collectorAutoPullStateKey, true),
			AutoPullStoppedAt:  pluginSettingString(binding.PluginState, collectorAutoPullStoppedAtKey, ""),
			AutoPullStopReason: pluginSettingString(binding.PluginState, collectorAutoPullStopReasonKey, ""),
			BindingStatus:      binding.Status,
			NoteCount:          noteCounts[binding.ID],
		}
		if queue != nil {
			item.Pending = queue.Pending
			item.Doing = queue.Doing
			item.Retry = queue.Retry
			item.Done = queue.Done
			item.Dead = queue.Dead
			item.Unknown = queue.Unknown
			item.LastError = queue.LastError
		}
		fillChannelWorkStatus(item)
		if status != "" && item.WorkStatusType != status {
			continue
		}
		if keyword != "" && !channelListMatchKeyword(item, keyword) {
			continue
		}
		res.List = append(res.List, item)
	}
	return res, nil
}

func (s *sLazySheepTGGo) channelNoteCounts(ctx context.Context) (map[int64]int, error) {
	var rows []*channelNoteCount
	err := g.DB().Model("hg_addon_lazysheep_tggo_note").
		Fields("binding_id,COUNT(*) count").
		Where("deleted_at IS NULL").
		Group("binding_id").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, row := range rows {
		if row != nil {
			out[row.BindingID] = row.Count
		}
	}
	return out, nil
}

func (s *sLazySheepTGGo) channelQueueCounts(ctx context.Context) (map[string]*channelQueueCount, error) {
	var rows []*channelQueueCount
	err := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Fields("binding_key,chat_id," +
			"SUM(CASE WHEN status=1 THEN 1 ELSE 0 END) pending," +
			"SUM(CASE WHEN status=2 THEN 1 ELSE 0 END) doing," +
			"SUM(CASE WHEN status=4 THEN 1 ELSE 0 END) retry," +
			"SUM(CASE WHEN status=3 THEN 1 ELSE 0 END) done," +
			"SUM(CASE WHEN status=5 THEN 1 ELSE 0 END) dead," +
			"SUM(CASE WHEN status=6 THEN 1 ELSE 0 END) unknown," +
			"MAX(last_error) last_error").
		Group("binding_key,chat_id").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*channelQueueCount, len(rows))
	for _, row := range rows {
		if row != nil {
			out[channelQueueKey(row.BindingKey, row.ChatID)] = row
		}
	}
	return out, nil
}

func channelListChatID(binding *model.BindingRecord) int64 {
	if binding == nil {
		return 0
	}
	if binding.AutoPush && binding.PublishChatID != 0 {
		return binding.PublishChatID
	}
	if binding.ReviewChatID != 0 {
		return binding.ReviewChatID
	}
	return binding.PublishChatID
}

func channelListAddedBy(binding *model.BindingRecord, botCfg *model.BotConfig) int64 {
	if binding != nil && binding.PluginState != nil {
		if value, ok := binding.PluginState[collectorBindOperatorIDKey]; ok {
			if id := g.NewVar(value).Int64(); id > 0 {
				return id
			}
		}
	}
	if botCfg == nil {
		return 0
	}
	if botCfg.MemberId > 0 {
		return botCfg.MemberId
	}
	return botCfg.CreatedBy
}

func formatGTime(t *gtime.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("Y-m-d H:i:s")
}

func channelQueueKey(bindingKey string, chatID int64) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(bindingKey), chatID)
}

func fillChannelWorkStatus(item *lsysin.ChannelListItem) {
	if item == nil {
		return
	}
	switch {
	case item.BindingStatus != "enabled":
		item.WorkStatus = "已停用"
		item.WorkStatusType = "disabled"
	case item.Doing > 0:
		item.WorkStatus = "正在同步"
		item.WorkStatusType = "running"
	case item.Pending+item.Retry > 0:
		item.WorkStatus = "等待推送"
		item.WorkStatusType = "pending"
	case item.Unknown > 0:
		item.WorkStatus = "待确认"
		item.WorkStatusType = "unknown"
	case item.Dead > 0:
		item.WorkStatus = "存在失败"
		item.WorkStatusType = "failed"
	case !item.AutoPull:
		item.WorkStatus = "自动拉取关闭"
		item.WorkStatusType = "paused"
	default:
		item.WorkStatus = "正常"
		item.WorkStatusType = "normal"
	}
}

func channelListMatchKeyword(item *lsysin.ChannelListItem, keyword string) bool {
	targets := []string{
		item.BotKey,
		item.BotName,
		item.BindingKey,
		item.SourceURL,
		item.ChatTitle,
		item.ChatUsername,
		item.ChatLabel,
		fmt.Sprintf("%d", item.ChatID),
		fmt.Sprintf("%d", item.AddedBy),
	}
	for _, target := range targets {
		if strings.Contains(strings.ToLower(target), keyword) {
			return true
		}
	}
	return false
}
