// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/dao"
)

const (
	invitePayloadPrefix       = "ref_"
	memberInvitePayloadPrefix = "mem_"
)

func init() {
	RegisterBotPlugin(&profilePlugin{})
	RegisterMessageHandler(&profileCommand{})
	RegisterMessageHandler(&helpCommand{})
	RegisterCallbackHandler(&profileRefreshCallback{})
	RegisterCallbackHandler(&profileSignCallback{})
}

type profilePlugin struct{}

func (p *profilePlugin) Key() string { return "profile" }

func isMemberInvitePayload(text string) bool {
	return strings.HasPrefix(startPayload(text), memberInvitePayloadPrefix)
}

func (p *profilePlugin) Handle(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, plugins map[string]*model.PluginConfig) (bool, error) {
	if req.Trigger != TriggerStart || req.Update == nil || req.Update.Message == nil || req.Update.Message.From == nil {
		return false, nil
	}
	payload := startPayload(req.Text)
	if strings.HasPrefix(payload, memberInvitePayloadPrefix) {
		return handleMemberInviteStart(ctx, b, req, payload, plugins)
	}
	if !strings.HasPrefix(payload, invitePayloadPrefix) {
		return false, nil
	}
	user := req.Update.Message.From
	_ = service.SysLazysheepTggo().TouchUser(ctx, &lsysin.TouchUserInp{
		TelegramID:   user.ID,
		BotKey:       req.BotKey,
		Username:     user.Username,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		LanguageCode: user.LanguageCode,
		IsBot:        user.IsBot,
	})
	recorded, err := recordInviteStart(ctx, req.BotKey, payload, user.ID, cfg)
	if err != nil {
		return true, err
	}
	text := settingString(cfg.Settings, "inviteStartText", "欢迎使用机器人。")
	if recorded {
		text = settingString(cfg.Settings, "inviteRecordedText", "邀请关系已记录，欢迎使用机器人。")
	}
	params := &bot.SendMessageParams{
		ChatID: req.Update.Message.Chat.ID,
		Text:   text,
	}
	if menuCfg := plugins["menu"]; menuCfg != nil && menuCfg.Enabled {
		params.ReplyMarkup = buildReplyKeyboard(menuCfg.Settings, plugins, false, currentBotRole(ctx))
	}
	_, err = b.SendMessage(ctx, params)
	return true, err
}

func handleMemberInviteStart(ctx context.Context, b *bot.Bot, req *PluginRequest, payload string, plugins map[string]*model.PluginConfig) (bool, error) {
	user := req.Update.Message.From
	token := strings.TrimPrefix(strings.TrimSpace(payload), memberInvitePayloadPrefix)
	memberCfg := plugins["member"]
	if token == "" || memberCfg == nil || token != settingString(memberCfg.Settings, "inviteToken", "") {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: req.Update.Message.Chat.ID, Text: "会员邀请链接无效或已失效。"})
		return true, err
	}
	_ = service.SysLazysheepTggo().TouchUser(ctx, &lsysin.TouchUserInp{
		TelegramID:   user.ID,
		BotKey:       req.BotKey,
		Username:     user.Username,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		LanguageCode: user.LanguageCode,
		IsBot:        user.IsBot,
	})
	if err := activateMemberByInvite(ctx, req.BotKey, user.ID); err != nil {
		return true, err
	}
	text := "会员已开通，欢迎使用。"
	if memberCfg != nil {
		text = settingString(memberCfg.Settings, "renewText", text)
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: req.Update.Message.Chat.ID, Text: text})
	return true, err
}

func activateMemberByInvite(ctx context.Context, botKey string, userID int64) error {
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return nil
	}
	cols := dao.AddonLazysheepTggoUser.Columns()
	_, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, userID).
		Data(g.Map{cols.MemberLevel: 1, cols.Status: 1, cols.UpdatedAt: gtime.Now()}).
		Update()
	return err
}

type profileCommand struct{}

func (h *profileCommand) Key() string              { return "profile" }
func (h *profileCommand) Pattern() string          { return "profile" }
func (h *profileCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *profileCommand) Description() string      { return "个人中心" }
func (h *profileCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "profile") {
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
	cfg := currentBotPlugins(ctx)["profile"]
	text, keyboard := buildProfileMessage(ctx, botKey, msg.From, cfg)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

type helpCommand struct{}

func (h *helpCommand) Key() string              { return "help" }
func (h *helpCommand) Pattern() string          { return "help" }
func (h *helpCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *helpCommand) Description() string      { return "帮助" }
func (h *helpCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "help") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	cfg := currentBotPlugins(ctx)["help"]
	text := "请联系管理员获取帮助。"
	if cfg != nil {
		text = settingString(cfg.Settings, "helpText", text)
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
	return err
}

type profileRefreshCallback struct{}

func (h *profileRefreshCallback) Key() string              { return "profile_refresh" }
func (h *profileRefreshCallback) Pattern() string          { return "profile:refresh" }
func (h *profileRefreshCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *profileRefreshCallback) Description() string      { return "刷新个人中心" }
func (h *profileRefreshCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return nil
	}
	if !pluginEnabled(ctx, "profile") {
		return replyCallback(ctx, b, update, "个人中心插件未启用。")
	}
	cfg := currentBotPlugins(ctx)["profile"]
	text, keyboard := buildProfileMessage(ctx, currentBotKey(ctx), &update.CallbackQuery.From, cfg)
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID, Text: "已刷新。"}); err != nil {
		return err
	}
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        text,
		ReplyMarkup: keyboard,
	})
	return err
}

type profileSignCallback struct{}

func (h *profileSignCallback) Key() string              { return "profile_sign" }
func (h *profileSignCallback) Pattern() string          { return "profile:sign" }
func (h *profileSignCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *profileSignCallback) Description() string      { return "个人中心签到" }
func (h *profileSignCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return nil
	}
	if !pluginEnabled(ctx, "signin") {
		return replyCallback(ctx, b, update, "签到插件未启用。")
	}
	return sendSignPrompt(ctx, b, update.CallbackQuery.Message.Message.Chat.ID, update.CallbackQuery.From.ID, currentBotKey(ctx))
}

type profileUserRow struct {
	TelegramID     int64       `json:"telegramId"`
	Username       string      `json:"username"`
	FirstName      string      `json:"firstName"`
	LastName       string      `json:"lastName"`
	MemberLevel    int         `json:"memberLevel"`
	Points         float64     `json:"points"`
	MemberExpireAt *gtime.Time `json:"memberExpireAt"`
	Status         int         `json:"status"`
}

func buildProfileMessage(ctx context.Context, botKey string, from *models.User, cfg *model.PluginConfig) (string, *models.InlineKeyboardMarkup) {
	userID := int64(0)
	if from != nil {
		userID = from.ID
	}
	row := loadProfileUser(ctx, botKey, userID)
	name := profileDisplayName(row, from)
	pointName := "积分"
	if cfg != nil {
		pointName = settingString(cfg.Settings, "pointName", pointName)
	}
	signDays := countProfileSigns(ctx, botKey, userID)
	inviteCount := countProfileInvites(ctx, botKey, userID)
	inviteURL := buildProfileInviteURL(ctx, botKey, userID)
	statusText, expireText := profilePluginStatus(row)
	lines := []string{
		"📋 个人中心",
		"",
		fmt.Sprintf("账号名称 %s", name),
		fmt.Sprintf("账户ID %d", userID),
		fmt.Sprintf("账号级别 %d", row.MemberLevel),
		fmt.Sprintf("当前%s %s 分", pointName, formatPoints(row.Points)),
		"────────────────",
		fmt.Sprintf("插件状态 %s", statusText),
	}
	if expireText != "" {
		lines = append(lines, fmt.Sprintf("到期时间 %s", expireText))
	}
	lines = append(lines,
		"────────────────",
		"使用统计",
		fmt.Sprintf("累计签到 %d天", signDays),
		"────────────────",
		"我的邀请链接",
		"点击即可复制发送好友获得积分",
		inviteURL,
		fmt.Sprintf("已邀请好友 %d 人", inviteCount),
	)
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: profileRefreshText(cfg), CallbackData: "profile:refresh"}, {Text: profileSignText(cfg), CallbackData: "profile:sign"}},
	}}
	return strings.Join(lines, "\n"), keyboard
}

func loadProfileUser(ctx context.Context, botKey string, userID int64) profileUserRow {
	cols := dao.AddonLazysheepTggoUser.Columns()
	row := profileUserRow{TelegramID: userID, Status: 1}
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return row
	}
	_ = dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.TelegramId, cols.Username, cols.FirstName, cols.LastName, cols.MemberLevel, cols.Points, "member_expire_at", cols.Status).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, userID).
		Scan(&row)
	return row
}

func profileDisplayName(row profileUserRow, from *models.User) string {
	name := strings.TrimSpace(row.Username)
	if name != "" {
		return "@" + strings.TrimPrefix(name, "@")
	}
	name = strings.TrimSpace(row.FirstName + " " + row.LastName)
	if name != "" {
		return name
	}
	if from != nil {
		name = strings.TrimSpace(from.Username)
		if name != "" {
			return "@" + strings.TrimPrefix(name, "@")
		}
		name = strings.TrimSpace(from.FirstName + " " + from.LastName)
		if name != "" {
			return name
		}
		return fmt.Sprintf("%d", from.ID)
	}
	return fmt.Sprintf("%d", row.TelegramID)
}

func profilePluginStatus(row profileUserRow) (status string, expire string) {
	if row.MemberLevel <= 0 || row.Status != 1 {
		return "未开通", ""
	}
	if row.MemberExpireAt == nil {
		return "已开通", "未配置"
	}
	expire = row.MemberExpireAt.String()
	if row.MemberExpireAt.Before(gtime.Now()) {
		return "已过期", expire
	}
	return "已开通", expire
}

func countProfileSigns(ctx context.Context, botKey string, userID int64) int {
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return 0
	}
	count, err := g.DB().Model("hg_addon_lazysheep_tggo_sign_log").
		Where("bot_key", botKey).
		Where("telegram_id", userID).
		Where("status", 1).
		Count()
	if err != nil {
		return 0
	}
	return count
}

func buildProfileInviteURL(ctx context.Context, botKey string, userID int64) string {
	username := ""
	if state, err := service.SysLazysheepTggo().GetState(ctx); err == nil && state != nil {
		if cfg := state.Bots[botKey]; cfg != nil {
			username = strings.TrimPrefix(strings.TrimSpace(cfg.Username), "@")
		}
	}
	if username == "" {
		username = "bot"
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", username, invitePayload(userID))
}

func invitePayload(userID int64) string {
	if userID <= 0 {
		return invitePayloadPrefix + "0"
	}
	return invitePayloadPrefix + strings.ToUpper(strconv.FormatInt(userID, 36))
}

func inviteUserID(payload string) int64 {
	code := strings.TrimPrefix(strings.TrimSpace(payload), invitePayloadPrefix)
	if code == "" || code == "0" {
		return 0
	}
	id, err := strconv.ParseInt(strings.ToLower(code), 36, 64)
	if err != nil {
		return 0
	}
	return id
}

func recordInviteStart(ctx context.Context, botKey, payload string, inviteeID int64, cfg *model.PluginConfig) (bool, error) {
	inviterID := inviteUserID(payload)
	if strings.TrimSpace(botKey) == "" || inviterID == 0 || inviteeID == 0 || inviterID == inviteeID {
		return false, nil
	}
	reward := float64(0)
	if cfg != nil {
		reward = settingFloat(cfg.Settings, "inviteRewardPoints", 0)
	}
	recorded := false
	err := dao.AddonLazysheepTggoUser.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		exists, err := g.DB().Model("hg_addon_lazysheep_tggo_invite_log").
			Where("bot_key", botKey).
			Where("invitee_telegram_id", inviteeID).
			Count()
		if err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}
		now := gtime.Now()
		if _, err = g.DB().Model("hg_addon_lazysheep_tggo_invite_log").Data(g.Map{
			"bot_key":             botKey,
			"inviter_telegram_id": inviterID,
			"invitee_telegram_id": inviteeID,
			"payload":             payload,
			"reward_points":       reward,
			"status":              1,
			"created_at":          now,
			"updated_at":          now,
		}).Insert(); err != nil {
			return err
		}
		recorded = true
		if reward <= 0 {
			return nil
		}
		return addInviteRewardPoints(ctx, botKey, inviterID, reward, now)
	})
	return recorded, err
}

func addInviteRewardPoints(ctx context.Context, botKey string, telegramID int64, reward float64, now *gtime.Time) error {
	cols := dao.AddonLazysheepTggoUser.Columns()
	var row struct {
		Points float64 `json:"points"`
	}
	if err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Fields(cols.Points).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, telegramID).
		Scan(&row); err != nil {
		return err
	}
	before := row.Points
	after := before + reward
	if _, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, telegramID).
		Data(g.Map{cols.Points: after, cols.UpdatedAt: now}).
		Update(); err != nil {
		return err
	}
	_, err := g.DB().Model("hg_addon_lazysheep_tggo_points_log").Data(g.Map{
		"bot_key":     botKey,
		"telegram_id": telegramID,
		"change_num":  reward,
		"before_num":  before,
		"after_num":   after,
		"action":      "invite_reward",
		"remark":      "邀请奖励",
		"status":      1,
		"created_at":  now,
		"updated_at":  now,
	}).Insert()
	return err
}

func countProfileInvites(ctx context.Context, botKey string, userID int64) int {
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return 0
	}
	count, err := g.DB().Model("hg_addon_lazysheep_tggo_invite_log").
		Where("bot_key", botKey).
		Where("inviter_telegram_id", userID).
		Where("status", 1).
		Count()
	if err != nil {
		return 0
	}
	return count
}

func profileRefreshText(plugin *model.PluginConfig) string {
	if plugin == nil || plugin.Settings == nil {
		return "刷新"
	}
	return settingString(plugin.Settings, "refreshText", "刷新")
}

func profileSignText(plugin *model.PluginConfig) string {
	if plugin == nil || plugin.Settings == nil {
		return "签到"
	}
	return settingString(plugin.Settings, "signText", "签到")
}
