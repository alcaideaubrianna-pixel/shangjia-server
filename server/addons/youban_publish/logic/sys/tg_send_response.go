package sys

import (
	"context"
	"hotgo/utility/telegrambudget"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// awaitTelegramUnknownResponse saves partial evidence and the reconciliation
// intent within one finalization deadline, independently of the failed send.
func (s *sSysPublish) awaitTelegramUnknownResponse(ctx context.Context, job telegramJobRecord, attempt telegramDeliveryAttempt, messages []*telegramSentMessage, cause error) error {
	ctx = telegramPublishJobTraceContext(ctx, job)
	ctx, cancel := telegramFinalizeContext(ctx, "response.await_confirmation")
	defer cancel()
	if err := persistTelegramAttemptResponseDurably(ctx, attempt, messages); err != nil {
		return telegramDeliveryUncertainError(err)
	}
	if len(messages) > 0 {
		if err := s.saveTelegramSentMessages(ctx, job, messages); err != nil {
			return telegramDeliveryUncertainError(err)
		}
	}
	if err := s.waitTelegramDeliveryWebhook(ctx, attempt, cause); err != nil {
		return gerror.Wrap(err, "保存Telegram未知响应待确认状态失败")
	}
	g.Log().Info(ctx, "TG未知响应已保存，等待消息事实确认", g.Map{
		"event": "telegram_response_pending", "jobId": job.Id,
		"profileId": job.ProfileId, "channelId": job.ChannelId,
		"targetChatId": job.TargetChatId, "operationNo": job.OperationNo,
		"attemptId": attempt.Id, "purpose": attempt.Phase,
		"transport": "bot", "messageIds": telegramSentMessageIDs(messages),
		"reason": "ambiguous_response", "verdict": "awaiting_confirmation",
	})
	return errTelegramAwaitingWebhook
}

// Reusable IDs are an optional projection of an already durable response.
// Cache failures must not turn a successful Telegram delivery into a resend.
func (s *sSysPublish) cacheTelegramReturnedFileIDs(ctx context.Context, job telegramJobRecord, purpose string, messages []*telegramSentMessage) {
	if len(messages) == 0 {
		return
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	// Shutdown may decline optional work. The authoritative response has already
	// been persisted synchronously by confirmTelegramAPISuccess.
	s.runTelegramFinalizeAsync(ctx, "response.cache_file_ids", func(cacheCtx context.Context) error {
		cacheCtx, cancel := telegramFinalizeContextWithTimeout(cacheCtx, "response.cache_file_ids", telegrambudget.Response(cacheCtx))
		defer cancel()
		err := s.updateTelegramMediaFileIds(cacheCtx, messages)
		if err != nil {
			g.Log().Warning(cacheCtx, "TG发送成功，file_id缓存等待后续恢复", g.Map{
				"event": "telegram_file_id_cache_failed", "jobId": job.Id,
				"profileId": job.ProfileId, "channelId": job.ChannelId,
				"targetChatId": job.TargetChatId, "purpose": purpose,
				"messageIds": telegramSentMessageIDs(messages),
				"reason":     "optional_projection_failed",
				"error":      telegramPublishLogError(err),
			})
		}
		return err
	})
}
