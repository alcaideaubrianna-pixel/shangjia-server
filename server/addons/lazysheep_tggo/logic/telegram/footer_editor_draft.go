// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/lazysheep_tggo/service"
)

func openFooterDraftPanel(ctx context.Context, b *bot.Bot, session footerEditSession, messageID int) error {
	text := "页脚内容已录入，请选择保存范围。\n\n" + displayFooterText(session.Draft)
	keyboard := buildFooterDraftKeyboard(session)
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      session.ChatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard,
	})
	return err
}

func sendFooterDraftPanel(ctx context.Context, b *bot.Bot, session footerEditSession) error {
	text := "页脚内容已录入，请选择保存范围。\n\n" + displayFooterText(session.Draft)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      session.ChatID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: buildFooterDraftKeyboard(session),
	})
	return err
}

func buildFooterDraftKeyboard(session footerEditSession) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, 3)
	if session.Scope == footerEditScopeBinding {
		rows = append(rows, []models.InlineKeyboardButton{{Text: "仅当前频道", CallbackData: "footer:save:binding"}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "修改全局", CallbackData: "footer:save:global"}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回", CallbackData: "footer:draft_back"}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "取消", CallbackData: "footer:cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func saveFooterDraft(ctx context.Context, b *bot.Bot, update *models.Update, scope footerEditScope) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	opCtx := context.WithoutCancel(ctx)
	chatID := callbackChatID(update.CallbackQuery)
	userID := update.CallbackQuery.From.ID
	session, ok := getFooterEditSession(chatID, userID)
	if !ok || strings.TrimSpace(session.Draft) == "" {
		return replyCallback(ctx, b, update, "请先输入页脚内容。")
	}
	state, err := service.SysLazysheepTggo().GetState(opCtx)
	if err != nil {
		return err
	}
	switch scope {
	case footerEditScopeGlobal:
		err = updateGlobalFooter(state, session.Draft)
	case footerEditScopeBinding:
		err = updateBindingFooter(state, session.BotKey, chatID, session.Draft)
	default:
		err = fmt.Errorf("未知保存范围")
	}
	if err != nil {
		return replyCallback(ctx, b, update, fmt.Sprintf("保存页脚失败：%v", err))
	}
	if err = service.SysLazysheepTggo().SaveState(opCtx, state); err != nil {
		return err
	}
	clearFooterEditSession(chatID, userID)
	if err = openFooterEditorPanel(opCtx, b, session.BotKey, chatID, userID, callbackMessageID(update.CallbackQuery)); err != nil {
		return err
	}
	return replyCallback(opCtx, b, update, "页脚已更新。")
}
