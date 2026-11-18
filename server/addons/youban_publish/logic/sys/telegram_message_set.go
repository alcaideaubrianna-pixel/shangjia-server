package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const telegramDeleteMessagesMaxItems = 100
const telegramDeleteMessagesBatchThreshold = 10

// Keep per-job Telegram delete requests bounded to avoid triggering channel/API rate limits.
const telegramDeleteMessagesConcurrency = 3

func (s *sSysPublish) deleteTelegramMessageSet(ctx context.Context, job telegramJobRecord, reason string) error {
	return s.withTelegramCleanupExecution(ctx, job, func(cleanupCtx context.Context) error {
		return s.deleteTelegramMessageSetLockedByChannel(cleanupCtx, job, reason)
	})
}

func (s *sSysPublish) deleteTelegramMessageSetLockedByChannel(ctx context.Context, job telegramJobRecord, reason string) error {
	messages, err := s.telegramJobActiveMessages(ctx, job)
	if err != nil {
		return err
	}
	return s.deleteTelegramMessagesLockedByChannel(ctx, job, messages, reason)
}

func (s *sSysPublish) deleteTelegramMessagePurposeSetLockedByChannel(ctx context.Context, job telegramJobRecord, purpose string, reason string) error {
	messages, err := s.telegramJobActiveMessages(ctx, job, purpose)
	if err != nil {
		return err
	}
	return s.deleteTelegramMessagesLockedByChannel(ctx, job, messages, reason)
}

func (s *sSysPublish) deleteTelegramMessagesLockedByChannel(ctx context.Context, job telegramJobRecord, messages []telegramDeleteMessage, reason string) error {
	if len(messages) == 0 {
		s.appendTelegramJobLog(ctx, job, "delete", "skipped", reason+"，未找到可删除的TG消息")
		return nil
	}
	botToken, err := s.telegramCleanupJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		if isTelegramBotConfigMissingError(err) {
			if markErr := markTelegramMessagesUndeletable(ctx, messages); markErr != nil {
				return markErr
			}
			s.enqueueTelegramMessageDeleteFallback(ctx, job, reason, err)
			return nil
		}
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "delete", "started", reason+"，开始删除TG消息")
	startedAt := time.Now()
	// Always use the Bot API's per-message DeleteMessage call here. The
	// custom deleteMessages endpoint can report success without proving that
	// Telegram removed the messages, which leaves local rows marked deleted
	// while the channel messages remain visible. The individual path also
	// preserves the existing permission/error fallback handling.
	if err = s.deleteTelegramMessagesIndividually(ctx, bot, job, messages, reason); err != nil {
		return err
	}
	g.Log().Infof(ctx, "TG消息逐条删除完成 job:%d messages:%d duration:%s", job.Id, len(messages), time.Since(startedAt).Round(time.Millisecond))
	s.appendTelegramJobLog(ctx, job, "delete", "success", reason+"，TG消息删除成功")
	return nil
}

func (s *sSysPublish) deleteTelegramMessagesIndividually(ctx context.Context, bot *tgbot.Bot, job telegramJobRecord, messages []telegramDeleteMessage, reason string) error {
	if len(messages) == 0 {
		return nil
	}
	deleteErrors, rateErr, rateDelay := telegramDeleteMessageRequests(ctx, bot, messages)

	// Persist all confirmed deletions before inspecting failures: an earlier
	// non-retryable error must not discard a later in-flight success.
	confirmed := make([]telegramDeleteMessage, 0, len(messages))
	for index, item := range messages {
		if deleteErrors[index] == nil || isTelegramMessageAlreadyDeletedError(deleteErrors[index]) {
			confirmed = append(confirmed, item)
		}
	}
	if err := markTelegramMessagesDeleted(ctx, confirmed); err != nil {
		return err
	}

	fallbackQueued := false
	var retryErrors []string
	for index, item := range messages {
		chatID := normalizeTelegramChannelChatID(item.TargetChatId)
		err := deleteErrors[index]
		if err != nil {
			if _, limited := telegramDeleteRateDelay(err); limited {
				continue
			}
			if isTelegramMessageAlreadyDeletedError(err) {
				s.appendTelegramJobLog(ctx, job, "delete", "skipped", fmt.Sprintf("%s，TG消息已不存在，已同步本地删除状态，频道:%s，消息:%d", reason, chatID, item.MessageId))
				continue
			}
			if isTelegramMessagePermanentlyUndeletableError(err) || isTelegramBotRemovedError(err) {
				if markErr := markTelegramMessagesUndeletable(ctx, []telegramDeleteMessage{item}); markErr != nil {
					return markErr
				}
				if !fallbackQueued {
					s.enqueueTelegramMessageDeleteFallback(ctx, job, reason, err)
					fallbackQueued = true
				}
				continue
			}
			message := fmt.Sprintf("%s，删除TG消息失败，频道:%s，消息:%d，错误:%s", reason, chatID, item.MessageId, err.Error())
			s.appendTelegramJobLog(ctx, job, "delete", "failed", message)
			if isTelegramDeleteRetryableError(err) {
				retryErrors = append(retryErrors, message)
				continue
			}
			return gerror.New(message)
		}
	}
	if rateErr != nil {
		return &tgRetryAfterError{after: rateDelay, err: rateErr}
	}
	if len(retryErrors) > 0 {
		return gerror.New(strings.Join(retryErrors, "; "))
	}
	return nil
}

func telegramDeleteMessageRequests(ctx context.Context, bot *tgbot.Bot, messages []telegramDeleteMessage) ([]error, error, time.Duration) {
	workerCount := telegramDeleteMessagesConcurrency
	if len(messages) < workerCount {
		workerCount = len(messages)
	}

	type deleteWork struct {
		index int
		item  telegramDeleteMessage
	}
	jobs := make(chan deleteWork)
	deleteErrors := make([]error, len(messages))
	var wg sync.WaitGroup
	var rateMu sync.Mutex
	var rateErr error
	var rateDelay time.Duration
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range jobs {
				rateMu.Lock()
				stopped := rateErr
				rateMu.Unlock()
				if stopped != nil {
					deleteErrors[work.index] = stopped
					continue
				}
				chatID := normalizeTelegramChannelChatID(work.item.TargetChatId)
				requestStarted := time.Now()
				deleted, err := bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatID, MessageID: int(work.item.MessageId)})
				if err == nil && !deleted {
					err = gerror.New("TG未确认消息删除成功")
				}
				publishCleanupObservability.messageRequest(ctx, deleted, time.Since(requestStarted), err)
				deleteErrors[work.index] = err
				if delay, limited := telegramDeleteRateDelay(err); limited {
					rateMu.Lock()
					if rateErr == nil || delay > rateDelay {
						rateErr, rateDelay = err, delay
					}
					rateMu.Unlock()
				}
			}
		}()
	}
	for index, item := range messages {
		jobs <- deleteWork{index: index, item: item}
	}
	close(jobs)
	wg.Wait()
	return deleteErrors, rateErr, rateDelay
}

func telegramDeleteRateDelay(err error) (time.Duration, bool) {
	var rate *tgbot.TooManyRequestsError
	if errors.As(err, &rate) {
		if rate == nil {
			return 0, false
		}
		if rate.RetryAfter <= 0 {
			return telegramRetryMinDelay, true
		}
		seconds := int64(rate.RetryAfter)
		maxSeconds := int64((time.Duration(1<<63-1) - time.Minute) / time.Second)
		if seconds > maxSeconds {
			seconds = maxSeconds
		}
		return time.Duration(seconds) * time.Second, true
	}
	return telegramRateLimitRetryDelay(err)
}

type telegramDeleteMessageBatch struct {
	chatId   string
	messages []telegramDeleteMessage
}

func telegramDeleteMessageBatches(messages []telegramDeleteMessage, maxItems int) []telegramDeleteMessageBatch {
	if maxItems <= 0 {
		maxItems = telegramDeleteMessagesMaxItems
	}
	batches := make([]telegramDeleteMessageBatch, 0)
	batchIndexes := make(map[string]int)
	for _, item := range messages {
		chatId := normalizeTelegramChannelChatID(item.TargetChatId)
		if item.Id <= 0 || item.MessageId <= 0 || chatId == "" {
			continue
		}
		index, exists := batchIndexes[chatId]
		if !exists || len(batches[index].messages) >= maxItems {
			batches = append(batches, telegramDeleteMessageBatch{chatId: chatId, messages: make([]telegramDeleteMessage, 0, maxItems)})
			index = len(batches) - 1
			batchIndexes[chatId] = index
		}
		batches[index].messages = append(batches[index].messages, item)
	}
	return batches
}

func markTelegramMessagesDeleted(ctx context.Context, messages []telegramDeleteMessage) error {
	return markTelegramMessagesStatus(ctx, messages, "deleted")
}

func markTelegramMessagesUndeletable(ctx context.Context, messages []telegramDeleteMessage) error {
	return markTelegramMessagesStatus(ctx, messages, "undeletable")
}

func markTelegramMessagesStatus(ctx context.Context, messages []telegramDeleteMessage, status string) error {
	ids := make([]int64, 0, len(messages))
	for _, item := range messages {
		if item.Id > 0 {
			ids = append(ids, item.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return gerror.New("TG消息状态不能为空")
	}
	data := g.Map{"status": status, "updated_at": gtime.Now()}
	if status == "deleted" {
		data["deleted_at"] = gtime.Now()
	} else {
		data["deleted_at"] = nil
	}
	_, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Data(data).
		Update()
	return gerror.Wrap(err, "更新TG消息删除状态失败")
}

func isTelegramMessagePermanentlyUndeletableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "message can't be deleted") ||
		strings.Contains(message, "message can’t be deleted")
}

func isTelegramBotConfigMissingError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Bot配置不存在")
}

func isTelegramBotRemovedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bot was kicked") ||
		strings.Contains(message, "bot was blocked") ||
		strings.Contains(message, "chat not found")
}

func isTelegramMessageAlreadyDeletedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	messageMissing := strings.Contains(message, "message to delete not found") ||
		strings.Contains(message, "message not found")
	if !messageMissing {
		return false
	}
	return errors.Is(err, tgbot.ErrorBadRequest) ||
		errors.Is(err, tgbot.ErrorNotFound) ||
		strings.Contains(message, "bad request") ||
		strings.Contains(message, "not found")
}

func isTelegramDeleteRetryableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, keyword := range []string{
		"too many requests", "retry after", "timeout", "deadline exceeded",
		"goaway", "graceful shutdown", "connection reset", "connection refused",
		"broken pipe", "bad gateway", "eof",
	} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}
