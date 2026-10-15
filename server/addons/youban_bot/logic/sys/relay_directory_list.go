package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/entity"
)

const relayDirectoryPageSize = 10

func (s *sSysBot) relayDirectoryList(ctx context.Context, botID int64, kind, args string) (string, error) {
	page := 1
	if value := strings.TrimSpace(args); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return "", fmt.Errorf("页码不正确")
		}
		page = parsed
	}
	if kind == "私聊用户" {
		return relayPrivateUserList(ctx, botID, page)
	}
	return relayGroupList(ctx, botID, page)
}

func relayPrivateUserList(ctx context.Context, botID int64, page int) (string, error) {
	var rows []*entity.YoubanBotUser
	c := dao.YoubanBotUser.Columns()
	err := dao.YoubanBotUser.Ctx(ctx).Where(c.BotId, botID).Where(c.Status, 1).Where(c.ChatType, "private").Page(page, relayDirectoryPageSize).OrderDesc(c.LastMessageAt).Scan(&rows)
	if err != nil {
		return "", err
	}
	lines := []string{fmt.Sprintf("<b>私聊用户</b> · 第%d页", page)}
	for _, row := range rows {
		name := firstNonEmpty(strings.TrimSpace(row.TelegramFirstName+" "+row.TelegramLastName), row.TelegramUsername, row.TelegramUserId)
		username := ""
		if row.TelegramUsername != "" {
			username = " @" + row.TelegramUsername
		}
		lines = append(lines, fmt.Sprintf("%s%s · <code>%s</code>", html.EscapeString(name), html.EscapeString(username), row.TelegramUserId))
	}
	if len(rows) == 0 {
		lines = append(lines, "暂无私聊用户。")
	}
	return strings.Join(lines, "\n"), nil
}

func relayGroupList(ctx context.Context, botID int64, page int) (string, error) {
	var rows []*entity.YoubanBotChannelCache
	c := dao.YoubanBotChannelCache.Columns()
	err := dao.YoubanBotChannelCache.Ctx(ctx).Where(c.BotId, botID).WhereIn(c.ChatType, []string{"group", "supergroup"}).Page(page, relayDirectoryPageSize).OrderDesc(c.LastMessageAt).Scan(&rows)
	if err != nil {
		return "", err
	}
	lines := []string{fmt.Sprintf("<b>群聊列表</b> · 第%d页", page)}
	for _, row := range rows {
		name := firstNonEmpty(row.ChatTitle, row.ChatUsername, row.ChatId)
		username := ""
		if row.ChatUsername != "" {
			username = " @" + row.ChatUsername
		}
		lines = append(lines, fmt.Sprintf("%s%s · <code>%s</code>", html.EscapeString(name), html.EscapeString(username), row.ChatId))
	}
	if len(rows) == 0 {
		lines = append(lines, "暂无群聊。请将机器人加入群聊并发送一条消息。")
	}
	return strings.Join(lines, "\n"), nil
}
