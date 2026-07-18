// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/dao"
)

const (
	rightsPayloadVerifyPrefix   = "note_"
	rightsPayloadLocationPrefix = "loc_"
)

func isRightsPayload(text string) bool {
	payload := startPayload(text)
	return strings.HasPrefix(payload, rightsPayloadVerifyPrefix) || strings.HasPrefix(payload, rightsPayloadLocationPrefix)
}

func init() {
	RegisterBotPlugin(&contentRightsPlugin{})
}

type contentRightsPlugin struct{}

func (p *contentRightsPlugin) Key() string { return "rights" }

func (p *contentRightsPlugin) Handle(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, plugins map[string]*model.PluginConfig) (bool, error) {
	if req.Trigger != TriggerStart || req.Update == nil || req.Update.Message == nil || req.Update.Message.From == nil {
		return false, nil
	}
	payload := startPayload(req.Text)
	if payload == "" {
		return false, nil
	}
	g.Log().Debugf(ctx, "Telegram 内容权益入口 bot:%s payload:%s user:%d", req.BotKey, payload, req.Update.Message.From.ID)
	user := req.Update.Message.From
	_ = service.SysLazysheepTggo().TouchUser(ctx, &sysin.TouchUserInp{
		TelegramID:   user.ID,
		BotKey:       req.BotKey,
		Username:     user.Username,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		LanguageCode: user.LanguageCode,
		IsBot:        user.IsBot,
	})
	switch {
	case strings.HasPrefix(payload, rightsPayloadVerifyPrefix):
		code := strings.TrimPrefix(payload, rightsPayloadVerifyPrefix)
		return true, handleVerifyVideoReveal(ctx, b, req, cfg, code)
	case strings.HasPrefix(payload, rightsPayloadLocationPrefix):
		code := strings.TrimPrefix(payload, rightsPayloadLocationPrefix)
		return true, handleLocationReveal(ctx, b, req, cfg, plugins, code)
	default:
		return false, nil
	}
}

type rightsNoteRow struct {
	Id      int64  `json:"id"`
	BotId   int64  `json:"botId"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	BotKey  string `json:"botKey"`
	BotName string `json:"botName"`
}

type rightsVideoRow struct {
	SourceUrl   string  `json:"sourceUrl"`
	PreviewUrl  string  `json:"previewUrl"`
	TgFileId    string  `json:"tgFileId"`
	Duration    int     `json:"duration"`
	AspectRatio float64 `json:"aspectRatio"`
}

type rightsLocationRow struct {
	Title    string `json:"title"`
	SubTitle string `json:"subTitle"`
	Content  string `json:"content"`
}

func handleVerifyVideoReveal(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, code string) error {
	note, err := loadRightsNote(ctx, req.BotKey, code)
	if err != nil {
		return sendRightsText(ctx, b, req, fmt.Sprintf("内容不存在或已失效：%v", err))
	}
	if ok, text, err := ensureRightsAccess(ctx, cfg, req.BotKey, req.Update.Message.From.ID, "verify"); err != nil || !ok {
		if err != nil {
			text = fmt.Sprintf("权限校验失败：%v", err)
		}
		return sendRightsText(ctx, b, req, text)
	}
	videos, err := loadVerifyVideos(ctx, note.Id)
	if err != nil {
		return sendRightsText(ctx, b, req, fmt.Sprintf("查询验证视频失败：%v", err))
	}
	if len(videos) == 0 {
		return sendRightsText(ctx, b, req, "该内容没有可查看的验证视频。")
	}
	for _, item := range videos {
		video := rightsVideoInput(ctx, item)
		if video == nil {
			continue
		}
		width, height := rightsVideoDimensions(item.AspectRatio)
		if _, err = b.SendVideo(ctx, &bot.SendVideoParams{
			ChatID:            req.Update.Message.Chat.ID,
			Video:             video,
			Duration:          item.Duration,
			Width:             width,
			Height:            height,
			SupportsStreaming: true,
			Caption:           fmt.Sprintf("验证视频：%s", note.Code),
		}); err != nil {
			return err
		}
	}
	return nil
}

func handleLocationReveal(ctx context.Context, b *bot.Bot, req *PluginRequest, cfg *model.PluginConfig, plugins map[string]*model.PluginConfig, code string) error {
	note, err := loadRightsNote(ctx, req.BotKey, code)
	if err != nil {
		return sendRightsText(ctx, b, req, fmt.Sprintf("内容不存在或已失效：%v", err))
	}
	if ok, text, err := ensureRightsAccess(ctx, cfg, req.BotKey, req.Update.Message.From.ID, "location"); err != nil || !ok {
		if err != nil {
			text = fmt.Sprintf("权限校验失败：%v", err)
		}
		return sendRightsText(ctx, b, req, text)
	}
	location, err := loadNoteLocation(ctx, note.Id)
	if err != nil {
		return sendRightsText(ctx, b, req, fmt.Sprintf("查询位置失败：%v", err))
	}
	if location == nil {
		return sendRightsText(ctx, b, req, "该内容没有可查看的位置。")
	}
	text := formatLocationText(location)
	lat, lng, hasCoord := parseLocationCoord(location.Content)
	if hasCoord {
		_, err = b.SendVenue(ctx, &bot.SendVenueParams{
			ChatID:      req.Update.Message.Chat.ID,
			Latitude:    lat,
			Longitude:   lng,
			Title:       fallbackText(location.Title, note.Title),
			Address:     location.SubTitle,
			ReplyMarkup: mapKeyboard(location, plugins, lat, lng),
		})
		return err
	}
	return sendRightsHTML(ctx, b, req, text, mapKeyboard(location, plugins, lat, lng))
}

func loadRightsNote(ctx context.Context, botKey string, code string) (*rightsNoteRow, error) {
	code = strings.TrimSpace(code)
	if code == "" || strings.TrimSpace(botKey) == "" {
		return nil, fmt.Errorf("编号为空")
	}
	noteCols := dao.AddonLazysheepTggoNote.Columns()
	botCols := dao.AddonLazysheepTggoBot.Columns()
	var row rightsNoteRow
	err := dao.AddonLazysheepTggoNote.Ctx(ctx).As("n").
		LeftJoin(dao.AddonLazysheepTggoBot.Table()+" b", "b.id=n.bot_id").
		Fields("n."+noteCols.Id, "n."+noteCols.BotId, "n."+noteCols.Code, "n."+noteCols.Title, "b."+botCols.BotKey, "b."+botCols.BotName).
		Where("b."+botCols.BotKey, botKey).
		Where("n."+noteCols.Code, code).
		Where("n."+noteCols.Status, 1).
		Scan(&row)
	if err != nil {
		return nil, err
	}
	if row.Id == 0 {
		return nil, fmt.Errorf("编号不存在")
	}
	return &row, nil
}

func loadVerifyVideos(ctx context.Context, noteID int64) ([]rightsVideoRow, error) {
	itemCols := dao.AddonLazysheepTggoNoteItem.Columns()
	var rows []rightsVideoRow
	err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).
		Fields(itemCols.Content+" source_url", itemCols.PreviewUrl, itemCols.TgFileId, itemCols.Duration, itemCols.AspectRatio).
		Where(itemCols.NoteId, noteID).
		Where(itemCols.ItemType, "NOTE_TYPE_VIDEO").
		Where(itemCols.VerifyVideo, 1).
		Where(itemCols.Status, 1).
		OrderAsc(itemCols.ItemIndex).
		Scan(&rows)
	return rows, err
}

func loadNoteLocation(ctx context.Context, noteID int64) (*rightsLocationRow, error) {
	itemCols := dao.AddonLazysheepTggoNoteItem.Columns()
	var row rightsLocationRow
	err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).
		Fields(itemCols.Title, itemCols.SubTitle, itemCols.Content).
		Where(itemCols.NoteId, noteID).
		Where(itemCols.ItemType, "NOTE_TYPE_LOCATION").
		Where(itemCols.Status, 1).
		OrderAsc(itemCols.ItemIndex).
		Scan(&row)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(row.Title+row.SubTitle+row.Content) == "" {
		return nil, nil
	}
	return &row, nil
}

func ensureRightsAccess(ctx context.Context, cfg *model.PluginConfig, botKey string, telegramID int64, kind string) (bool, string, error) {
	mode := "none"
	if cfg != nil && cfg.Settings != nil {
		mode = settingString(cfg.Settings, kind+"Mode", mode)
		if mode == "member" && !settingBool(cfg.Settings, "memberOnly", true) {
			mode = "public"
		}
	}
	switch mode {
	case "public", "free", "none", "":
		return true, "", nil
	case "points":
		return deductRightsPoints(ctx, cfg, botKey, telegramID, kind)
	case "member_or_points":
		if isRightsMember(ctx, botKey, telegramID) {
			return true, "", nil
		}
		return deductRightsPoints(ctx, cfg, botKey, telegramID, kind)
	default:
		if isRightsMember(ctx, botKey, telegramID) {
			return true, "", nil
		}
		return false, settingString(cfg.Settings, "privateText", "请先完成会员或积分校验后查看隐藏内容。"), nil
	}
}

func isRightsMember(ctx context.Context, botKey string, telegramID int64) bool {
	cols := dao.AddonLazysheepTggoUser.Columns()
	count, err := dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where(cols.BotKey, botKey).
		Where(cols.TelegramId, telegramID).
		WhereGT(cols.MemberLevel, 0).
		Where(cols.Status, 1).
		Count()
	return err == nil && count > 0
}

func deductRightsPoints(ctx context.Context, cfg *model.PluginConfig, botKey string, telegramID int64, kind string) (bool, string, error) {
	cost := rightsCost(cfg, kind)
	if cost <= 0 {
		return true, "", nil
	}
	allowed := false
	err := dao.AddonLazysheepTggoUser.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		cols := dao.AddonLazysheepTggoUser.Columns()
		var row struct {
			Points float64 `json:"points"`
		}
		if err := dao.AddonLazysheepTggoUser.Ctx(ctx).Fields(cols.Points).
			Where(cols.BotKey, botKey).
			Where(cols.TelegramId, telegramID).
			Scan(&row); err != nil {
			return err
		}
		if row.Points < cost {
			return nil
		}
		now := gtime.Now()
		after := row.Points - cost
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
			"change_num":  -cost,
			"before_num":  row.Points,
			"after_num":   after,
			"action":      "content_rights",
			"remark":      rightsCostRemark(kind),
			"status":      1,
			"created_at":  now,
			"updated_at":  now,
		}).Insert()
		allowed = err == nil
		return err
	})
	if err != nil {
		return false, "", err
	}
	if !allowed {
		return false, fmt.Sprintf("积分不足，需要 %s 积分。", formatRightsNumber(cost)), nil
	}
	return true, "", nil
}

func rightsCost(cfg *model.PluginConfig, kind string) float64 {
	if cfg == nil || cfg.Settings == nil {
		return 0
	}
	if v := settingFloat(cfg.Settings, kind+"PointsCost", -1); v >= 0 {
		return v
	}
	return settingFloat(cfg.Settings, "pointsCost", 0)
}

func rightsCostRemark(kind string) string {
	if kind == "location" {
		return "查看位置"
	}
	return "查看验证视频"
}

func rightsVideoInput(ctx context.Context, item rightsVideoRow) models.InputFile {
	if strings.TrimSpace(item.SourceUrl) != "" {
		if filename, data, err := downloadRightsMedia(ctx, item.SourceUrl); err == nil && len(data) > 0 {
			return &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)}
		} else if err != nil {
			g.Log().Warningf(ctx, "下载权益验证视频失败 url:%s err:%+v", item.SourceUrl, err)
		}
	}
	for _, value := range []string{item.TgFileId, item.PreviewUrl, item.SourceUrl} {
		value = strings.TrimSpace(value)
		if value != "" {
			return &models.InputFileString{Data: value}
		}
	}
	return nil
}

func rightsVideoDimensions(aspectRatio float64) (int, int) {
	if aspectRatio <= 0 {
		return 0, 0
	}
	width := 720
	height := int(float64(width) / aspectRatio)
	if height <= 0 {
		return 0, 0
	}
	return width, height
}

func parseLocationCoord(raw string) (float64, float64, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lat, lng, true
}

func formatLocationText(item *rightsLocationRow) string {
	if item == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if strings.TrimSpace(item.Title) != "" {
		parts = append(parts, "<b>"+html.EscapeString(item.Title)+"</b>")
	}
	if strings.TrimSpace(item.SubTitle) != "" {
		parts = append(parts, html.EscapeString(item.SubTitle))
	}
	if strings.TrimSpace(item.Content) != "" {
		parts = append(parts, "<code>"+html.EscapeString(item.Content)+"</code>")
	}
	return strings.Join(parts, "\n")
}

func mapKeyboard(item *rightsLocationRow, plugins map[string]*model.PluginConfig, lat, lng float64) *models.InlineKeyboardMarkup {
	if item == nil || lat == 0 || lng == 0 {
		return nil
	}
	providers := []string{"amap", "baidu", "tencent"}
	if cfg := plugins["map"]; cfg != nil && cfg.Enabled {
		providers = settingStringSlice(cfg.Settings, "providerButtons", providers)
	}
	buttons := make([]models.InlineKeyboardButton, 0, len(providers))
	for _, provider := range providers {
		switch provider {
		case "amap":
			buttons = append(buttons, models.InlineKeyboardButton{Text: "高德地图", URL: amapURL(item, lat, lng)})
		case "baidu":
			buttons = append(buttons, models.InlineKeyboardButton{Text: "百度地图", URL: baiduURL(item, lat, lng)})
		case "tencent":
			buttons = append(buttons, models.InlineKeyboardButton{Text: "腾讯地图", URL: tencentURL(item, lat, lng)})
		}
	}
	if len(buttons) == 0 {
		return nil
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{buttons}}
}

func amapURL(item *rightsLocationRow, lat, lng float64) string {
	q := url.Values{}
	q.Set("position", fmt.Sprintf("%f,%f", lng, lat))
	q.Set("name", fallbackText(item.Title, item.SubTitle))
	return "https://uri.amap.com/marker?" + q.Encode()
}

func baiduURL(item *rightsLocationRow, lat, lng float64) string {
	q := url.Values{}
	q.Set("location", fmt.Sprintf("%f,%f", lat, lng))
	q.Set("title", fallbackText(item.Title, item.SubTitle))
	q.Set("content", item.SubTitle)
	q.Set("output", "html")
	return "https://api.map.baidu.com/marker?" + q.Encode()
}

func tencentURL(item *rightsLocationRow, lat, lng float64) string {
	q := url.Values{}
	q.Set("marker", fmt.Sprintf("coord:%f,%f;title:%s;addr:%s", lat, lng, fallbackText(item.Title, item.SubTitle), item.SubTitle))
	return "https://apis.map.qq.com/uri/v1/marker?" + q.Encode()
}

func sendRightsText(ctx context.Context, b *bot.Bot, req *PluginRequest, text string) error {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: req.Update.Message.Chat.ID,
		Text:   text,
	})
	return err
}

func sendRightsHTML(ctx context.Context, b *bot.Bot, req *PluginRequest, text string, keyboard *models.InlineKeyboardMarkup) error {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      req.Update.Message.Chat.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	return err
}

func settingStringSlice(settings map[string]any, key string, fallback []string) []string {
	if settings == nil {
		return fallback
	}
	raw, ok := settings[key].([]any)
	if !ok {
		return fallback
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func formatRightsNumber(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}
