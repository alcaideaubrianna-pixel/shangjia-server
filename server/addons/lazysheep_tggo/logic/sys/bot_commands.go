// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/lazysheep_tggo/model"
)

func telegramAllowedUpdates() bot.AllowedUpdates {
	return bot.AllowedUpdates{
		"message",
		"edited_message",
		"channel_post",
		"edited_channel_post",
		"callback_query",
	}
}

func pluginBotCommands(plugins map[string]*model.PluginConfig) []models.BotCommand {
	if len(plugins) == 0 {
		return nil
	}
	type item struct {
		sort int
		cmd  models.BotCommand
	}
	rows := make([]item, 0, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil || !plugin.Enabled || !plugin.UserEnabled {
			continue
		}
		if plugin.Key == "collector" {
			continue
		}
		if !botSettingBool(plugin.Settings, "menuVisible", true) {
			continue
		}
		desc := plugin.Description
		if strings.TrimSpace(desc) == "" {
			desc = plugin.Name
		}
		sortVal := plugin.Sort
		for _, cmd := range pluginCommandList(plugin.Settings) {
			cmd = normalizeTelegramCommandName(cmd)
			if cmd == "" {
				continue
			}
			rows = append(rows, item{
				sort: sortVal,
				cmd: models.BotCommand{
					Command:     cmd,
					Description: desc,
				},
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].sort == rows[j].sort {
			return rows[i].cmd.Command < rows[j].cmd.Command
		}
		return rows[i].sort < rows[j].sort
	})
	out := make([]models.BotCommand, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.cmd)
	}
	return out
}

func pluginCommandList(settings map[string]any) []string {
	raw := botToAnySlice(settings["commands"])
	out := make([]string, 0, len(raw)+1)
	if single := strings.TrimSpace(fmt.Sprint(settings["command"])); single != "" {
		out = append(out, single)
	}
	for _, item := range raw {
		cmd := strings.TrimSpace(fmt.Sprint(item))
		if cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func dedupeBotCommands(commands []models.BotCommand) []models.BotCommand {
	seen := make(map[string]struct{}, len(commands))
	out := make([]models.BotCommand, 0, len(commands))
	for _, cmd := range commands {
		name := strings.TrimSpace(cmd.Command)
		if name == "" {
			continue
		}
		name = strings.TrimPrefix(name, "/")
		name = normalizeTelegramCommandName(name)
		if name == "" {
			continue
		}
		cmd.Command = name
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, cmd)
	}
	return out
}

func botCommandNames(commands []models.BotCommand) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.TrimSpace(command.Command) != "" {
			out = append(out, command.Command)
		}
	}
	return out
}

func normalizeTelegramCommandName(command string) string {
	name := strings.TrimPrefix(strings.TrimSpace(command), "/")
	switch name {
	case "绑定":
		name = "bind"
	case "绑定审核":
		name = "bind_review"
	case "绑定发布":
		name = "bind_publish"
	case "拉取":
		name = "pull"
	case "签到":
		name = "sign"
	case "积分":
		name = "points"
	case "会员":
		name = "member"
	case "个人中心":
		name = "profile"
	case "帮助":
		name = "help"
	case "审核":
		name = "review"
	}
	if !isTelegramCommandName(name) {
		return ""
	}
	return name
}

func isTelegramCommandName(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' {
			continue
		}
		return false
	}
	return true
}

func matchTelegramTextAlias(text, key string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, alias := range telegramTextAliases(key) {
		if text == alias || strings.HasPrefix(text, alias+" ") || strings.HasPrefix(text, alias+"@") {
			return true
		}
	}
	return false
}

func telegramTextAliases(key string) []string {
	switch key {
	case "bind":
		return []string{"/bind", "/绑定", "绑定"}
	case "bind_review":
		return []string{"/bind_review", "/绑定审核", "绑定审核"}
	case "bind_publish":
		return []string{"/bind_publish", "/绑定发布", "绑定发布"}
	case "pull":
		return []string{"/pull", "pull", "/拉取", "拉取"}
	case "sign":
		return []string{"/sign", "/签到", "签到"}
	case "points":
		return []string{"/points", "/积分", "积分"}
	case "profile":
		return []string{"/profile", "/个人中心", "个人中心"}
	case "help":
		return []string{"/help", "/帮助", "帮助"}
	default:
		return nil
	}
}
