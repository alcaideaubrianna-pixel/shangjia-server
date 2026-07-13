package sys

import (
	"context"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_bot/model/input/sysin"
)

func (s *sSysBot) NotifyRich(ctx context.Context, in *sysin.NotifyRichInp) error {
	if in == nil {
		return gerror.New("消息内容不能为空")
	}
	if strings.TrimSpace(in.ChatId) == "" {
		return gerror.New("目标Chat ID不能为空")
	}
	if strings.TrimSpace(in.Text) == "" && !in.SourceHasMedia {
		return gerror.New("消息内容不能为空")
	}
	botToken := ""
	if in.BotId > 0 {
		row, err := s.botById(ctx, in.BotId)
		if err != nil {
			return err
		}
		botToken = row.BotToken
	} else {
		row, err := s.officialBot(ctx)
		if err != nil {
			return err
		}
		botToken = row.BotToken
	}
	replyMarkup := buildNotifyInlineKeyboard(in.ButtonLabel, in.ButtonURL)
	sourceMessageIds := normalizeNotifySourceMessageIds(in.SourceMessageId, in.SourceMessageIds)
	if in.SourceHasMedia && strings.TrimSpace(in.SourceChatId) != "" && len(sourceMessageIds) > 1 {
		if err := s.copyMessagesWithFollowup(ctx, botToken, in, sourceMessageIds, replyMarkup); err == nil {
			return nil
		} else if strings.TrimSpace(in.Text) == "" {
			return err
		}
	}
	if in.SourceHasMedia && strings.TrimSpace(in.SourceChatId) != "" && in.SourceMessageId > 0 {
		if err := s.copyMessageWithMarkup(ctx, botToken, in, replyMarkup); err == nil {
			return nil
		} else if in.SourceHasMedia && strings.TrimSpace(in.Text) == "" {
			return err
		}
	}
	if replyMarkup != nil {
		_, err := s.sendMessageWithMarkup(ctx, botToken, in.ChatId, in.Text, firstNonEmpty(in.ParseMode, "HTML"), in.DisableNotice, replyMarkup)
		if err != nil && in.BotId > 0 && shouldMarkBotOffline(err) {
			_ = s.markBotOffline(ctx, in.BotId, err)
		}
		return err
	}
	_, err := s.sendMessage(ctx, botToken, in.ChatId, in.Text, firstNonEmpty(in.ParseMode, "HTML"), in.DisableNotice)
	if err != nil && in.BotId > 0 && shouldMarkBotOffline(err) {
		_ = s.markBotOffline(ctx, in.BotId, err)
	}
	return err
}

func (s *sSysBot) copyMessagesWithFollowup(ctx context.Context, botToken string, in *sysin.NotifyRichInp, sourceMessageIds []int, replyMarkup models.ReplyMarkup) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, err = bot.CopyMessages(callCtx, &tgbot.CopyMessagesParams{
		ChatID:              in.ChatId,
		FromChatID:          in.SourceChatId,
		MessageIDs:          sourceMessageIds,
		DisableNotification: in.DisableNotice,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil
	}
	if replyMarkup != nil {
		_, err = s.sendMessageWithMarkup(ctx, botToken, in.ChatId, in.Text, firstNonEmpty(in.ParseMode, "HTML"), in.DisableNotice, replyMarkup)
		return err
	}
	_, err = s.sendMessage(ctx, botToken, in.ChatId, in.Text, firstNonEmpty(in.ParseMode, "HTML"), in.DisableNotice)
	return err
}

func (s *sSysBot) copyMessageWithMarkup(ctx context.Context, botToken string, in *sysin.NotifyRichInp, replyMarkup models.ReplyMarkup) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	params := &tgbot.CopyMessageParams{
		ChatID:              in.ChatId,
		FromChatID:          in.SourceChatId,
		MessageID:           in.SourceMessageId,
		DisableNotification: in.DisableNotice,
		ReplyMarkup:         replyMarkup,
	}
	if strings.TrimSpace(in.Text) != "" {
		params.Caption = in.Text
		params.ParseMode = models.ParseMode(firstNonEmpty(in.ParseMode, "HTML"))
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, err = bot.CopyMessage(callCtx, params)
	return err
}

func buildNotifyInlineKeyboard(label, url string) *models.InlineKeyboardMarkup {
	label = strings.TrimSpace(label)
	url = strings.TrimSpace(url)
	if label == "" || url == "" {
		return nil
	}
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: label, URL: url}},
		},
	}
}

func normalizeNotifySourceMessageIds(sourceMessageId int, sourceMessageIds []int) []int {
	seen := make(map[int]struct{}, len(sourceMessageIds)+1)
	out := make([]int, 0, len(sourceMessageIds)+1)
	for _, id := range sourceMessageIds {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if sourceMessageId > 0 {
		if _, ok := seen[sourceMessageId]; !ok {
			out = append(out, sourceMessageId)
		}
	}
	return out
}
