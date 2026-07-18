// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/dao"
)

func init() {
	RegisterMessageHandler(&adminConfigCommand{})
	RegisterMessageHandler(&adminUserSearchInputHandler{})
	RegisterMessageHandler(&adminSetSignChannelCommand{})
	RegisterMessageHandler(&adminAddSignChannelCommand{})
	RegisterCallbackHandler(&adminConfigCallback{})
}

const adminUserListPageSize = 8

var adminUserListSessions sync.Map

type adminUserListSession struct {
	BotKey          string
	ChatID          int64
	UserID          int64
	Keyword         string
	Page            int
	AwaitingSearch  bool
	PanelMessageID  int
	PromptMessageID int
}

type adminConfigCommand struct{}

func (h *adminConfigCommand) Key() string              { return "admin_config" }
func (h *adminConfigCommand) Pattern() string          { return "管理员配置" }
func (h *adminConfigCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *adminConfigCommand) Description() string      { return "管理员配置入口" }
func (h *adminConfigCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	botKey := currentBotKey(ctx)
	userID := userIDFromMessage(msg)
	if ok, err := service.SysLazysheepTggo().IsBotAdmin(ctx, botKey, userID); err != nil {
		return err
	} else if !ok {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "该菜单仅机器人管理员可用。"})
		return err
	}
	return sendAdminConfigPanel(ctx, b, botKey, msg.Chat.ID, userID)
}

type adminUserSearchInputHandler struct{}

func (h *adminUserSearchInputHandler) Key() string              { return "admin_user_search_input" }
func (h *adminUserSearchInputHandler) Pattern() string          { return "" }
func (h *adminUserSearchInputHandler) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *adminUserSearchInputHandler) Description() string      { return "管理员用户搜索输入" }
func (h *adminUserSearchInputHandler) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil || strings.TrimSpace(msg.Text) == "" {
		return false
	}
	session, ok := getAdminUserListSession(msg.Chat.ID, userIDFromMessage(msg))
	return ok && session.AwaitingSearch
}
func (h *adminUserSearchInputHandler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	session, ok := getAdminUserListSession(msg.Chat.ID, userIDFromMessage(msg))
	if !ok {
		return nil
	}
	if session.BotKey != currentBotKey(ctx) {
		clearAdminUserListSession(msg.Chat.ID, session.UserID)
		return nil
	}
	keyword := strings.TrimSpace(msg.Text)
	if keyword == "取消" || strings.EqualFold(keyword, "cancel") {
		keyword = ""
	}
	session.Keyword = keyword
	session.Page = 1
	session.AwaitingSearch = false
	storeAdminUserListSession(session)
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
	if session.PromptMessageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: session.PromptMessageID})
	}
	if session.PanelMessageID > 0 {
		return editAdminUserListPanel(ctx, b, session, session.PanelMessageID)
	}
	return sendAdminUserListPanel(ctx, b, session)
}

type adminSetSignChannelCommand struct{}

func (h *adminSetSignChannelCommand) Key() string              { return "admin_set_sign_channel" }
func (h *adminSetSignChannelCommand) Pattern() string          { return "设为签到频道" }
func (h *adminSetSignChannelCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *adminSetSignChannelCommand) Description() string {
	return "设置当前聊天为签到关注频道"
}
func (h *adminSetSignChannelCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	botKey := currentBotKey(ctx)
	if ok, err := adminCanConfigureFromMessage(ctx, botKey, msg); err != nil {
		return err
	} else if !ok {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "该命令仅机器人管理员可用。"})
		return err
	}
	title := strings.TrimSpace(msg.Chat.Title)
	if title == "" {
		title = strings.TrimSpace(msg.Chat.Username)
	}
	if title == "" {
		title = fmt.Sprintf("%d", msg.Chat.ID)
	}
	ref := adminChatRef(msg.Chat)
	url := ""
	if strings.TrimSpace(msg.Chat.Username) != "" {
		url = "https://t.me/" + strings.TrimPrefix(strings.TrimSpace(msg.Chat.Username), "@")
	}
	if err := upsertAdminSignChannel(ctx, botKey, signChannelConfig{ChatRef: ref, Title: title, URL: url}); err != nil {
		return err
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "已设置当前聊天为签到必须关注频道。"})
	return err
}

type adminAddSignChannelCommand struct{}

func (h *adminAddSignChannelCommand) Key() string              { return "admin_add_sign_channel" }
func (h *adminAddSignChannelCommand) Pattern() string          { return "签到频道" }
func (h *adminAddSignChannelCommand) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *adminAddSignChannelCommand) Description() string      { return "添加签到关注频道" }
func (h *adminAddSignChannelCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	botKey := currentBotKey(ctx)
	if ok, err := adminCanConfigureFromMessage(ctx, botKey, msg); err != nil {
		return err
	} else if !ok {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "该命令仅机器人管理员可用。"})
		return err
	}
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(msg.Text, "签到频道")))
	if len(args) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: "用法：签到频道 @channel 频道名称\n也可以在目标频道发送：设为签到频道"})
		return err
	}
	ref := args[0]
	title := ref
	if len(args) > 1 {
		title = strings.Join(args[1:], " ")
	}
	url := ""
	if strings.HasPrefix(ref, "https://t.me/") {
		url = ref
		ref = "@" + strings.TrimPrefix(ref, "https://t.me/")
	}
	if strings.HasPrefix(ref, "@") && url == "" {
		url = "https://t.me/" + strings.TrimPrefix(ref, "@")
	}
	if err := upsertAdminSignChannel(ctx, botKey, signChannelConfig{ChatRef: ref, Title: title, URL: url}); err != nil {
		return err
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: fmt.Sprintf("已添加签到频道：%s", title)})
	return err
}

type adminConfigCallback struct{}

func (h *adminConfigCallback) Key() string              { return "admin_config_callback" }
func (h *adminConfigCallback) Pattern() string          { return "admin:" }
func (h *adminConfigCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *adminConfigCallback) Description() string      { return "管理员配置动作" }
func (h *adminConfigCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	botKey := currentBotKey(ctx)
	if ok, err := service.SysLazysheepTggo().IsBotAdmin(ctx, botKey, update.CallbackQuery.From.ID); err != nil {
		return err
	} else if !ok {
		return replyCallback(ctx, b, update, "该菜单仅机器人管理员可用。")
	}
	data := strings.TrimSpace(update.CallbackQuery.Data)
	switch {
	case data == "admin:panel":
		if err := editAdminPanel(ctx, b, update, buildAdminPanelText(ctx, botKey, update.CallbackQuery.From.ID), buildAdminPanelKeyboard()); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已返回管理员配置。")
	case data == "admin:plugins":
		if err := editAdminPanel(ctx, b, update, buildAdminPluginListText(ctx, botKey), buildAdminPluginListKeyboard(ctx, botKey)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已打开插件列表。")
	case data == "admin:users":
		session := adminUserListSession{BotKey: botKey, ChatID: callbackChatID(update.CallbackQuery), UserID: update.CallbackQuery.From.ID, Page: 1, PanelMessageID: callbackMessageID(update.CallbackQuery)}
		storeAdminUserListSession(session)
		if err := editAdminUserListPanel(ctx, b, session, session.PanelMessageID); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已打开用户列表。")
	case strings.HasPrefix(data, "admin:users:page:"):
		page, err := strconv.Atoi(strings.TrimPrefix(data, "admin:users:page:"))
		if err != nil || page < 1 {
			page = 1
		}
		session := adminUserListSession{BotKey: botKey, ChatID: callbackChatID(update.CallbackQuery), UserID: update.CallbackQuery.From.ID, Page: page, PanelMessageID: callbackMessageID(update.CallbackQuery)}
		if old, ok := getAdminUserListSession(session.ChatID, session.UserID); ok {
			session.Keyword = old.Keyword
		}
		session.AwaitingSearch = false
		storeAdminUserListSession(session)
		if err := editAdminUserListPanel(ctx, b, session, session.PanelMessageID); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已更新用户列表。")
	case data == "admin:users:search":
		session := adminUserListSession{BotKey: botKey, ChatID: callbackChatID(update.CallbackQuery), UserID: update.CallbackQuery.From.ID, Page: 1, PanelMessageID: callbackMessageID(update.CallbackQuery)}
		if old, ok := getAdminUserListSession(session.ChatID, session.UserID); ok {
			session.Keyword = old.Keyword
		}
		session.AwaitingSearch = true
		promptID, err := sendAdminUserSearchPrompt(ctx, b, update, session)
		if err != nil {
			return err
		}
		session.PromptMessageID = promptID
		storeAdminUserListSession(session)
		return replyCallback(ctx, b, update, "请输入搜索关键词。")
	case strings.HasPrefix(data, "admin:user:points:"):
		parts := strings.Split(data, ":")
		if len(parts) != 5 {
			return replyCallback(ctx, b, update, "积分参数错误。")
		}
		targetID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || targetID == 0 {
			return replyCallback(ctx, b, update, "用户参数错误。")
		}
		points, err := strconv.ParseFloat(parts[4], 64)
		if err != nil || points == 0 {
			return replyCallback(ctx, b, update, "积分数错误。")
		}
		if err := adminAddUserPoints(ctx, botKey, targetID, points); err != nil {
			return err
		}
		_ = notifyAdminTargetUser(ctx, b, targetID, fmt.Sprintf("管理员已为你增加 %s 积分。", formatAdminPoints(points)))
		if err := editAdminPanel(ctx, b, update, buildAdminUserDetailText(ctx, botKey, targetID), buildAdminUserDetailKeyboard(targetID)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "积分已更新。")
	case strings.HasPrefix(data, "admin:user:member:"):
		parts := strings.Split(data, ":")
		if len(parts) != 5 {
			return replyCallback(ctx, b, update, "会员参数错误。")
		}
		targetID, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil || targetID == 0 {
			return replyCallback(ctx, b, update, "用户参数错误。")
		}
		level, err := strconv.Atoi(parts[4])
		if err != nil || level < 0 {
			return replyCallback(ctx, b, update, "会员等级错误。")
		}
		if err := adminSetUserMemberLevel(ctx, botKey, targetID, level); err != nil {
			return err
		}
		_ = notifyAdminTargetUser(ctx, b, targetID, fmt.Sprintf("管理员已将你的会员等级调整为 %d。", level))
		if err := editAdminPanel(ctx, b, update, buildAdminUserDetailText(ctx, botKey, targetID), buildAdminUserDetailKeyboard(targetID)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "会员等级已更新。")
	case strings.HasPrefix(data, "admin:user:"):
		targetID, err := strconv.ParseInt(strings.TrimPrefix(data, "admin:user:"), 10, 64)
		if err != nil || targetID == 0 {
			return replyCallback(ctx, b, update, "用户参数错误。")
		}
		if err := editAdminPanel(ctx, b, update, buildAdminUserDetailText(ctx, botKey, targetID), buildAdminUserDetailKeyboard(targetID)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已打开用户详情。")
	case data == "admin:member_invite":
		text, keyboard, err := buildAdminMemberInvitePanel(ctx, botKey)
		if err != nil {
			return err
		}
		if err := editAdminPanel(ctx, b, update, text, keyboard); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已生成会员邀请链接。")
	case strings.HasPrefix(data, "admin:plugin:"):
		key := strings.TrimPrefix(data, "admin:plugin:")
		if err := editAdminPanel(ctx, b, update, buildAdminPluginDetailText(ctx, botKey, key), buildAdminPluginDetailKeyboard(ctx, botKey, key)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "已打开插件配置。")
	case strings.HasPrefix(data, "admin:rights:select:"):
		kind := strings.TrimPrefix(data, "admin:rights:select:")
		if err := editAdminPanel(ctx, b, update, buildAdminRightsSelectText(ctx, botKey, kind), buildAdminRightsSelectKeyboard(kind)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "请选择查看策略。")
	case strings.HasPrefix(data, "admin:toggle:"):
		key := strings.TrimPrefix(data, "admin:toggle:")
		if err := toggleAdminPlugin(ctx, botKey, key); err != nil {
			return err
		}
		if update.CallbackQuery.Message.Message != nil && strings.Contains(update.CallbackQuery.Message.Message.Text, "插件配置") {
			if err := editAdminPanel(ctx, b, update, buildAdminPluginDetailText(ctx, botKey, key), buildAdminPluginDetailKeyboard(ctx, botKey, key)); err != nil {
				return err
			}
			return replyCallback(ctx, b, update, "插件状态已更新。")
		}
		if err := editAdminPanel(ctx, b, update, buildAdminPluginListText(ctx, botKey), buildAdminPluginListKeyboard(ctx, botKey)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "插件状态已更新。")
	case strings.HasPrefix(data, "admin:rights:mode:"):
		parts := strings.Split(data, ":")
		if len(parts) != 5 {
			return replyCallback(ctx, b, update, "权益策略参数错误。")
		}
		if err := setAdminRightsMode(ctx, botKey, parts[3], parts[4]); err != nil {
			return err
		}
		if parts[4] == "points" && (parts[3] == "verify" || parts[3] == "location") {
			if err := editAdminPanel(ctx, b, update, buildAdminRightsCostText(ctx, botKey, parts[3]), buildAdminRightsCostKeyboard(parts[3])); err != nil {
				return err
			}
			return replyCallback(ctx, b, update, "请选择扣除积分。")
		}
		if err := editAdminPanel(ctx, b, update, buildAdminRightsSelectText(ctx, botKey, parts[3]), buildAdminRightsSelectKeyboard(parts[3])); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "权益策略已更新。")
	case strings.HasPrefix(data, "admin:rights:cost:"):
		parts := strings.Split(data, ":")
		if len(parts) != 5 {
			return replyCallback(ctx, b, update, "积分参数错误。")
		}
		kind := parts[3]
		value := parts[4]
		cost, err := strconv.ParseFloat(value, 64)
		if err != nil || cost < 0 {
			return replyCallback(ctx, b, update, "积分数错误。")
		}
		if err := setAdminRightsCost(ctx, botKey, kind, cost); err != nil {
			return err
		}
		if err := editAdminPanel(ctx, b, update, buildAdminRightsSelectText(ctx, botKey, kind), buildAdminRightsSelectKeyboard(kind)); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "积分消耗已更新。")
	case strings.HasPrefix(data, "admin:signin:follow:"):
		value := strings.TrimPrefix(data, "admin:signin:follow:")
		if err := setAdminSignFollowRequired(ctx, botKey, value == "on"); err != nil {
			return err
		}
		if err := editAdminPanel(ctx, b, update, buildAdminPluginDetailText(ctx, botKey, "signin"), buildAdminPluginDetailKeyboard(ctx, botKey, "signin")); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "签到关注校验已更新。")
	case data == "admin:signin:clear":
		if err := clearAdminSignChannels(ctx, botKey); err != nil {
			return err
		}
		if err := editAdminPanel(ctx, b, update, buildAdminPluginDetailText(ctx, botKey, "signin"), buildAdminPluginDetailKeyboard(ctx, botKey, "signin")); err != nil {
			return err
		}
		return replyCallback(ctx, b, update, "签到频道已清空。")
	case data == "admin:feedback":
		return replyCallback(ctx, b, update, "请联系技术支持反馈问题。")
	case data == "admin:cancel":
		return deleteAdminPanel(ctx, b, update)
	default:
		return replyCallback(ctx, b, update, "未知管理员配置操作。")
	}
}

func sendAdminConfigPanel(ctx context.Context, b *bot.Bot, botKey string, chatID int64, userID int64) error {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        buildAdminPanelText(ctx, botKey, userID),
		ReplyMarkup: buildAdminPanelKeyboard(),
	})
	return err
}

func editAdminPanel(ctx context.Context, b *bot.Bot, update *models.Update, text string, keyboard *models.InlineKeyboardMarkup) error {
	if update == nil || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return nil
	}
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return err
	}
	return nil
}

func deleteAdminPanel(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return nil
	}
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
		MessageID: update.CallbackQuery.Message.Message.ID,
	})
	return replyCallback(ctx, b, update, "已取消。")
}

func buildAdminPanelText(ctx context.Context, botKey string, userID int64) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return "管理员配置\n\n无法读取机器人配置。"
	}
	name := botKey
	if cfg := state.Bots[botKey]; cfg != nil && strings.TrimSpace(cfg.DisplayName) != "" {
		name = cfg.DisplayName
	}
	adminText := adminUserSummaryText(state, botKey, userID)
	return fmt.Sprintf("管理员配置\n\n%s\n\n机器人：%s\n\n这里用于管理插件开关。", adminText, name)
}

func buildAdminPanelKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "插件列表", CallbackData: "admin:plugins"}},
		{{Text: "用户列表", CallbackData: "admin:users"}},
		{{Text: "会员邀请", CallbackData: "admin:member_invite"}},
		{{Text: "反馈技术", CallbackData: "admin:feedback"}},
		{{Text: "取消", CallbackData: "admin:cancel"}},
	}}
}

type adminUserListRow struct {
	TelegramId   int64       `json:"telegramId"`
	Username     string      `json:"username"`
	FirstName    string      `json:"firstName"`
	LastName     string      `json:"lastName"`
	MemberLevel  int         `json:"memberLevel"`
	Points       float64     `json:"points"`
	LastActiveAt *gtime.Time `json:"lastActiveAt"`
}

func sendAdminUserListPanel(ctx context.Context, b *bot.Bot, session adminUserListSession) error {
	text, keyboard, err := buildAdminUserListPanel(ctx, session)
	if err != nil {
		return err
	}
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      session.ChatID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

func editAdminUserListPanel(ctx context.Context, b *bot.Bot, session adminUserListSession, messageID int) error {
	text, keyboard, err := buildAdminUserListPanel(ctx, session)
	if err != nil {
		return err
	}
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      session.ChatID,
		MessageID:   messageID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

func buildAdminUserListPanel(ctx context.Context, session adminUserListSession) (string, *models.InlineKeyboardMarkup, error) {
	if session.Page < 1 {
		session.Page = 1
	}
	rows, total, err := loadAdminUserRows(ctx, session.BotKey, session.Keyword, session.Page, adminUserListPageSize)
	if err != nil {
		return "", nil, err
	}
	totalPages := (total + adminUserListPageSize - 1) / adminUserListPageSize
	if totalPages < 1 {
		totalPages = 1
	}
	lines := []string{
		"用户列表",
		"",
		fmt.Sprintf("搜索：%s", adminDisplayKeyword(session.Keyword)),
		fmt.Sprintf("页码：%d/%d，共 %d 人", session.Page, totalPages, total),
		"",
	}
	if len(rows) == 0 {
		lines = append(lines, "暂无用户。")
	} else {
		for idx, row := range rows {
			lines = append(lines, fmt.Sprintf("%d. %s", (session.Page-1)*adminUserListPageSize+idx+1, adminUserName(row)))
			lines = append(lines, fmt.Sprintf("ID：%d｜积分：%s｜会员等级：%d", row.TelegramId, formatAdminPoints(row.Points), row.MemberLevel))
			if row.LastActiveAt != nil {
				lines = append(lines, "活跃："+row.LastActiveAt.Format("Y-m-d H:i"))
			}
			lines = append(lines, "")
		}
	}
	keyboardRows := make([][]models.InlineKeyboardButton, 0, 6)
	for idx := 0; idx < len(rows); idx += 4 {
		buttonRow := make([]models.InlineKeyboardButton, 0, 4)
		for j := idx; j < len(rows) && j < idx+4; j++ {
			buttonRow = append(buttonRow, models.InlineKeyboardButton{
				Text:         fmt.Sprintf("%d", (session.Page-1)*adminUserListPageSize+j+1),
				CallbackData: fmt.Sprintf("admin:user:%d", rows[j].TelegramId),
			})
		}
		keyboardRows = append(keyboardRows, buttonRow)
	}
	keyboardRows = append(keyboardRows,
		[]models.InlineKeyboardButton{{Text: "搜索", CallbackData: "admin:users:search"}},
		[]models.InlineKeyboardButton{
			{Text: "上一页", CallbackData: fmt.Sprintf("admin:users:page:%d", maxInt(session.Page-1, 1))},
			{Text: "下一页", CallbackData: fmt.Sprintf("admin:users:page:%d", minInt(session.Page+1, totalPages))},
		},
		[]models.InlineKeyboardButton{{Text: "返回首页", CallbackData: "admin:panel"}, {Text: "取消", CallbackData: "admin:cancel"}},
	)
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: keyboardRows}
	return strings.TrimSpace(strings.Join(lines, "\n")), keyboard, nil
}

func buildAdminUserDetailText(ctx context.Context, botKey string, telegramID int64) string {
	row, err := loadAdminUserRow(ctx, botKey, telegramID)
	if err != nil {
		return "用户详情\n\n读取用户失败：" + err.Error()
	}
	if row.TelegramId == 0 {
		return "用户详情\n\n用户不存在。"
	}
	lines := []string{
		"用户详情",
		"",
		"用户：" + adminUserName(row),
		fmt.Sprintf("ID：%d", row.TelegramId),
		fmt.Sprintf("积分：%s", formatAdminPoints(row.Points)),
		fmt.Sprintf("会员等级：%d", row.MemberLevel),
	}
	if row.LastActiveAt != nil {
		lines = append(lines, "活跃："+row.LastActiveAt.Format("Y-m-d H:i"))
	}
	lines = append(lines, "", "请选择要执行的操作。")
	return strings.Join(lines, "\n")
}

func buildAdminUserDetailKeyboard(telegramID int64) *models.InlineKeyboardMarkup {
	id := strconv.FormatInt(telegramID, 10)
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: "+1 积分", CallbackData: "admin:user:points:" + id + ":1"},
			{Text: "+5 积分", CallbackData: "admin:user:points:" + id + ":5"},
			{Text: "+10 积分", CallbackData: "admin:user:points:" + id + ":10"},
		},
		{
			{Text: "+20 积分", CallbackData: "admin:user:points:" + id + ":20"},
			{Text: "+50 积分", CallbackData: "admin:user:points:" + id + ":50"},
			{Text: "+100 积分", CallbackData: "admin:user:points:" + id + ":100"},
		},
		{
			{Text: "普通用户", CallbackData: "admin:user:member:" + id + ":0"},
			{Text: "会员", CallbackData: "admin:user:member:" + id + ":1"},
			{Text: "高级会员", CallbackData: "admin:user:member:" + id + ":2"},
		},
		{{Text: "管理员", CallbackData: "admin:user:member:" + id + ":9"}},
		{{Text: "返回用户列表", CallbackData: "admin:users"}, {Text: "取消", CallbackData: "admin:cancel"}},
	}}
}

func loadAdminUserRow(ctx context.Context, botKey string, telegramID int64) (adminUserListRow, error) {
	cols := dao.AddonLazysheepTggoUser.Columns()
	var row adminUserListRow
	err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.TelegramId, cols.Username, cols.FirstName, cols.LastName, cols.MemberLevel, cols.Points, cols.LastActiveAt).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, telegramID).
		Scan(&row)
	return row, err
}

func adminAddUserPoints(ctx context.Context, botKey string, telegramID int64, points float64) error {
	cols := dao.AddonLazysheepTggoUser.Columns()
	row, err := loadAdminUserRow(ctx, botKey, telegramID)
	if err != nil {
		return err
	}
	if row.TelegramId == 0 {
		return fmt.Errorf("用户不存在")
	}
	now := gtime.Now()
	after := row.Points + points
	if _, err = dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, telegramID).
		Data(g.Map{cols.Points: after, cols.UpdatedAt: now}).
		Update(); err != nil {
		return err
	}
	_, err = g.DB().Model("hg_addon_lazysheep_tggo_points_log").Data(g.Map{
		"bot_key":     botKey,
		"telegram_id": telegramID,
		"change_num":  points,
		"before_num":  row.Points,
		"after_num":   after,
		"action":      "admin_add",
		"remark":      "管理员加积分",
		"status":      1,
		"created_at":  now,
		"updated_at":  now,
	}).Insert()
	return err
}

func adminSetUserMemberLevel(ctx context.Context, botKey string, telegramID int64, level int) error {
	cols := dao.AddonLazysheepTggoUser.Columns()
	result, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, telegramID).
		Data(g.Map{cols.MemberLevel: level, cols.Status: 1, cols.UpdatedAt: gtime.Now()}).
		Update()
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

func notifyAdminTargetUser(ctx context.Context, b *bot.Bot, telegramID int64, text string) error {
	if telegramID == 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: telegramID, Text: text})
	return err
}

func loadAdminUserRows(ctx context.Context, botKey, keyword string, page, pageSize int) ([]adminUserListRow, int, error) {
	cols := dao.AddonLazysheepTggoUser.Columns()
	mod := dao.AddonLazysheepTggoUser.Ctx(ctx).Where(cols.BotKey, botKey)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("("+cols.Username+" LIKE ? OR "+cols.FirstName+" LIKE ? OR "+cols.LastName+" LIKE ? OR CAST("+cols.TelegramId+" AS CHAR) LIKE ?)", like, like, like, like)
	}
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, err
	}
	var rows []adminUserListRow
	err = mod.Fields(cols.TelegramId, cols.Username, cols.FirstName, cols.LastName, cols.MemberLevel, cols.Points, cols.LastActiveAt).
		OrderDesc(cols.LastActiveAt).
		OrderDesc(cols.Id).
		Limit((page-1)*pageSize, pageSize).
		Scan(&rows)
	return rows, total, err
}

func sendAdminUserSearchPrompt(ctx context.Context, b *bot.Bot, update *models.Update, session adminUserListSession) (int, error) {
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          session.ChatID,
		MessageThreadID: callbackMessageThreadID(update.CallbackQuery),
		Text:            "请回复这条消息输入用户名、昵称或 Telegram ID。\n发送“取消”可清空搜索。",
		ReplyMarkup: &models.ForceReply{
			ForceReply:            true,
			Selective:             true,
			InputFieldPlaceholder: "输入搜索关键词",
		},
	})
	if err != nil || sent == nil {
		return 0, err
	}
	return sent.ID, nil
}

func buildAdminMemberInvitePanel(ctx context.Context, botKey string) (string, *models.InlineKeyboardMarkup, error) {
	token, err := ensureAdminMemberInviteToken(ctx, botKey)
	if err != nil {
		return "", nil, err
	}
	username := adminBotUsername(ctx, botKey)
	if username == "" {
		return "", nil, fmt.Errorf("机器人用户名为空，无法生成邀请链接")
	}
	link := fmt.Sprintf("https://t.me/%s?start=%s%s", username, memberInvitePayloadPrefix, token)
	text := "会员邀请\n\n用户打开下面链接后，会自动注册为会员。\n\n" + link
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "返回首页", CallbackData: "admin:panel"}, {Text: "取消", CallbackData: "admin:cancel"}},
	}}
	return text, keyboard, nil
}

func ensureAdminMemberInviteToken(ctx context.Context, botKey string) (string, error) {
	token := ""
	err := updateAdminPlugin(ctx, botKey, "member", func(plugin *model.PluginConfig) {
		token = settingString(plugin.Settings, "inviteToken", "")
		if token == "" {
			token = randomAdminInviteToken()
			plugin.Settings["inviteToken"] = token
		}
	})
	return token, err
}

func randomAdminInviteToken() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return strings.ToUpper(strconv.FormatInt(time.Now().UnixNano(), 36))
	}
	return hex.EncodeToString(buf)
}

func adminBotUsername(ctx context.Context, botKey string) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return ""
	}
	if cfg := state.Bots[botKey]; cfg != nil {
		return strings.TrimPrefix(strings.TrimSpace(cfg.Username), "@")
	}
	return ""
}

func adminDisplayKeyword(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "无"
	}
	return keyword
}

func adminUserName(row adminUserListRow) string {
	parts := []string{}
	if row.Username != "" {
		parts = append(parts, "@"+strings.TrimPrefix(row.Username, "@"))
	}
	name := strings.TrimSpace(row.FirstName + " " + row.LastName)
	if name != "" {
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d", row.TelegramId)
	}
	return strings.Join(parts, " / ")
}

func storeAdminUserListSession(session adminUserListSession) {
	adminUserListSessions.Store(adminUserListSessionKey(session.ChatID, session.UserID), session)
}

func getAdminUserListSession(chatID, userID int64) (adminUserListSession, bool) {
	raw, ok := adminUserListSessions.Load(adminUserListSessionKey(chatID, userID))
	if !ok {
		return adminUserListSession{}, false
	}
	session, ok := raw.(adminUserListSession)
	return session, ok
}

func clearAdminUserListSession(chatID, userID int64) {
	adminUserListSessions.Delete(adminUserListSessionKey(chatID, userID))
}

func adminUserListSessionKey(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func buildAdminPluginListText(ctx context.Context, botKey string) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return "插件列表\n\n无法读取插件配置。"
	}
	enabled, total := adminPluginStats(adminPluginsForBot(state, botKey))
	return fmt.Sprintf("插件列表\n\n已启用插件：%d/%d\n点击插件进入配置页。", enabled, total)
}

func buildAdminPluginListKeyboard(ctx context.Context, botKey string) *models.InlineKeyboardMarkup {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return nil
	}
	plugins := adminPluginsForBot(state, botKey)
	keys := sortedAdminPluginKeys(plugins)
	rows := make([][]models.InlineKeyboardButton, 0, len(keys)+1)
	for _, key := range keys {
		item := plugins[key]
		if item == nil {
			continue
		}
		status := "关"
		if item.Enabled {
			status = "开"
		}
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s：%s", item.Name, status),
			CallbackData: "admin:plugin:" + key,
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回", CallbackData: "admin:panel"}, {Text: "取消", CallbackData: "admin:cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func buildAdminPluginDetailText(ctx context.Context, botKey, key string) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return "插件配置\n\n无法读取插件配置。"
	}
	plugin := adminPluginsForBot(state, botKey)[key]
	if plugin == nil {
		return "插件配置\n\n插件不存在。"
	}
	status := "关闭"
	if plugin.Enabled {
		status = "开启"
	}
	extra := ""
	switch key {
	case "rights":
		extra = "\n\n" + adminRightsSummary(plugin)
	case "signin":
		extra = "\n\n" + adminSigninSummary(plugin)
	case "help":
		extra = "\n\n" + adminHelpSummary(plugin)
	}
	return fmt.Sprintf("插件配置\n\n名称：%s\n状态：%s\n分类：%s\n描述：%s%s\n\n点击下方按钮可直接修改。", plugin.Name, status, plugin.Category, plugin.Description, extra)
}

func buildAdminPluginDetailKeyboard(ctx context.Context, botKey, key string) *models.InlineKeyboardMarkup {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return nil
	}
	plugin := adminPluginsForBot(state, botKey)[key]
	if plugin == nil {
		return nil
	}
	label := "启用"
	if plugin.Enabled {
		label = "关闭"
	}
	rows := [][]models.InlineKeyboardButton{
		{{Text: label, CallbackData: "admin:toggle:" + key}},
	}
	switch key {
	case "rights":
		rows = append(rows, adminRightsKeyboardRows(plugin)...)
	case "signin":
		rows = append(rows, adminSigninKeyboardRows(plugin)...)
	case "help":
		rows = append(rows, adminHelpKeyboardRows()...)
	}
	rows = append(rows,
		[]models.InlineKeyboardButton{{Text: "返回列表", CallbackData: "admin:plugins"}, {Text: "返回首页", CallbackData: "admin:panel"}},
		[]models.InlineKeyboardButton{{Text: "取消", CallbackData: "admin:cancel"}},
	)
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func toggleAdminPlugin(ctx context.Context, botKey, key string) error {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	botCfg := state.Bots[botKey]
	if botCfg == nil {
		return fmt.Errorf("机器人配置不存在")
	}
	if botCfg.Plugins == nil {
		botCfg.Plugins = cloneAdminPlugins(state.Plugins)
	}
	item := botCfg.Plugins[key]
	if item == nil {
		return fmt.Errorf("插件不存在")
	}
	item.Enabled = !item.Enabled
	return service.SysLazysheepTggo().SaveState(ctx, state)
}

func adminUserSummaryText(state *model.State, botKey string, userID int64) string {
	if state == nil {
		return "管理员：未找到\n积分：0\n用户等级：0\n到期时间：未配置"
	}
	if item := state.Users[userID]; item != nil && item.BotKey == botKey {
		name := strings.TrimSpace(item.Username)
		if name == "" {
			name = strings.TrimSpace(item.FirstName + " " + item.LastName)
		}
		if name == "" {
			name = fmt.Sprintf("%d", item.TelegramID)
		}
		return fmt.Sprintf("管理员：%s (%d)\n积分：%s\n用户等级：%d\n到期时间：未配置\n插件购买：后台可查", name, item.TelegramID, formatAdminPoints(item.Points), item.MemberLevel)
	}
	return fmt.Sprintf("管理员：未同步 (%d)\n积分：0\n用户等级：0\n到期时间：未配置\n插件购买：后台可查", userID)
}

func formatAdminPoints(points float64) string {
	if points == float64(int64(points)) {
		return fmt.Sprintf("%d", int64(points))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", points), "0"), ".")
}

func adminRightsSummary(plugin *model.PluginConfig) string {
	settings := adminPluginSettings(plugin)
	return fmt.Sprintf(
		"当前权益\n查看验证：%s，扣 %s 积分\n查看位置：%s，扣 %s 积分",
		adminRightsModeText(settingString(settings, "verifyMode", "none")),
		formatAdminPoints(adminRightsCost(settings, "verify")),
		adminRightsModeText(settingString(settings, "locationMode", "none")),
		formatAdminPoints(adminRightsCost(settings, "location")),
	)
}

func adminRightsModeText(mode string) string {
	switch mode {
	case "none", "public", "free", "":
		return "不限制"
	case "points":
		return "积分查看"
	case "member_or_points":
		return "会员或积分"
	default:
		return "仅会员"
	}
}

func adminRightsKeyboardRows(plugin *model.PluginConfig) [][]models.InlineKeyboardButton {
	settings := adminPluginSettings(plugin)
	return [][]models.InlineKeyboardButton{
		{{Text: "查看验证 [" + adminRightsModeText(settingString(settings, "verifyMode", "none")) + "]", CallbackData: "admin:rights:select:verify"}},
		{{Text: "查看位置 [" + adminRightsModeText(settingString(settings, "locationMode", "none")) + "]", CallbackData: "admin:rights:select:location"}},
	}
}

func buildAdminRightsSelectText(ctx context.Context, botKey, kind string) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return "查看权限\n\n无法读取插件配置。"
	}
	plugin := adminPluginsForBot(state, botKey)["rights"]
	settings := adminPluginSettings(plugin)
	mode := settingString(settings, kind+"Mode", "none")
	return fmt.Sprintf("%s\n\n当前：%s\n扣积分：%s\n\n请选择查看策略。", adminRightsKindText(kind), adminRightsModeText(mode), formatAdminPoints(adminRightsCost(settings, kind)))
}

func buildAdminRightsSelectKeyboard(kind string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "仅会员可观看", CallbackData: "admin:rights:mode:" + kind + ":member"}},
		{{Text: "不限制", CallbackData: "admin:rights:mode:" + kind + ":none"}},
		{{Text: "积分查看", CallbackData: "admin:rights:mode:" + kind + ":points"}},
		{{Text: "返回权益", CallbackData: "admin:plugin:rights"}, {Text: "取消", CallbackData: "admin:cancel"}},
	}}
}

func buildAdminRightsCostText(ctx context.Context, botKey, kind string) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return "积分查看\n\n无法读取插件配置。"
	}
	plugin := adminPluginsForBot(state, botKey)["rights"]
	settings := adminPluginSettings(plugin)
	return fmt.Sprintf("%s\n\n当前扣积分：%s\n\n请选择每次查看需要扣除多少积分。", adminRightsKindText(kind), formatAdminPoints(adminRightsCost(settings, kind)))
}

func buildAdminRightsCostKeyboard(kind string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: "0", CallbackData: "admin:rights:cost:" + kind + ":0"},
			{Text: "1", CallbackData: "admin:rights:cost:" + kind + ":1"},
			{Text: "3", CallbackData: "admin:rights:cost:" + kind + ":3"},
		},
		{
			{Text: "5", CallbackData: "admin:rights:cost:" + kind + ":5"},
			{Text: "10", CallbackData: "admin:rights:cost:" + kind + ":10"},
			{Text: "20", CallbackData: "admin:rights:cost:" + kind + ":20"},
		},
		{{Text: "返回", CallbackData: "admin:rights:select:" + kind}, {Text: "取消", CallbackData: "admin:cancel"}},
	}}
}

func adminRightsKindText(kind string) string {
	if kind == "location" {
		return "查看位置"
	}
	return "查看验证"
}

func adminRightsCost(settings map[string]any, kind string) float64 {
	if v := settingFloat(settings, kind+"PointsCost", -1); v >= 0 {
		return v
	}
	return settingFloat(settings, "pointsCost", 0)
}

func adminSigninSummary(plugin *model.PluginConfig) string {
	settings := adminPluginSettings(plugin)
	channels := parseSignChannelsAny(settings["channels"])
	lines := []string{
		"当前签到",
		"必须关注频道：" + boolText(settingBool(settings, "followRequired", false), "开启", "关闭"),
		fmt.Sprintf("签到奖励：%s 积分", formatAdminPoints(settingFloat(settings, "rewardPoints", 0))),
		fmt.Sprintf("频道数量：%d", len(channels)),
	}
	for idx, channel := range channels {
		if idx >= 5 {
			lines = append(lines, "...")
			break
		}
		title := strings.TrimSpace(channel.Title)
		if title == "" {
			title = channel.ChatRef
		}
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, title))
	}
	return strings.Join(lines, "\n")
}

func adminSigninKeyboardRows(plugin *model.PluginConfig) [][]models.InlineKeyboardButton {
	settings := adminPluginSettings(plugin)
	followRequired := settingBool(settings, "followRequired", false)
	followLabel := "开启必须关注"
	followValue := "on"
	if followRequired {
		followLabel = "关闭必须关注"
		followValue = "off"
	}
	return [][]models.InlineKeyboardButton{
		{{Text: followLabel, CallbackData: "admin:signin:follow:" + followValue}},
		{{Text: "清空签到频道", CallbackData: "admin:signin:clear"}},
	}
}

func adminHelpSummary(plugin *model.PluginConfig) string {
	text := settingString(adminPluginSettings(plugin), "helpText", "请联系管理员获取帮助。")
	return "当前帮助文案：" + displayFooterText(text)
}

func adminHelpKeyboardRows() [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{{Text: "编辑帮助文案", CallbackData: "textedit:help"}},
	}
}

func setAdminRightsMode(ctx context.Context, botKey, scope, mode string) error {
	return updateAdminPlugin(ctx, botKey, "rights", func(plugin *model.PluginConfig) {
		if scope == "verify" || scope == "all" {
			plugin.Settings["verifyMode"] = mode
		}
		if scope == "location" || scope == "all" {
			plugin.Settings["locationMode"] = mode
		}
		plugin.Settings["memberOnly"] = mode != "none"
	})
}

func setAdminRightsCost(ctx context.Context, botKey string, kind string, cost float64) error {
	return updateAdminPlugin(ctx, botKey, "rights", func(plugin *model.PluginConfig) {
		if kind == "verify" || kind == "location" {
			plugin.Settings[kind+"PointsCost"] = cost
			return
		}
		plugin.Settings["pointsCost"] = cost
	})
}

func setAdminSignFollowRequired(ctx context.Context, botKey string, enabled bool) error {
	return updateAdminPlugin(ctx, botKey, "signin", func(plugin *model.PluginConfig) {
		plugin.Settings["followRequired"] = enabled
	})
}

func clearAdminSignChannels(ctx context.Context, botKey string) error {
	return updateAdminPlugin(ctx, botKey, "signin", func(plugin *model.PluginConfig) {
		plugin.Settings["channels"] = []any{}
	})
}

func upsertAdminSignChannel(ctx context.Context, botKey string, channel signChannelConfig) error {
	channel.ChatRef = strings.TrimSpace(channel.ChatRef)
	if channel.ChatRef == "" {
		return fmt.Errorf("签到频道标识为空")
	}
	if strings.TrimSpace(channel.Title) == "" {
		channel.Title = channel.ChatRef
	}
	return updateAdminPlugin(ctx, botKey, "signin", func(plugin *model.PluginConfig) {
		channels := parseSignChannelsAny(plugin.Settings["channels"])
		found := false
		for idx, item := range channels {
			if item.ChatRef == channel.ChatRef {
				channels[idx] = channel
				found = true
				break
			}
		}
		if !found {
			channels = append(channels, channel)
		}
		plugin.Settings["followRequired"] = true
		plugin.Settings["channels"] = adminSignChannelsToSettings(channels)
	})
}

func updateAdminPlugin(ctx context.Context, botKey, key string, update func(plugin *model.PluginConfig)) error {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return err
	}
	botCfg := state.Bots[botKey]
	if botCfg == nil {
		return fmt.Errorf("机器人配置不存在")
	}
	if botCfg.Plugins == nil {
		botCfg.Plugins = cloneAdminPlugins(state.Plugins)
	}
	plugin := botCfg.Plugins[key]
	if plugin == nil {
		if def := model.DefaultPluginConfigs()[key]; def != nil {
			botCfg.Plugins[key] = cloneAdminPlugins(map[string]*model.PluginConfig{key: def})[key]
			plugin = botCfg.Plugins[key]
		}
	}
	if plugin == nil {
		return fmt.Errorf("插件不存在")
	}
	if plugin.Settings == nil {
		plugin.Settings = map[string]any{}
	}
	update(plugin)
	return service.SysLazysheepTggo().SaveState(ctx, state)
}

func adminPluginSettings(plugin *model.PluginConfig) map[string]any {
	if plugin == nil || plugin.Settings == nil {
		return map[string]any{}
	}
	return plugin.Settings
}

func adminSignChannelsToSettings(channels []signChannelConfig) []any {
	items := make([]any, 0, len(channels))
	for _, channel := range channels {
		if strings.TrimSpace(channel.ChatRef) == "" {
			continue
		}
		items = append(items, map[string]any{
			"chatId": channel.ChatRef,
			"title":  channel.Title,
			"url":    channel.URL,
		})
	}
	return items
}

func adminCanConfigureFromMessage(ctx context.Context, botKey string, msg *models.Message) (bool, error) {
	if msg == nil {
		return false, nil
	}
	if userID := userIDFromMessage(msg); userID > 0 {
		return service.SysLazysheepTggo().IsBotAdmin(ctx, botKey, userID)
	}
	return ensureBotCreatorForChat(ctx, botKey, msg)
}

func adminChatRef(chat models.Chat) string {
	if username := strings.TrimSpace(chat.Username); username != "" {
		return "@" + strings.TrimPrefix(username, "@")
	}
	return strconv.FormatInt(chat.ID, 10)
}

func adminPluginsForBot(state *model.State, botKey string) map[string]*model.PluginConfig {
	if state == nil {
		return nil
	}
	if botCfg := state.Bots[botKey]; botCfg != nil && botCfg.Plugins != nil {
		return botCfg.Plugins
	}
	return state.Plugins
}

func adminPluginStats(plugins map[string]*model.PluginConfig) (enabled int, total int) {
	for _, item := range plugins {
		if item == nil {
			continue
		}
		total++
		if item.Enabled {
			enabled++
		}
	}
	return enabled, total
}

func sortedAdminPluginKeys(plugins map[string]*model.PluginConfig) []string {
	keys := make([]string, 0, len(plugins))
	for key := range plugins {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left := plugins[keys[i]]
		right := plugins[keys[j]]
		if left == nil || right == nil || left.Sort == right.Sort {
			return keys[i] < keys[j]
		}
		return left.Sort < right.Sort
	})
	return keys
}

func cloneAdminPlugins(plugins map[string]*model.PluginConfig) map[string]*model.PluginConfig {
	out := make(map[string]*model.PluginConfig, len(plugins))
	for key, item := range plugins {
		if item == nil {
			continue
		}
		copied := *item
		if item.Settings != nil {
			copied.Settings = make(map[string]any, len(item.Settings))
			for settingKey, value := range item.Settings {
				copied.Settings[settingKey] = value
			}
		}
		if item.BindingActions != nil {
			copied.BindingActions = append([]model.PluginBindingAction(nil), item.BindingActions...)
		}
		out[key] = &copied
	}
	return out
}
