package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type accountMenuContextKey struct{}
type accountMenuSavedKey struct{}
type accountMenuContext struct {
	MessageID int
	Page      int
}

func accountMenuPage(ctx context.Context) int {
	state, _ := ctx.Value(accountMenuContextKey{}).(accountMenuContext)
	if state.Page < 1 {
		return 1
	}
	return state.Page
}
func accountMenuWithPage(ctx context.Context, page int) context.Context {
	state, _ := ctx.Value(accountMenuContextKey{}).(accountMenuContext)
	state.Page = page
	return context.WithValue(ctx, accountMenuContextKey{}, state)
}

// Keep page context in the buttons, so navigation also works across instances.
func accountMenuMarkup(ctx context.Context, markup models.ReplyMarkup) models.ReplyMarkup {
	keyboard, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok || keyboard == nil {
		return markup
	}
	copyKeyboard := &models.InlineKeyboardMarkup{}
	for _, row := range keyboard.InlineKeyboard {
		buttons := append([]models.InlineKeyboardButton(nil), row...)
		for i := range buttons {
			data := buttons[i].CallbackData
			if strings.HasPrefix(data, "acct:") && !strings.Contains(data, "~p") && accountMenuPage(ctx) > 1 {
				buttons[i].CallbackData = fmt.Sprintf("%s~p%d", data, accountMenuPage(ctx))
			}
		}
		copyKeyboard.InlineKeyboard = append(copyKeyboard.InlineKeyboard, buttons)
	}
	return copyKeyboard
}
func (s *sSysBot) accountMenuReply(ctx context.Context, botID int64, chatID, text string, markup models.ReplyMarkup) error {
	if saved, _ := ctx.Value(accountMenuSavedKey{}).(bool); saved {
		text = "✅ 已保存\n\n" + text
	}
	markup = accountMenuMarkup(ctx, markup)
	state, _ := ctx.Value(accountMenuContextKey{}).(accountMenuContext)
	if state.MessageID <= 0 {
		return s.replyWithMarkup(ctx, botID, chatID, text, markup)
	}
	keyboard, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		return s.replyWithMarkup(ctx, botID, chatID, text, markup)
	}
	// Reuse the existing menu transport. Timeouts remain errors; never blindly
	// send a second message when the edit result is unknown.
	userID, err := parseAccountMenuChat(chatID)
	if err != nil {
		return err
	}
	_, err = s.managerSendMessage(ctx, botID, userID, state.MessageID, text, keyboard.InlineKeyboard)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "message to edit not found") || strings.Contains(message, "message can't be edited") {
			return s.replyWithMarkup(ctx, botID, chatID, text, markup)
		}
	}
	return err
}
func parseAccountMenuChat(chatID string) (int64, error) {
	return strconv.ParseInt(chatID, 10, 64)
}
func (s *sSysBot) cancelAccountInput(ctx context.Context, botID int64, userID, chatID string) error {
	_, err := g.DB().Model(profileSessionTable).Ctx(ctx).
		Where("bot_id", botID).Where("telegram_user_id", userID).Where("chat_id", chatID).
		Where("status", profileSessionStatusActive).
		WhereIn("scene", []string{"account_create", "account_setting", "account_renewal"}).
		Data(g.Map{"status": profileSessionStatusCanceled}).Update()
	if err != nil {
		return gerror.Wrap(err, "退出账户输入失败，请重试")
	}
	return nil
}

func (s *sSysBot) cancelAccountInputForNavigation(ctx context.Context, botID int64, userID, chatID, data string) error {
	if strings.HasPrefix(data, "acct:renew_confirm:") {
		return nil
	}
	return s.cancelAccountInput(ctx, botID, userID, chatID)
}

func (s *sSysBot) cancelAccountSessions(ctx context.Context, botID int64, userID, chatID string) error {
	_, err := g.DB().Model(profileSessionTable).Ctx(ctx).
		Where("bot_id", botID).Where("telegram_user_id", userID).Where("chat_id", chatID).
		Where("status", profileSessionStatusActive).
		WhereIn("scene", []string{"account_create", "account_setting", "account_renewal", "account_delete", "account_reset", "account_managed_reset"}).
		Data(g.Map{"status": profileSessionStatusCanceled}).Update()
	if err != nil {
		return gerror.Wrap(err, "取消账户操作失败，请重试")
	}
	return nil
}
