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
	"time"

	"hotgo/addons/lazysheep_tggo/logic/shared"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

func init() {
	RegisterMessageHandler(&startCommand{})
	RegisterMessageHandler(&menuButtonMessage{})
	RegisterMessageHandler(&bindReviewCommand{})
	RegisterMessageHandler(&bindPublishCommand{})
	RegisterMessageHandler(&bindCommand{})
	RegisterMessageHandler(&pullCommand{})
	RegisterMessageHandler(&settingsCommand{})
	RegisterMessageHandler(&syncCommand{})
	RegisterMessageHandler(&pauseCommand{})
	RegisterMessageHandler(&resetCommand{})
	RegisterMessageHandler(&clearCommand{})
	RegisterMessageHandler(&signCommand{})

	RegisterCallbackHandler(&reviewApproveCallback{})
	RegisterCallbackHandler(&reviewLocationCallback{})
	RegisterCallbackHandler(&reviewVerifyCallback{})
	RegisterCallbackHandler(&collectorCallback{})
}

type startCommand struct{}

func (h *startCommand) Key() string              { return "start" }
func (h *startCommand) Pattern() string          { return "start" }
func (h *startCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *startCommand) Description() string      { return "记录用户身份并欢迎关注" }
func (h *startCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	text := ""
	if msg := messageFromUpdate(update); msg != nil {
		text = msg.Text
	}
	return Dispatch(ctx, b, &PluginRequest{
		Trigger: TriggerStart,
		BotKey:  currentBotKey(ctx),
		Text:    text,
		Update:  update,
	})
}

type menuButtonMessage struct{}

func (h *menuButtonMessage) Key() string              { return "menu_button" }
func (h *menuButtonMessage) Pattern() string          { return "" }
func (h *menuButtonMessage) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *menuButtonMessage) Description() string      { return "底部菜单按钮调度" }
func (h *menuButtonMessage) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil || msg.Text == "" {
		return nil
	}
	return Dispatch(ctx, b, &PluginRequest{
		Trigger: TriggerMenuButton,
		BotKey:  currentBotKey(ctx),
		Text:    strings.TrimSpace(msg.Text),
		Update:  update,
	})
}

type bindCommand struct{}

func (h *bindCommand) Key() string              { return "bind" }
func (h *bindCommand) Pattern() string          { return "bind" }
func (h *bindCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *bindCommand) Description() string      { return "绑定资源链接" }
func (h *bindCommand) Match(update *models.Update) bool {
	return bindCommandMatch(update, h.Pattern(), h.MatchType(), h.Key(), "绑定")
}
func (h *bindCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	return handleBindSource(ctx, b, update, "", "/bind", "/绑定", "绑定")
}

type bindReviewCommand struct{}

func (h *bindReviewCommand) Key() string              { return "bind_review" }
func (h *bindReviewCommand) Pattern() string          { return "bind_review" }
func (h *bindReviewCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *bindReviewCommand) Description() string      { return "绑定审核采集链接" }
func (h *bindReviewCommand) Match(update *models.Update) bool {
	return bindCommandMatch(update, h.Pattern(), h.MatchType(), h.Key(), "绑定审核")
}
func (h *bindReviewCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	return handleBindSource(ctx, b, update, "review", "/bind_review", "/绑定审核", "绑定审核")
}

type bindPublishCommand struct{}

func (h *bindPublishCommand) Key() string              { return "bind_publish" }
func (h *bindPublishCommand) Pattern() string          { return "bind_publish" }
func (h *bindPublishCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *bindPublishCommand) Description() string      { return "绑定发布频道" }
func (h *bindPublishCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	botKey := currentBotKey(ctx)
	reply, err := service.SysLazysheepTggo().SetBindingPublishChat(ctx, botKey, msg.Chat.ID)
	if err != nil {
		reply = fmt.Sprintf("绑定发布频道失败：%v", err)
	}
	_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   reply,
	})
	if sendErr != nil {
		return sendErr
	}
	return err
}

func bindCommandMatch(update *models.Update, pattern string, matchType bot.MatchType, key string, aliases ...string) bool {
	if matchBindMessage(messageFromUpdate(update), pattern, matchType, key, aliases...) {
		return true
	}
	return false
}

func matchBindMessage(msg *models.Message, pattern string, matchType bot.MatchType, key string, aliases ...string) bool {
	if msg == nil {
		return false
	}
	if matchBindTextAlias(msg.Text, key) {
		return true
	}
	for _, e := range msg.Entities {
		if e.Type != models.MessageEntityTypeBotCommand {
			continue
		}
		if e.Offset != 0 && matchType == bot.MatchTypeCommandStartOnly {
			continue
		}
		end := e.Offset + e.Length
		if end > len(msg.Text) || e.Offset < 0 {
			continue
		}
		if msg.Text[e.Offset+1:end] == pattern {
			return true
		}
	}
	text := strings.TrimSpace(msg.Text)
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || !strings.HasPrefix(text, alias) {
			continue
		}
		tail := strings.TrimSpace(strings.TrimPrefix(text, alias))
		if tail == "" || strings.HasPrefix(tail, "http://") || strings.HasPrefix(tail, "https://") {
			return true
		}
	}
	return false
}

func matchBindTextAlias(text, key string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, alias := range bindTextAliases(key) {
		if text == alias || strings.HasPrefix(text, alias+" ") || strings.HasPrefix(text, alias+"@") {
			return true
		}
	}
	return false
}

func bindTextAliases(key string) []string {
	switch key {
	case "bind":
		return []string{"/bind", "/绑定", "绑定"}
	case "bind_review":
		return []string{"/bind_review", "/绑定审核", "绑定审核"}
	default:
		return nil
	}
}

func handleBindSource(ctx context.Context, b *bot.Bot, update *models.Update, mode string, commands ...string) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	botKey := currentBotKey(ctx)
	if msg.From != nil {
		_ = service.SysLazysheepTggo().TouchUser(ctx, &sysin.TouchUserInp{
			TelegramID:   msg.From.ID,
			BotKey:       botKey,
			Username:     msg.From.Username,
			FirstName:    msg.From.FirstName,
			LastName:     msg.From.LastName,
			LanguageCode: msg.From.LanguageCode,
			IsBot:        msg.From.IsBot,
		})
	}
	args := bindCommandArgs(msg.Text, commands...)
	if args == "" {
		return sendPlainText(ctx, b, msg.Chat.ID, fmt.Sprintf("请发送 %s <BangChat链接>", commands[0]))
	}
	if botKey == "" {
		return sendPlainText(ctx, b, msg.Chat.ID, "当前命令上下文缺少 bot 标识，请先检查 webhook 入口。")
	}
	sourceURL := firstField(args)
	operatorID := int64(0)
	if msg.From != nil {
		operatorID = msg.From.ID
	}
	if err := service.SysLazysheepTggo().BindSource(ctx, &sysin.BindSourceInp{
		BotKey:     botKey,
		ChatID:     msg.Chat.ID,
		OperatorID: operatorID,
		Mode:       mode,
		SourceURL:  sourceURL,
	}); err != nil {
		sendErr := sendPlainText(ctx, b, msg.Chat.ID, fmt.Sprintf("绑定失败：%v", err))
		if sendErr != nil {
			return sendErr
		}
		return err
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   collectorBindText(ctx, sourceURL),
	})
	if err == nil {
		deleteMessageLater(b, msg.Chat.ID, sent.ID)
		userID := int64(0)
		if msg.From != nil {
			userID = msg.From.ID
		}
		go func() {
			notifyCtx := WithBotKey(context.Background(), botKey)
			if notifyErr := service.SysLazysheepTggo().NotifyBindingCreated(notifyCtx, botKey, msg.Chat.ID, sourceURL, userID, mode); notifyErr != nil {
				g.Log().Warningf(ctx, "发送绑定通知失败 bot:%s chat:%d err:%+v", botKey, msg.Chat.ID, notifyErr)
			}
		}()
		if panelErr := sendBindingConfigPanel(ctx, b, botKey, msg.Chat.ID, userID); panelErr != nil {
			g.Log().Warningf(ctx, "发送绑定配置面板失败 bot:%s chat:%d err:%+v", botKey, msg.Chat.ID, panelErr)
		}
	}
	return err
}

func bindCommandArgs(text string, commands ...string) string {
	args := commandArgs(text, commands...)
	if strings.TrimSpace(args) != strings.TrimSpace(text) {
		return args
	}
	text = strings.TrimSpace(text)
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" || !strings.HasPrefix(text, command) {
			continue
		}
		tail := strings.TrimSpace(strings.TrimPrefix(text, command))
		if strings.HasPrefix(tail, "http://") || strings.HasPrefix(tail, "https://") {
			return tail
		}
	}
	return args
}

func firstField(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func collectorBindText(ctx context.Context, sourceURL string) string {
	text := "绑定已保存：{source}\n\n发送 pull 或 拉取 可立即拉取。\n发送 pull 10 或 拉取 10 可拉取最近 10 条消息。"
	if cfg := currentBotPlugins(ctx)["collector"]; cfg != nil {
		text = settingString(cfg.Settings, "quickBindText", text)
	}
	return strings.ReplaceAll(text, "{source}", sourceURL)
}

type pullCommand struct{}

func (h *pullCommand) Key() string              { return "pull" }
func (h *pullCommand) Pattern() string          { return "pull" }
func (h *pullCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *pullCommand) Description() string      { return "主动触发采集" }
func (h *pullCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	botKey := currentBotKey(ctx)
	args := commandArgs(msg.Text, "/pull", "pull", "/拉取", "拉取")
	if settingsKeyword(args) {
		if err := sendBindingConfigPanel(ctx, b, botKey, msg.Chat.ID, userIDFromMessage(msg)); err != nil {
			return err
		}
		return nil
	}
	limit := 0
	sourceURL := ""
	retryOld := false
	if args != "" {
		fields := strings.Fields(args)
		if len(fields) > 0 {
			for _, field := range fields {
				if pullRetryKeyword(field) {
					retryOld = true
					continue
				}
				if n, parseErr := strconv.Atoi(field); parseErr == nil && n > 0 && limit == 0 {
					limit = n
					continue
				}
				if sourceURL == "" {
					sourceURL = field
				}
			}
		}
	}
	modeText := resolvePullModeText(ctx, botKey, sourceURL, msg.Chat.ID)
	progress, sendProgressErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   pullProgressText(modeText, limit),
	})
	if sendProgressErr == nil && progress != nil {
		deleteMessageLater(b, msg.Chat.ID, progress.ID)
	}

	taskCtx, cancel := context.WithTimeout(WithBotKey(context.Background(), botKey), 15*time.Minute)
	lastProgressText := ""
	taskCtx = shared.WithPullProgressReporter(taskCtx, func(text string) {
		if sendProgressErr != nil || progress == nil || strings.TrimSpace(text) == "" || text == lastProgressText {
			return
		}
		lastProgressText = text
		_, _ = b.EditMessageText(taskCtx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: progress.ID,
			Text:      text,
		})
	})
	go func() {
		defer cancel()
		result, err := service.SysLazysheepTggo().PullNow(taskCtx, &sysin.PullInp{
			BotKey:    botKey,
			SourceURL: sourceURL,
			ChatID:    msg.Chat.ID,
			Limit:     limit,
			Retry:     retryOld,
		})
		if err != nil {
			g.Log().Warningf(taskCtx, "Telegram pull task failed bot:%s chat:%d err:%+v", botKey, msg.Chat.ID, err)
			deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, fmt.Sprintf("采集失败：%v", err))
			return
		}
		deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, result)
	}()
	return nil
}

type settingsCommand struct{}

func (h *settingsCommand) Key() string              { return "channel_settings" }
func (h *settingsCommand) Pattern() string          { return "配置" }
func (h *settingsCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *settingsCommand) Description() string      { return "显示当前频道配置面板" }
func (h *settingsCommand) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil {
		return false
	}
	return settingsKeyword(msg.Text)
}
func (h *settingsCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	return sendBindingConfigPanel(ctx, b, currentBotKey(ctx), msg.Chat.ID, userIDFromMessage(msg))
}

type syncCommand struct{}

func (h *syncCommand) Key() string              { return "sync_channel_notes" }
func (h *syncCommand) Pattern() string          { return "刷新" }
func (h *syncCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *syncCommand) Description() string      { return "同步刷新当前频道商品" }
func (h *syncCommand) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	return text == "刷新" || text == "/刷新" || text == "同步" || text == "/同步" || strings.EqualFold(text, "sync")
}
func (h *syncCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	if ok, err := ensureBotCreatorForChat(ctx, currentBotKey(ctx), msg); err != nil {
		return err
	} else if !ok {
		return sendPlainText(ctx, b, msg.Chat.ID, "只有机器人创建者可以同步刷新当前频道。")
	}
	botKey := currentBotKey(ctx)
	progress, sendProgressErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "正在同步当前频道商品，请稍候...",
	})
	if sendProgressErr == nil && progress != nil {
		deleteMessageLater(b, msg.Chat.ID, progress.ID)
	}
	taskCtx, cancel := context.WithTimeout(WithBotKey(context.Background(), botKey), 15*time.Minute)
	go func() {
		defer cancel()
		result, err := service.SysLazysheepTggo().PullNow(taskCtx, &sysin.PullInp{
			BotKey: botKey,
			ChatID: msg.Chat.ID,
			Retry:  true,
			Sync:   true,
		})
		if err != nil {
			g.Log().Warningf(taskCtx, "Telegram sync task failed bot:%s chat:%d err:%+v", botKey, msg.Chat.ID, err)
			deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, fmt.Sprintf("同步失败：%v", err))
			return
		}
		deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, "同步完成：\n"+result)
	}()
	return nil
}

type pauseCommand struct{}

func (h *pauseCommand) Key() string              { return "pause_channel_work" }
func (h *pauseCommand) Pattern() string          { return "暂停" }
func (h *pauseCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *pauseCommand) Description() string      { return "暂停当前频道采集和推送" }
func (h *pauseCommand) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil {
		return false
	}
	if msg.Chat.ID >= 0 {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	return text == "暂停" || text == "/暂停" || text == "取消" || text == "/取消" || strings.EqualFold(text, "pause") || strings.EqualFold(text, "cancel")
}
func (h *pauseCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	if ok, err := ensureBotCreatorForChat(ctx, currentBotKey(ctx), msg); err != nil {
		return err
	} else if !ok {
		return sendPlainText(ctx, b, msg.Chat.ID, "只有机器人创建者可以暂停当前频道。")
	}
	text, err := service.SysLazysheepTggo().PauseBindingWork(ctx, currentBotKey(ctx), msg.Chat.ID)
	if err != nil {
		return sendPlainText(ctx, b, msg.Chat.ID, fmt.Sprintf("暂停失败：%v", err))
	}
	return sendPlainText(ctx, b, msg.Chat.ID, text)
}

type resetCommand struct{}

func (h *resetCommand) Key() string              { return "reset_pull" }
func (h *resetCommand) Pattern() string          { return "重置" }
func (h *resetCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *resetCommand) Description() string      { return "重置当前频道采集记录" }
func (h *resetCommand) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	return text == "重置" || text == "/重置" || strings.EqualFold(text, "reset")
}
func (h *resetCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	if ok, err := ensureBotCreatorForChat(ctx, currentBotKey(ctx), msg); err != nil {
		return err
	} else if !ok {
		return sendPlainText(ctx, b, msg.Chat.ID, "只有机器人创建者可以重置当前频道记录。")
	}
	text, err := service.SysLazysheepTggo().ResetBindingPull(ctx, currentBotKey(ctx), msg.Chat.ID)
	if err != nil {
		return sendPlainText(ctx, b, msg.Chat.ID, fmt.Sprintf("重置失败：%v", err))
	}
	return sendPlainText(ctx, b, msg.Chat.ID, text)
}

type clearCommand struct{}

func (h *clearCommand) Key() string              { return "clear_channel_notes" }
func (h *clearCommand) Pattern() string          { return "清空" }
func (h *clearCommand) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *clearCommand) Description() string      { return "清空当前频道笔记" }
func (h *clearCommand) Match(update *models.Update) bool {
	msg := messageFromUpdate(update)
	if msg == nil {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	return text == "清空" || text == "/清空" || text == "频道清空" || text == "清空并拉取"
}
func (h *clearCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	deleteMessageLater(b, msg.Chat.ID, msg.ID)
	if ok, err := ensureBotCreatorForChat(ctx, currentBotKey(ctx), msg); err != nil {
		return err
	} else if !ok {
		return sendPlainText(ctx, b, msg.Chat.ID, "只有机器人创建者可以清空当前频道笔记。")
	}
	text := strings.TrimSpace(msg.Text)
	if text != "频道清空" && text != "清空并拉取" {
		return sendPlainText(ctx, b, msg.Chat.ID, "清空会删除当前频道已记录的频道消息，清理全部笔记，并重置采集记录。\n如确认，请发送：清空并拉取")
	}
	botKey := currentBotKey(ctx)
	progress, sendProgressErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "已开始清空当前频道，完成后会自动重新拉取。",
	})
	if sendProgressErr == nil && progress != nil {
		deleteMessageLater(b, msg.Chat.ID, progress.ID)
	}
	taskCtx, cancel := context.WithTimeout(WithBotKey(context.Background(), botKey), 20*time.Minute)
	go func() {
		defer cancel()
		result, err := service.SysLazysheepTggo().ClearBindingNotes(taskCtx, botKey, msg.Chat.ID)
		if err != nil {
			g.Log().Warningf(taskCtx, "Telegram clear task failed bot:%s chat:%d err:%+v", botKey, msg.Chat.ID, err)
			deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, fmt.Sprintf("清空失败：%v", err))
			return
		}
		pullResult, pullErr := service.SysLazysheepTggo().PullNow(taskCtx, &sysin.PullInp{
			BotKey: botKey,
			ChatID: msg.Chat.ID,
			Retry:  true,
		})
		if pullErr != nil {
			g.Log().Warningf(taskCtx, "Telegram clear repull failed bot:%s chat:%d err:%+v", botKey, msg.Chat.ID, pullErr)
			deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, result+"\n\n重新拉取失败："+pullErr.Error())
			return
		}
		deliverPullResult(taskCtx, b, msg.Chat.ID, progress, sendProgressErr, result+"\n\n已重新拉取：\n"+pullResult)
	}()
	return nil
}

func deliverPullResult(ctx context.Context, b *bot.Bot, chatID int64, progress *models.Message, progressErr error, text string) {
	sentResult := false
	text = truncateTelegramText(text)
	if strings.TrimSpace(text) != "" {
		if sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
		}); err == nil {
			sentResult = true
			deleteMessageLater(b, chatID, sent.ID)
		} else {
			g.Log().Warningf(ctx, "发送采集结果失败 chat:%d err:%+v", chatID, err)
		}
	}
	if sentResult && progressErr == nil && progress != nil {
		if _, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: progress.ID,
		}); err == nil {
			return
		}
	}
	if !sentResult && progressErr == nil && progress != nil {
		if _, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: progress.ID,
			Text:      text,
		}); err == nil {
			deleteMessageLater(b, chatID, progress.ID)
		}
	}
}

func truncateTelegramText(text string) string {
	text = strings.TrimSpace(text)
	const maxRunes = 3800
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "\n\n内容过长，已截断。完整错误请在后台日志查看。"
}

func pullProgressText(modeText string, limit int) string {
	if strings.TrimSpace(modeText) == "" {
		modeText = "采集"
	}
	if limit > 0 {
		return fmt.Sprintf("已启动%s，本次最多拉取 %d 条，请稍等。", modeText, limit)
	}
	return fmt.Sprintf("已启动%s，请稍等。", modeText)
}

func resolvePullModeText(ctx context.Context, botKey, sourceURL string, chatID int64) string {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil || state == nil {
		return "采集"
	}
	for _, item := range state.Bindings {
		if item == nil {
			continue
		}
		if item.BotKey != botKey {
			continue
		}
		if chatID != 0 && (item.ReviewChatID == chatID || item.PublishChatID == chatID) {
			if sourceURL != "" && item.SourceURL != sourceURL {
				return "采集"
			}
			if item.ReviewChatID != 0 {
				return "审核模式"
			}
			return "采集"
		}
	}
	return "采集"
}

type signCommand struct{}

func (h *signCommand) Key() string              { return "sign" }
func (h *signCommand) Pattern() string          { return "sign" }
func (h *signCommand) MatchType() bot.MatchType { return bot.MatchTypeCommandStartOnly }
func (h *signCommand) Description() string      { return "签到入口" }
func (h *signCommand) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "signin") {
		return nil
	}
	msg := messageFromUpdate(update)
	if msg == nil {
		return nil
	}
	return sendSignPrompt(ctx, b, msg.Chat.ID, func() int64 {
		if msg.From == nil {
			return 0
		}
		return msg.From.ID
	}(), currentBotKey(ctx))
}

type reviewApproveCallback struct{}

func (h *reviewApproveCallback) Key() string              { return "review_approve" }
func (h *reviewApproveCallback) Pattern() string          { return "review:approve" }
func (h *reviewApproveCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *reviewApproveCallback) Description() string      { return "审批通过" }
func (h *reviewApproveCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "review") {
		return nil
	}
	return replyCallback(ctx, b, update, "审批动作已接入，后续会落到审核流与频道推送。")
}

type reviewLocationCallback struct{}

func (h *reviewLocationCallback) Key() string              { return "review_location" }
func (h *reviewLocationCallback) Pattern() string          { return "review:location" }
func (h *reviewLocationCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *reviewLocationCallback) Description() string      { return "查看位置" }
func (h *reviewLocationCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "review") {
		return nil
	}
	return replyCallback(ctx, b, update, "位置查看动作已接入，后续会从消息笔记里提取 location。")
}

type reviewVerifyCallback struct{}

func (h *reviewVerifyCallback) Key() string              { return "review_verify" }
func (h *reviewVerifyCallback) Pattern() string          { return "review:verify" }
func (h *reviewVerifyCallback) MatchType() bot.MatchType { return bot.MatchTypeExact }
func (h *reviewVerifyCallback) Description() string      { return "查看验证" }
func (h *reviewVerifyCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if !pluginEnabled(ctx, "review") {
		return nil
	}
	return replyCallback(ctx, b, update, "验证查看动作已接入，后续会生成 start deep link。")
}

type collectorCallback struct{}

func (h *collectorCallback) Key() string              { return "collector" }
func (h *collectorCallback) Pattern() string          { return "collector:" }
func (h *collectorCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *collectorCallback) Description() string      { return "采集审核按钮" }
func (h *collectorCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	if !pluginEnabled(ctx, "collector") {
		return nil
	}
	data := update.CallbackQuery.Data
	switch {
	case strings.HasPrefix(data, "collector:publish:"):
		return replyCallback(ctx, b, update, "发布动作已收到。下一步会把该编号内容推送到绑定的公开频道。")
	case strings.HasPrefix(data, "collector:edit:"):
		return replyCallback(ctx, b, update, "编辑动作已收到。下一步会进入文案编辑会话。")
	case strings.HasPrefix(data, "collector:verify:"):
		return replyCallback(ctx, b, update, "验证视频已保存在笔记中，公开入口会通过私聊链接打开。")
	case strings.HasPrefix(data, "collector:location:"):
		return replyCallback(ctx, b, update, "位置已保存在笔记中，公开入口会通过私聊链接打开。")
	default:
		return replyCallback(ctx, b, update, "未知采集操作。")
	}
}

func replyCallback(ctx context.Context, b *bot.Bot, update *models.Update, text string) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	if CallbackAnswered(ctx) {
		return nil
	}
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            text,
		ShowAlert:       false,
	})
	return err
}

func userIDFromMessage(msg *models.Message) int64 {
	if msg == nil || msg.From == nil {
		return 0
	}
	return msg.From.ID
}

func settingsKeyword(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "设置" || text == "setting" || text == "settings" || text == "config" || text == "配置"
}

func pullRetryKeyword(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "重试" || text == "retry" || text == "force" || text == "重新拉取"
}

func ensureBotCreator(ctx context.Context, botKey string, userID int64) (bool, error) {
	if strings.TrimSpace(botKey) == "" || userID == 0 {
		return false, nil
	}
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return false, err
	}
	cfg := state.Bots[botKey]
	return cfg != nil && cfg.MemberId == userID, nil
}

func ensureBotCreatorForChat(ctx context.Context, botKey string, msg *models.Message) (bool, error) {
	if msg == nil {
		return false, nil
	}
	if userID := userIDFromMessage(msg); userID > 0 {
		return ensureBotCreator(ctx, botKey, userID)
	}
	if msg.SenderChat != nil && msg.SenderChat.ID == msg.Chat.ID {
		state, err := service.SysLazysheepTggo().GetState(ctx)
		if err != nil {
			return false, err
		}
		return findBindingByChat(state, botKey, msg.Chat.ID) != nil, nil
	}
	return false, nil
}

func sendPlainText(ctx context.Context, b *bot.Bot, chatID int64, text string) error {
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err == nil && sent != nil {
		deleteMessageLater(b, chatID, sent.ID)
	}
	return err
}

func deleteMessageLater(b *bot.Bot, chatID int64, messageID int) {
	if b == nil || chatID == 0 || messageID == 0 {
		return
	}
	go func() {
		time.Sleep(30 * time.Second)
		_, _ = b.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		})
	}()
}

func currentBotKey(ctx context.Context) string {
	return strings.TrimSpace(CurrentBotKey(ctx))
}

func pluginEnabled(ctx context.Context, key string) bool {
	plugins := currentBotPlugins(ctx)
	if len(plugins) == 0 {
		return false
	}
	cfg := plugins[key]
	return cfg != nil && cfg.Enabled
}

func messageFromUpdate(update *models.Update) *models.Message {
	if update == nil {
		return nil
	}
	if update.Message != nil {
		return update.Message
	}
	if update.ChannelPost != nil {
		return update.ChannelPost
	}
	if update.EditedChannelPost != nil {
		return update.EditedChannelPost
	}
	if update.EditedMessage != nil {
		return update.EditedMessage
	}
	return nil
}

func commandArgs(text string, commands ...string) string {
	text = strings.TrimSpace(text)
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		if strings.HasPrefix(text, command+" ") {
			return strings.TrimSpace(strings.TrimPrefix(text, command))
		}
		if text == command {
			return ""
		}
		if strings.HasPrefix(text, command+"@") {
			parts := strings.SplitN(text, " ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
			return ""
		}
	}
	return text
}
