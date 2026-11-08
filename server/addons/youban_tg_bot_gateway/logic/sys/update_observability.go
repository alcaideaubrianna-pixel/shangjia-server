package sys

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const gatewaySlowStageLogThreshold = 100 * time.Millisecond

func observeGatewayCallback(ctx context.Context, result string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("result", result))
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.bot.callback.requests")
	histogram, _ := gatewayObserveMeter.Float64Histogram("xiaohuiji.tg.bot.callback.duration_seconds")
	counter.Add(ctx, 1, attrs)
	histogram.Record(ctx, duration.Seconds(), attrs)
}

func observeGatewayWebhook(ctx context.Context, result string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("result", result))
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.bot.webhook.requests")
	histogram, _ := gatewayObserveMeter.Float64Histogram("xiaohuiji.tg.bot.webhook.duration_seconds")
	counter.Add(ctx, 1, attrs)
	histogram.Record(ctx, duration.Seconds(), attrs)
}

func observeGatewayQueue(ctx context.Context, queue, result string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("queue", queue), attribute.String("result", result))
	histogram, _ := gatewayObserveMeter.Float64Histogram("xiaohuiji.tg.bot.queue.wait_seconds")
	histogram.Record(ctx, duration.Seconds(), attrs)
}

func observeGatewayDispatch(ctx context.Context, kind, result string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("update_kind", kind), attribute.String("result", result))
	histogram, _ := gatewayObserveMeter.Float64Histogram("xiaohuiji.tg.bot.dispatch.duration_seconds")
	histogram.Record(ctx, duration.Seconds(), attrs)
}

func observeGatewayQueueTimeout(ctx context.Context, queue string) {
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.bot.queue.timeouts")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("queue", queue)))
}

func observeGatewayTelegramError(ctx context.Context, apiMethod string, err error) {
	result := "error"
	message := strings.ToLower(errString(err))
	if strings.Contains(message, "429") || strings.Contains(message, "too many requests") {
		result = "rate_limited"
	} else if strings.Contains(message, "500") || strings.Contains(message, "502") || strings.Contains(message, "503") || strings.Contains(message, "504") {
		result = "5xx"
	}
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.bot.telegram_api.errors")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("api_method", apiMethod), attribute.String("result", result)))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

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
