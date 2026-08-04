package sys

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

var botListenerBindCodeRegexp = regexp.MustCompile(`(?i)\bOX[A-Z0-9]{6}\b`)

type botMessageEvent struct {
	BotId int64
	Msg   *models.Message
	Text  string
}

type botMessageHandler interface {
	Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (handled bool, err error)
}

var botMessageHandlers = []botMessageHandler{
	quickPushSessionMessageHandler{},
	botFeatureMessageHandler{},
	publishListenerBindMessageHandler{},
	profileManageMessageHandler{},
	scanMediaMessageHandler{},
	authCodeMessageHandler{},
}

func botListenerBindCode(text string) string {
	return strings.ToUpper(botListenerBindCodeRegexp.FindString(text))
}

func botAllowedUpdates() []string {
	return []string{
		models.AllowedUpdateMessage,
		models.AllowedUpdateEditedMessage,
		models.AllowedUpdateChannelPost,
		models.AllowedUpdateEditedChannelPost,
		models.AllowedUpdateCallbackQuery,
		models.AllowedUpdateInlineQuery,
	}
}

func botMessageFromUpdate(update *models.Update) *models.Message {
	if update == nil {
		return nil
	}
	if update.Message != nil {
		return update.Message
	}
	if update.EditedMessage != nil {
		return update.EditedMessage
	}
	if update.ChannelPost != nil {
		return update.ChannelPost
	}
	return update.EditedChannelPost
}

func (s *sSysBot) dispatchBotMessage(ctx context.Context, event *botMessageEvent) (bool, error) {
	if event == nil || event.Msg == nil {
		return false, nil
	}
	for _, handler := range botMessageHandlers {
		handled, err := handler.Handle(ctx, s, event)
		if handled || err != nil {
			return handled, err
		}
	}
	return false, nil
}

type botFeatureMessageHandler struct{}

func (botFeatureMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (bool, error) {
	return bot.dispatchFeature(ctx, event.BotId, event.Msg, event.Text)
}

type publishListenerBindMessageHandler struct{}

func (publishListenerBindMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (handled bool, err error) {
	code := botListenerBindCode(event.Text)
	if code == "" {
		return false, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Warningf(ctx, "消息监听绑定处理器不可用 err:%v", recovered)
			handled = false
			err = nil
		}
	}()
	return publishService.SysPublish().HandleBotMessage(ctx, &publishsysin.BotMessageInp{
		BotId:     event.BotId,
		ChatId:    fmt.Sprintf("%d", event.Msg.Chat.ID),
		ChatTitle: botMessageChatTitle(event.Msg),
		ChatType:  string(event.Msg.Chat.Type),
		Text:      code,
	})
}

type authCodeMessageHandler struct{}

func (authCodeMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (bool, error) {
	if event.Msg.From == nil {
		return false, nil
	}
	code := sixDigitRegexp.FindString(event.Text)
	if code == "" {
		return false, nil
	}
	return true, bot.consumeCode(ctx, event.BotId, event.Msg, code)
}

func botMessageChatTitle(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	return firstNonEmpty(
		msg.Chat.Title,
		strings.TrimSpace(msg.Chat.FirstName+" "+msg.Chat.LastName),
		msg.Chat.Username,
		fmt.Sprintf("%d", msg.Chat.ID),
	)
}
