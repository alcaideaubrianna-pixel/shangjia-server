package sys

import (
	"context"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

func logGatewayUpdate(ctx context.Context, update *models.Update, bindings int) {
	if update == nil {
		return
	}
	msg, kind := update.Message, "message"
	if msg == nil {
		msg, kind = update.ChannelPost, "channel_post"
	}
	if msg == nil {
		msg, kind = update.EditedMessage, "edited_message"
	}
	if msg == nil {
		msg, kind = update.EditedChannelPost, "edited_channel_post"
	}
	if msg == nil {
		g.Log().Infof(ctx, "TG链路 gateway_dispatch updateId:%d kind:non_message reaction:%t callback:%t inline:%t bindings:%d", update.ID, update.MessageReaction != nil, update.CallbackQuery != nil, update.InlineQuery != nil, bindings)
		return
	}
	g.Log().Infof(ctx, "TG链路 gateway_dispatch updateId:%d kind:%s chatId:%d topicId:%d messageId:%d photo:%d video:%t document:%t sticker:%t animation:%t bindings:%d", update.ID, kind, msg.Chat.ID, msg.MessageThreadID, msg.ID, len(msg.Photo), msg.Video != nil, msg.Document != nil, msg.Sticker != nil, msg.Animation != nil, bindings)
}
