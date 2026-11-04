package sys

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

const gatewaySlowStageLogThreshold = 100 * time.Millisecond

func logGatewaySlowStage(ctx context.Context, updateID int64, botKey, kind, stage, name string, referenceID int64, duration time.Duration, handled *bool, err error) {
	if duration < gatewaySlowStageLogThreshold && err == nil {
		return
	}
	handledValue := "-"
	if handled != nil {
		handledValue = "false"
		if *handled {
			handledValue = "true"
		}
	}
	result := "ok"
	if err != nil {
		result = "error"
	}
	if err != nil {
		g.Log().Warningf(ctx, "TG链路 gateway_dispatch_slow_stage botKey:%s updateId:%d kind:%s stage:%s name:%s reference:%d duration:%s handled:%s result:%s errorType:%T", botKey, updateID, kind, stage, name, referenceID, duration, handledValue, result, err)
		return
	}
	g.Log().Infof(ctx, "TG链路 gateway_dispatch_slow_stage botKey:%s updateId:%d kind:%s stage:%s name:%s reference:%d duration:%s handled:%s result:%s", botKey, updateID, kind, stage, name, referenceID, duration, handledValue, result)
}

func gatewayUpdateKind(update *models.Update) string {
	if update == nil {
		return "nil"
	}
	switch {
	case update.Message != nil:
		return "message"
	case update.EditedMessage != nil:
		return "edited_message"
	case update.ChannelPost != nil:
		return "channel_post"
	case update.EditedChannelPost != nil:
		return "edited_channel_post"
	case update.CallbackQuery != nil:
		return "callback"
	case update.InlineQuery != nil:
		return "inline"
	case update.MessageReaction != nil:
		return "reaction"
	case update.MyChatMember != nil:
		return "my_chat_member"
	default:
		return "other"
	}
}

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
