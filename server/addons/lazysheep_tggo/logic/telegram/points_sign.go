// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/dao"
)

type signChannelConfig struct {
	ChatRef string
	Title   string
	URL     string
}

type signPromptConfig struct {
	FollowRequired bool
	PromptText     string
	FinishText     string
	OpenText       string
	VerifyText     string
	SuccessText    string
	FailText       string
	RewardPoints   float64
	Channels       []signChannelConfig
}

func init() {
	RegisterMessageHandler(&pointsCommand{})
	RegisterCallbackHandler(&pointsRefreshCallback{})
	RegisterCallbackHandler(&pointsSignCallback{})
	RegisterCallbackHandler(&signChannelVerifyCallback{})
	RegisterCallbackHandler(&signFinishCallback{})
}

type pointsCommand struct{}

func (h *pointsCommand) Key() string              { return "points" }
func (h *pointsCommand) Pattern() string          { return "points" }
func (h *pointsCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *pointsCommand) Description() string      { return "积分中心" }
func (h *pointsCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "points") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil || msg.From == nil {
		return nil
	}
	botKey := currentBotKey(ctx)
	_ = service.SysLazysheepTggo().TouchUser(ctx, &lsysin.TouchUserInp{
		TelegramID:   msg.From.ID,
		BotKey:       botKey,
		Username:     msg.From.Username,
		FirstName:    msg.From.FirstName,
		LastName:     msg.From.LastName,
		LanguageCode: msg.From.LanguageCode,
		IsBot:        msg.From.IsBot,
	})
	cfg := currentBotPlugins(ctx)["points"]
	balanceText, ruleText, pointName, points := loadPointSummary(ctx, botKey, msg.From.ID, cfg)
	logs, _ := loadRecentPointLogs(ctx, botKey, msg.From.ID, 5)
	text := strings.TrimSpace(balanceText)
	if text == "" {
		text = "当前积分：{points}"
	}
	text = strings.ReplaceAll(text, "{points}", formatPoints(points))
	text = strings.ReplaceAll(text, "{point_name}", pointName)
	if ruleText != "" {
		text += "\n\n" + ruleText
	}
	if logText := formatPointLogs(logs); logText != "" {
		text += "\n\n最近记录\n" + logText
	}
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: pointRefreshText(cfg), CallbackData: "points:refresh"},
				{Text: pointSignText(cfg), CallbackData: "points:sign"},
			},
		},
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

type pointsRefreshCallback struct{}

func (h *pointsRefreshCallback) Key() string              { return "points_refresh" }
func (h *pointsRefreshCallback) Pattern() string          { return "points:refresh" }
func (h *pointsRefreshCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *pointsRefreshCallback) Description() string      { return "刷新积分中心" }
func (h *pointsRefreshCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		return nil
	}
	if !pluginEnabled(ctx, "points") {
		return nil
	}
	botKey := currentBotKey(ctx)
	cfg := currentBotPlugins(ctx)["points"]
	balanceText, ruleText, pointName, points := loadPointSummary(ctx, botKey, update.CallbackQuery.From.ID, cfg)
	logs, _ := loadRecentPointLogs(ctx, botKey, update.CallbackQuery.From.ID, 5)
	text := strings.TrimSpace(balanceText)
	if text == "" {
		text = "当前积分：{points}"
	}
	text = strings.ReplaceAll(text, "{points}", formatPoints(points))
	text = strings.ReplaceAll(text, "{point_name}", pointName)
	if ruleText != "" {
		text += "\n\n" + ruleText
	}
	if logText := formatPointLogs(logs); logText != "" {
		text += "\n\n最近记录\n" + logText
	}
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            "已刷新。",
		ShowAlert:       false,
	})
	if err != nil {
		return err
	}
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      msg.Chat.ID,
		MessageID:   msg.ID,
		Text:        text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: pointRefreshText(cfg), CallbackData: "points:refresh"}, {Text: pointSignText(cfg), CallbackData: "points:sign"}}}},
	})
	return err
}

type pointsSignCallback struct{}

func (h *pointsSignCallback) Key() string              { return "points_sign" }
func (h *pointsSignCallback) Pattern() string          { return "points:sign" }
func (h *pointsSignCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *pointsSignCallback) Description() string      { return "打开签到入口" }
func (h *pointsSignCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return nil
	}
	if !pluginEnabled(ctx, "signin") {
		return replyCallback(ctx, b, update, "签到插件未启用。")
	}
	return sendSignPrompt(ctx, b, update.CallbackQuery.Message.Message.Chat.ID, update.CallbackQuery.From.ID, currentBotKey(ctx))
}

type signChannelVerifyCallback struct{}

func (h *signChannelVerifyCallback) Key() string              { return "sign_channel_verify" }
func (h *signChannelVerifyCallback) Pattern() string          { return "sign:verify:" }
func (h *signChannelVerifyCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *signChannelVerifyCallback) Description() string      { return "验证签到频道" }
func (h *signChannelVerifyCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	if !pluginEnabled(ctx, "signin") {
		return replyCallback(ctx, b, update, "签到插件未启用。")
	}
	parts := strings.Split(update.CallbackQuery.Data, ":")
	if len(parts) != 3 {
		return nil
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil || idx < 0 {
		return replyCallback(ctx, b, update, "频道索引错误。")
	}
	cfg, err := loadSignPromptConfig(ctx, currentBotKey(ctx))
	if err != nil {
		return err
	}
	if idx >= len(cfg.Channels) {
		return replyCallback(ctx, b, update, "频道不存在。")
	}
	ok, detail, err := verifySignChannel(ctx, b, currentBotKey(ctx), update.CallbackQuery.From.ID, cfg.Channels[idx])
	if err != nil {
		return err
	}
	return replyCallback(ctx, b, update, detailIf(ok, detail, "未关注该频道，请先关注后再验证。"))
}

type signFinishCallback struct{}

func (h *signFinishCallback) Key() string              { return "sign_finish" }
func (h *signFinishCallback) Pattern() string          { return "sign:finish" }
func (h *signFinishCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *signFinishCallback) Description() string      { return "完成签到校验" }
func (h *signFinishCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return nil
	}
	if !pluginEnabled(ctx, "signin") {
		return replyCallback(ctx, b, update, "签到插件未启用。")
	}
	cfg, err := loadSignPromptConfig(ctx, currentBotKey(ctx))
	if err != nil {
		return err
	}
	for _, channel := range cfg.Channels {
		ok, _, err := verifySignChannel(ctx, b, currentBotKey(ctx), update.CallbackQuery.From.ID, channel)
		if err != nil {
			return err
		}
		if !ok {
			return replyCallback(ctx, b, update, cfg.FailText)
		}
	}
	_ = service.SysLazysheepTggo().TouchUser(ctx, &lsysin.TouchUserInp{
		TelegramID:   update.CallbackQuery.From.ID,
		BotKey:       currentBotKey(ctx),
		Username:     update.CallbackQuery.From.Username,
		FirstName:    update.CallbackQuery.From.FirstName,
		LastName:     update.CallbackQuery.From.LastName,
		LanguageCode: update.CallbackQuery.From.LanguageCode,
		IsBot:        update.CallbackQuery.From.IsBot,
	})
	ok, err := completeSignIn(ctx, currentBotKey(ctx), update.CallbackQuery.From.ID, cfg)
	if err != nil {
		return err
	}
	if !ok {
		return replyCallback(ctx, b, update, "今日已完成签到。")
	}
	return replyCallback(ctx, b, update, buildSignSuccessText(ctx, currentBotKey(ctx), update.CallbackQuery.From.ID, cfg))
}

func sendSignPrompt(ctx context.Context, b *bot.Bot, chatID int64, userID int64, botKey string) error {
	cfg, err := loadSignPromptConfig(ctx, botKey)
	if err != nil {
		return err
	}
	if len(cfg.Channels) == 0 || !cfg.FollowRequired {
		ok, err := completeSignIn(ctx, botKey, userID, cfg)
		if err != nil {
			return err
		}
		if !ok {
			return replyCallbackText(ctx, b, chatID, "今日已完成签到。")
		}
		return replyCallbackText(ctx, b, chatID, buildSignSuccessText(ctx, botKey, userID, cfg))
	}
	if userID != 0 {
		_ = service.SysLazysheepTggo().TouchUser(ctx, &lsysin.TouchUserInp{TelegramID: userID, BotKey: botKey})
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(cfg.Channels)+1)
	for idx, channel := range cfg.Channels {
		row := make([]models.InlineKeyboardButton, 0, 2)
		if channel.URL != "" {
			row = append(row, models.InlineKeyboardButton{Text: openText(cfg), URL: channel.URL})
		}
		row = append(row, models.InlineKeyboardButton{Text: verifyText(cfg), CallbackData: fmt.Sprintf("sign:verify:%d", idx)})
		rows = append(rows, row)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: cfg.FinishText, CallbackData: "sign:finish"}})
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        buildSignPromptText(ctx, botKey, userID, cfg),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

func loadSignPromptConfig(ctx context.Context, botKey string) (*signPromptConfig, error) {
	cfg := &signPromptConfig{
		FollowRequired: true,
		PromptText:     "请先关注以下频道，再点击验证按钮完成签到。",
		FinishText:     "全部验证并签到",
		OpenText:       "打开频道",
		VerifyText:     "验证关注",
		SuccessText:    "签到成功，感谢关注。",
		FailText:       "请先完成频道关注后再签到。",
		RewardPoints:   0,
	}
	plugins := currentBotPlugins(ctx)
	if plugin := plugins["signin"]; plugin != nil && plugin.Settings != nil {
		cfg.FollowRequired = settingBool(plugin.Settings, "followRequired", cfg.FollowRequired)
		cfg.PromptText = settingString(plugin.Settings, "promptText", cfg.PromptText)
		cfg.FinishText = settingString(plugin.Settings, "finishText", cfg.FinishText)
		cfg.OpenText = settingString(plugin.Settings, "openText", cfg.OpenText)
		cfg.VerifyText = settingString(plugin.Settings, "verifyText", cfg.VerifyText)
		cfg.SuccessText = settingString(plugin.Settings, "successText", cfg.SuccessText)
		cfg.FailText = settingString(plugin.Settings, "failText", cfg.FailText)
		cfg.RewardPoints = settingFloat(plugin.Settings, "rewardPoints", cfg.RewardPoints)
		cfg.Channels = parseSignChannelsAny(plugin.Settings["channels"])
	}
	cols := dao.AddonLazysheepTggoBot.Columns()
	var row struct {
		SignFollow   int    `json:"signFollow"`
		SignChannels string `json:"signChannels"`
	}
	if err := dao.AddonLazysheepTggoBot.Ctx(ctx).
		Fields(cols.SignFollow, cols.SignChannels).
		Where(cols.BotKey, botKey).
		Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "查询签到配置失败")
	}
	if row.SignFollow > 0 {
		cfg.FollowRequired = true
	}
	if channels := parseSignChannels(row.SignChannels); len(channels) > 0 {
		cfg.Channels = channels
	}
	return cfg, nil
}

func loadPointSummary(ctx context.Context, botKey string, userID int64, plugin *model.PluginConfig) (balanceText string, ruleText string, pointName string, points float64) {
	balanceText = "当前积分：{points}"
	pointName = "积分"
	if plugin != nil && plugin.Settings != nil {
		balanceText = settingString(plugin.Settings, "balanceText", balanceText)
		ruleText = settingString(plugin.Settings, "ruleText", "")
		pointName = settingString(plugin.Settings, "pointName", pointName)
	}
	cols := dao.AddonLazysheepTggoUser.Columns()
	var row struct {
		Points float64 `json:"points"`
	}
	_ = dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.Points).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, userID).
		Scan(&row)
	return balanceText, ruleText, pointName, row.Points
}

type pointLogItem struct {
	ChangeNum float64     `json:"changeNum"`
	BeforeNum float64     `json:"beforeNum"`
	AfterNum  float64     `json:"afterNum"`
	Action    string      `json:"action"`
	Remark    string      `json:"remark"`
	CreatedAt *gtime.Time `json:"createdAt"`
}

func loadRecentPointLogs(ctx context.Context, botKey string, userID int64, limit int) ([]pointLogItem, error) {
	if limit <= 0 {
		limit = 5
	}
	var rows []pointLogItem
	err := g.DB().Model("hg_addon_lazysheep_tggo_points_log").
		Where("bot_key", botKey).
		Where("telegram_id", userID).
		OrderDesc("id").
		Limit(limit).
		Scan(&rows)
	return rows, err
}

func formatPointLogs(items []pointLogItem) string {
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		prefix := "+"
		if item.ChangeNum < 0 {
			prefix = ""
		}
		lines = append(lines, fmt.Sprintf("%s%s %s", prefix, formatPoints(item.ChangeNum), strings.TrimSpace(item.Remark)))
	}
	return strings.Join(lines, "\n")
}

func completeSignIn(ctx context.Context, botKey string, userID int64, cfg *signPromptConfig) (bool, error) {
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return false, nil
	}
	if cfg == nil {
		cfg = &signPromptConfig{}
	}
	day := gtime.Now().Format("Y-m-d")
	inserted := false
	err := dao.AddonLazysheepTggoUser.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		exists, err := g.DB().Model("hg_addon_lazysheep_tggo_sign_log").
			Where("bot_key", botKey).
			Where("telegram_id", userID).
			Where("sign_day", day).
			Count()
		if err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}
		now := gtime.Now()
		if _, err = g.DB().Model("hg_addon_lazysheep_tggo_sign_log").Data(g.Map{
			"bot_key":        botKey,
			"telegram_id":    userID,
			"sign_day":       day,
			"channel_total":  len(cfg.Channels),
			"verified_total": len(cfg.Channels),
			"points_reward":  cfg.RewardPoints,
			"remark":         "签到成功",
			"status":         1,
			"created_at":     now,
			"updated_at":     now,
		}).Insert(); err != nil {
			return err
		}
		inserted = true
		if cfg.RewardPoints == 0 {
			return nil
		}
		cols := dao.AddonLazysheepTggoUser.Columns()
		var row struct {
			Points float64 `json:"points"`
		}
		if err = dao.AddonLazysheepTggoUser.Ctx(ctx).
			Fields(cols.Points).
			Where(cols.BotKey, botKey).
			Where(cols.TelegramId, userID).
			Scan(&row); err != nil {
			return err
		}
		before := row.Points
		after := before + cfg.RewardPoints
		if _, err = dao.AddonLazysheepTggoUser.Ctx(ctx).
			Where(cols.BotKey, botKey).
			Where(cols.TelegramId, userID).
			Data(g.Map{cols.Points: after, cols.UpdatedAt: now}).
			Update(); err != nil {
			return err
		}
		_, err = g.DB().Model("hg_addon_lazysheep_tggo_points_log").Data(g.Map{
			"bot_key":     botKey,
			"telegram_id": userID,
			"change_num":  cfg.RewardPoints,
			"before_num":  before,
			"after_num":   after,
			"action":      "sign_reward",
			"remark":      "签到奖励",
			"status":      1,
			"created_at":  now,
			"updated_at":  now,
		}).Insert()
		return err
	})
	if err != nil {
		return false, err
	}
	return inserted, nil
}

func verifySignChannel(ctx context.Context, b *bot.Bot, botKey string, userID int64, channel signChannelConfig) (bool, string, error) {
	ref := strings.TrimSpace(channel.ChatRef)
	if ref == "" {
		return false, "频道配置缺少 chatId。", nil
	}
	member, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{
		ChatID: channelRef(ref),
		UserID: userID,
	})
	if err != nil {
		return false, "", err
	}
	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return true, "已关注该频道。", nil
	case models.ChatMemberTypeRestricted:
		if member.Restricted != nil && member.Restricted.IsMember {
			return true, "已关注该频道。", nil
		}
	}
	return false, "未关注该频道。", nil
}

func parseSignChannels(raw string) []signChannelConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var maps []map[string]any
	if err := json.Unmarshal([]byte(raw), &maps); err == nil {
		out := make([]signChannelConfig, 0, len(maps))
		for _, item := range maps {
			ch := signChannelConfig{
				ChatRef: stringAny(item["chatId"]),
				Title:   stringAny(item["title"]),
				URL:     stringAny(firstAny(item["url"], item["inviteUrl"], item["link"])),
			}
			if ch.ChatRef == "" {
				ch.ChatRef = stringAny(firstAny(item["chat"], item["username"], item["id"]))
			}
			if ch.URL == "" && strings.HasPrefix(ch.ChatRef, "@") {
				ch.URL = "https://t.me/" + strings.TrimPrefix(ch.ChatRef, "@")
			}
			if ch.Title == "" {
				ch.Title = ch.ChatRef
			}
			if ch.ChatRef != "" {
				out = append(out, ch)
			}
		}
		return out
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		out := make([]signChannelConfig, 0, len(list))
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			ch := signChannelConfig{ChatRef: item, Title: item}
			if strings.HasPrefix(item, "@") {
				ch.URL = "https://t.me/" + strings.TrimPrefix(item, "@")
			}
			out = append(out, ch)
		}
		return out
	}
	return nil
}

func parseSignChannelsAny(raw any) []signChannelConfig {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		return parseSignChannels(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return parseSignChannels(string(data))
	}
}

func buildSignPromptText(ctx context.Context, botKey string, userID int64, cfg *signPromptConfig) string {
	lines := []string{cfg.PromptText}
	if cfg.RewardPoints > 0 {
		lines = append(lines, fmt.Sprintf("完成后可获得 %s 积分。", formatPoints(cfg.RewardPoints)))
	}
	lines = append(lines, fmt.Sprintf("当前积分：%s", formatPoints(loadUserPoints(ctx, botKey, userID))))
	if len(cfg.Channels) > 0 {
		lines = append(lines, "", "需要关注：")
	}
	for idx, channel := range cfg.Channels {
		label := channel.Title
		if strings.TrimSpace(label) == "" {
			label = channel.ChatRef
		}
		lines = append(lines, fmt.Sprintf("%d. %s", idx+1, label))
	}
	return strings.Join(lines, "\n")
}

func buildSignSuccessText(ctx context.Context, botKey string, userID int64, cfg *signPromptConfig) string {
	lines := []string{cfg.SuccessText}
	if cfg.RewardPoints > 0 {
		lines = append(lines, fmt.Sprintf("本次获得：%s 积分", formatPoints(cfg.RewardPoints)))
	}
	lines = append(lines, fmt.Sprintf("当前积分：%s", formatPoints(loadUserPoints(ctx, botKey, userID))))
	return strings.Join(lines, "\n")
}

func loadUserPoints(ctx context.Context, botKey string, userID int64) float64 {
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return 0
	}
	cols := dao.AddonLazysheepTggoUser.Columns()
	var row struct {
		Points float64 `json:"points"`
	}
	_ = dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.Points).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, userID).
		Scan(&row)
	return row.Points
}

func pointRefreshText(plugin *model.PluginConfig) string {
	if plugin == nil || plugin.Settings == nil {
		return "刷新余额"
	}
	return settingString(plugin.Settings, "refreshText", "刷新余额")
}

func pointSignText(plugin *model.PluginConfig) string {
	if plugin == nil || plugin.Settings == nil {
		return "去签到"
	}
	return settingString(plugin.Settings, "signText", "去签到")
}

func openText(cfg *signPromptConfig) string {
	return cfg.OpenText
}

func verifyText(cfg *signPromptConfig) string {
	return cfg.VerifyText
}

func detailIf(ok bool, success, fallback string) string {
	if ok {
		return success
	}
	return fallback
}

func replyCallbackText(ctx context.Context, b *bot.Bot, chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	return err
}

func channelRef(ref string) any {
	if ref == "" {
		return ref
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil && id != 0 {
		return id
	}
	return ref
}

func stringAny(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	case json.Number:
		return x.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func settingFloat(settings map[string]any, key string, fallback float64) float64 {
	if settings == nil {
		return fallback
	}
	switch v := settings[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	}
	if s := strings.TrimSpace(fmt.Sprint(settings[key])); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return fallback
}

func firstAny(values ...any) any {
	for _, v := range values {
		if strings.TrimSpace(fmt.Sprint(v)) != "" {
			return v
		}
	}
	return nil
}

func formatPoints(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
