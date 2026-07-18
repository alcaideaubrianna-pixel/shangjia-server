// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/internal/dao"
)

const (
	defaultCaptionTemplate = "编号：<code>{code}</code>\n\n{verify_link}\n{location_link}\n\n<b>{title}</b>\n{text}\n\n{footer}"
	legacyCaptionTemplate  = "<b>{title}</b>\n\n{text}\n\n编号：<code>{code}</code>\n\n{verify_link}\n{location_link}\n\n{footer}"
)

func (s *sLazySheepTGGo) pushCollectedNote(ctx context.Context, botKey string, binding *model.BindingRecord, noteID int64, fallbackChatID int64) ([]*models.Message, error) {
	if binding == nil {
		return nil, nil
	}
	reviewMode := !binding.AutoPush && binding.ReviewChatID != 0
	targetChatID := pushTargetChatID(binding, fallbackChatID)
	if targetChatID == 0 {
		return nil, nil
	}
	rt := s.runtime.get(botKey)
	if rt == nil || rt.client == nil {
		return nil, gerror.New("机器人运行实例不存在，请先启动机器人")
	}
	started := time.Now()
	note, err := s.loadPushNote(ctx, noteID)
	if err != nil {
		return nil, err
	}
	plugins := s.collectorPlugins(ctx, botKey)
	settings := map[string]any{}
	if cfg := plugins["collector"]; cfg != nil && cfg.Settings != nil {
		settings = cfg.Settings
	}
	settings = withBindingCollectorSettings(settings, plugins, binding.PluginState)
	caption := buildNoteCaption(note, rt.cfg, binding, settings, plugins)
	g.Log().Debugf(ctx, "%s 推送采集笔记开始 botKey:%s binding:%s noteId:%d targetChat:%d reviewMode:%t", pullTraceTag(ctx), botKey, binding.Key, noteID, targetChatID, reviewMode)
	msgs, err := s.sendCollectedNoteMainMessage(ctx, rt.client, rt.cfg.Token, targetChatID, note, caption, settings)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	msg := msgs[0]
	g.Log().Debugf(ctx, "%s 推送采集笔记完成 botKey:%s binding:%s noteId:%d messageID:%d elapsed:%s", pullTraceTag(ctx), botKey, binding.Key, noteID, msg.ID, time.Since(started).Round(time.Millisecond))
	cols := dao.AddonLazysheepTggoNote.Columns()
	update := g.Map{cols.UpdatedAt: gtime.Now()}
	if reviewMode {
		update[cols.ReviewMessageId] = msg.ID
	} else {
		update[cols.PublishMessageId] = msg.ID
	}
	_, _ = dao.AddonLazysheepTggoNote.Ctx(ctx).WherePri(noteID).Data(update).Update()
	return msgs, nil
}

func (s *sLazySheepTGGo) sendCollectedNoteMainMessage(ctx context.Context, client *bot.Bot, token string, chatID int64, note *pushNote, caption string, settings map[string]any) ([]*models.Message, error) {
	items, merged := selectQuickMediaItemsForPush(note.Items, settings)
	g.Log().Debugf(ctx, "%s 推送媒体选择 note:%d mergeVerifyInGroup:%t items:%d selected:%d", pullTraceTag(ctx), note.Id, pushSettingBool(settings, "mergeVerifyInGroup", false), len(note.Items), len(items))
	mediaAssets, err := buildQuickMediaAssets(ctx, items)
	if err != nil {
		return nil, err
	}
	if len(mediaAssets) == 0 {
		msg, err := client.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      caption,
			ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			return nil, err
		}
		return []*models.Message{msg}, nil
	}
	msgs, err := s.sendQuickMediaAssetsWithMode(ctx, client, token, chatID, mediaAssets, caption, merged)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		msg, err := client.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      caption,
			ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			return nil, err
		}
		return []*models.Message{msg}, nil
	}
	return msgs, nil
}

type pushNote struct {
	Id              int64
	Code            string
	Title           string
	TextContent     string
	HasVerifyVideo  bool
	HasLocation     bool
	LocationTitle   string
	LocationAddress string
	LocationContent string
	Items           []noteItem
	VerifyVideos    []pushNoteVideo
}

type pushNoteVideo struct {
	SourceURL   string
	PreviewURL  string
	TgFileID    string
	Duration    int
	AspectRatio float64
}

func (s *sLazySheepTGGo) loadPushNote(ctx context.Context, noteID int64) (*pushNote, error) {
	cols := dao.AddonLazysheepTggoNote.Columns()
	var row struct {
		Id          int64  `json:"id"`
		Code        string `json:"code"`
		Title       string `json:"title"`
		TextContent string `json:"textContent"`
	}
	if err := dao.AddonLazysheepTggoNote.Ctx(ctx).
		Fields(cols.Id, cols.Code, cols.Title, cols.TextContent).
		Where(cols.Id, noteID).
		Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "查询笔记失败")
	}
	if row.Id == 0 {
		return nil, gerror.New("笔记不存在")
	}
	itemCols := dao.AddonLazysheepTggoNoteItem.Columns()
	var items []struct {
		ItemType    string  `json:"itemType"`
		Title       string  `json:"title"`
		SubTitle    string  `json:"subTitle"`
		Content     string  `json:"content"`
		VerifyVideo int     `json:"verifyVideo"`
		PreviewUrl  string  `json:"previewUrl"`
		TgFileId    string  `json:"tgFileId"`
		Duration    int     `json:"duration"`
		AspectRatio float64 `json:"aspectRatio"`
	}
	if err := dao.AddonLazysheepTggoNoteItem.Ctx(ctx).
		Fields(itemCols.ItemType, itemCols.Title, itemCols.SubTitle, itemCols.Content, itemCols.VerifyVideo, itemCols.PreviewUrl, itemCols.TgFileId, itemCols.Duration, itemCols.AspectRatio).
		Where(itemCols.NoteId, noteID).
		OrderAsc(itemCols.ItemIndex).
		Scan(&items); err != nil {
		return nil, gerror.Wrap(err, "查询笔记资源失败")
	}
	out := &pushNote{Id: row.Id, Code: row.Code, Title: row.Title, TextContent: row.TextContent}
	for _, item := range items {
		out.Items = append(out.Items, noteItem{
			Type:        item.ItemType,
			Title:       item.Title,
			SubTitle:    item.SubTitle,
			Content:     item.Content,
			Duration:    item.Duration,
			VerifyVideo: item.VerifyVideo > 0,
			AspectRatio: item.AspectRatio,
			TgFileID:    item.TgFileId,
		})
		if item.ItemType == noteTypeLocation {
			out.HasLocation = true
			out.LocationTitle = item.Title
			out.LocationAddress = item.SubTitle
			out.LocationContent = item.Content
		}
		if item.ItemType == noteTypeVideo && item.VerifyVideo > 0 {
			out.HasVerifyVideo = true
			out.VerifyVideos = append(out.VerifyVideos, pushNoteVideo{
				SourceURL:   item.Content,
				PreviewURL:  item.PreviewUrl,
				TgFileID:    item.TgFileId,
				Duration:    item.Duration,
				AspectRatio: item.AspectRatio,
			})
		}
	}
	return out, nil
}

func (s *sLazySheepTGGo) pushCollectedNotePublicExtras(ctx context.Context, client *bot.Bot, chatID int64, note *pushNote, binding *model.BindingRecord, settings map[string]any) error {
	if client == nil || note == nil || binding == nil || chatID == 0 {
		return nil
	}
	if binding.VerifyEnabled && pushSettingBool(settings, "showVerifyLink", true) && !pushSettingBool(settings, "mergeVerifyInGroup", false) {
		for _, item := range note.VerifyVideos {
			if err := sendPublicVerifyVideo(ctx, client, chatID, note, item); err != nil {
				return err
			}
		}
	}
	if binding.LocationEnabled && pushSettingBool(settings, "showLocationLink", true) && note.HasLocation {
		return sendPublicLocation(ctx, client, chatID, note)
	}
	return nil
}

func sendPublicVerifyVideo(ctx context.Context, client *bot.Bot, chatID int64, note *pushNote, item pushNoteVideo) error {
	video := publicVideoInput(ctx, item)
	if video == nil {
		return nil
	}
	width, height := videoDimensions(item.AspectRatio)
	_, err := client.SendVideo(ctx, &bot.SendVideoParams{
		ChatID:            chatID,
		Video:             video,
		Duration:          item.Duration,
		Width:             width,
		Height:            height,
		SupportsStreaming: true,
		Caption:           fmt.Sprintf("验证视频：%s", note.Code),
	})
	return err
}

func publicVideoInput(ctx context.Context, item pushNoteVideo) models.InputFile {
	if strings.TrimSpace(item.SourceURL) != "" {
		if filename, data, err := downloadQuickMedia(ctx, item.SourceURL, noteTypeVideo, 0); err == nil && len(data) > 0 {
			return &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)}
		} else if err != nil {
			g.Log().Warningf(ctx, "下载公开验证视频失败 url:%s err:%+v", item.SourceURL, err)
		}
	}
	for _, value := range []string{item.TgFileID, item.PreviewURL, item.SourceURL} {
		if value = strings.TrimSpace(value); value != "" {
			return &models.InputFileString{Data: value}
		}
	}
	return nil
}

func sendPublicLocation(ctx context.Context, client *bot.Bot, chatID int64, note *pushNote) error {
	lat, lng, hasCoord := parsePublicLocationCoord(note.LocationContent)
	if hasCoord {
		_, _ = client.SendVenue(ctx, &bot.SendVenueParams{
			ChatID:    chatID,
			Latitude:  lat,
			Longitude: lng,
			Title:     fallbackPublicText(note.LocationTitle, note.Title),
			Address:   note.LocationAddress,
		})
	}
	text := formatPublicLocationText(note)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err := client.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
	return err
}

func parsePublicLocationCoord(raw string) (float64, float64, bool) {
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

func formatPublicLocationText(note *pushNote) string {
	if note == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if strings.TrimSpace(note.LocationTitle) != "" {
		parts = append(parts, "<b>"+html.EscapeString(note.LocationTitle)+"</b>")
	}
	if strings.TrimSpace(note.LocationAddress) != "" {
		parts = append(parts, html.EscapeString(note.LocationAddress))
	}
	if strings.TrimSpace(note.LocationContent) != "" {
		parts = append(parts, "<code>"+html.EscapeString(note.LocationContent)+"</code>")
	}
	return strings.Join(parts, "\n")
}

func fallbackPublicText(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "位置"
}

func (s *sLazySheepTGGo) collectorPlugins(ctx context.Context, botKey string) map[string]*model.PluginConfig {
	state, err := s.GetState(ctx)
	if err != nil || state == nil {
		return model.DefaultPluginConfigs()
	}
	if cfg := state.Bots[botKey]; cfg != nil && cfg.Plugins != nil {
		return cfg.Plugins
	}
	return state.Plugins
}

func buildNoteCaption(note *pushNote, cfg *model.BotConfig, binding *model.BindingRecord, settings map[string]any, plugins map[string]*model.PluginConfig) string {
	template := normalizeCaptionTemplate(pushSettingString(settings, "captionTemplate", defaultCaptionTemplate))
	revealLinks := collectorRevealLinksEnabled(plugins, binding.PluginState)
	verifyLink := ""
	if revealLinks && note.HasVerifyVideo && binding.VerifyEnabled && pushSettingBool(settings, "showVerifyLink", true) {
		verifyLink = buildDeepLink(cfg, "note_"+note.Code, pushSettingString(settings, "verifyLinkText", "📒 点击查看验证视频"))
	}
	locationLink := ""
	if revealLinks && note.HasLocation && binding.LocationEnabled && pushSettingBool(settings, "showLocationLink", true) {
		locationLink = buildDeepLink(cfg, "loc_"+note.Code, pushSettingString(settings, "locationLinkText", "📌 点击查看位置"))
	}
	locationBlock := ""
	if !revealLinks && note.HasLocation && binding.LocationEnabled && pushSettingBool(settings, "showLocationLink", true) {
		locationBlock = buildQuickLocationBlock([]quickLocationItem{{
			Title:    note.LocationTitle,
			SubTitle: note.LocationAddress,
			Content:  note.LocationContent,
		}})
	}
	footer := resolveContentFooter(plugins, settings, binding.PluginState)
	replacer := strings.NewReplacer(
		"{title}", html.EscapeString(note.Title),
		"{text}", compactCaptionBlankLines(strings.Join([]string{html.EscapeString(note.TextContent), locationBlock}, "\n\n")),
		"{code}", html.EscapeString(note.Code),
		"{verify_link}", verifyLink,
		"{location_link}", locationLink,
		"{footer}", footer,
	)
	return compactCaptionBlankLines(replacer.Replace(template))
}

func normalizeCaptionTemplate(template string) string {
	if strings.TrimSpace(template) == strings.TrimSpace(legacyCaptionTemplate) {
		return defaultCaptionTemplate
	}
	return template
}

func compactCaptionBlankLines(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func pushSettingString(settings map[string]any, key, fallback string) string {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(string); ok {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return fallback
}

func pushSettingBool(settings map[string]any, key string, fallback bool) bool {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(bool); ok {
		return v
	}
	return fallback
}

func buildDeepLink(cfg *model.BotConfig, payload string, text string) string {
	url := buildDeepLinkURL(cfg, payload)
	if url == "" {
		return ""
	}
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(url), html.EscapeString(text))
}

func buildDeepLinkURL(cfg *model.BotConfig, payload string) string {
	username := ""
	if cfg != nil {
		username = strings.TrimPrefix(strings.TrimSpace(cfg.Username), "@")
	}
	if username == "" {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", username, payload)
}
