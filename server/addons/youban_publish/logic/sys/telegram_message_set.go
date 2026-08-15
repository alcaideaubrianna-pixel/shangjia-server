package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const telegramDeleteMessagesMaxItems = 100

func (s *sSysPublish) deleteTelegramMessageSet(ctx context.Context, job telegramJobRecord, reason string) error {
	return s.withTelegramChannelLock(ctx, job.TargetChatId, func() error {
		return s.deleteTelegramMessageSetLockedByChannel(ctx, job, reason)
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
	batches := telegramDeleteMessageBatches(messages, telegramDeleteMessagesMaxItems)
	for _, batch := range batches {
		messageIds := make([]int, 0, len(batch.messages))
		for _, item := range batch.messages {
			messageIds = append(messageIds, int(item.MessageId))
		}
		deleted, batchErr := bot.DeleteMessages(ctx, &tgbot.DeleteMessagesParams{ChatID: batch.chatId, MessageIDs: messageIds})
		if batchErr == nil && deleted {
			if err = markTelegramMessagesDeleted(ctx, batch.messages); err != nil {
				return err
			}
			continue
		}
		if batchErr == nil {
			batchErr = gerror.New("Telegram批量删除返回失败")
		}
		if isTelegramBotRemovedError(batchErr) {
			if err = markTelegramMessagesUndeletable(ctx, batch.messages); err != nil {
				return err
			}
			s.enqueueTelegramMessageDeleteFallback(ctx, job, reason, batchErr)
			continue
		}
		if isTelegramMessagePermanentlyUndeletableError(batchErr) {
			if err = markTelegramMessagesUndeletable(ctx, batch.messages); err != nil {
				return err
			}
			s.enqueueTelegramMessageDeleteFallback(ctx, job, reason, batchErr)
			continue
		}
		g.Log().Debugf(ctx, "批量删除TG消息失败，回退逐条删除 job:%d chat:%s count:%d err:%+v", job.Id, batch.chatId, len(batch.messages), batchErr)
		if err = s.deleteTelegramMessagesIndividually(ctx, bot, job, batch.messages, reason); err != nil {
			return err
		}
	}
	g.Log().Infof(ctx, "TG消息删除完成 job:%d messages:%d batches:%d duration:%s", job.Id, len(messages), len(batches), time.Since(startedAt).Round(time.Millisecond))
	s.appendTelegramJobLog(ctx, job, "delete", "success", reason+"，TG消息删除成功")
	return nil
}

func (s *sSysPublish) deleteTelegramMessagesIndividually(ctx context.Context, bot *tgbot.Bot, job telegramJobRecord, messages []telegramDeleteMessage, reason string) error {
	fallbackQueued := false
	for _, item := range messages {
		chatId := normalizeTelegramChannelChatID(item.TargetChatId)
		_, err := bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: int(item.MessageId)})
		if err != nil {
			if isTelegramMessageAlreadyDeletedError(err) {
				_ = markTelegramMessagesDeleted(ctx, []telegramDeleteMessage{item})
				s.appendTelegramJobLog(ctx, job, "delete", "skipped", fmt.Sprintf("%s，TG消息已不存在，已同步本地删除状态，频道:%s，消息:%d", reason, chatId, item.MessageId))
				continue
			}
			if isTelegramMessagePermanentlyUndeletableError(err) {
				if markErr := markTelegramMessagesUndeletable(ctx, []telegramDeleteMessage{item}); markErr != nil {
					return markErr
				}
				if !fallbackQueued {
					s.enqueueTelegramMessageDeleteFallback(ctx, job, reason, err)
					fallbackQueued = true
				}
				continue
			}
			if isTelegramBotRemovedError(err) {
				if markErr := markTelegramMessagesUndeletable(ctx, []telegramDeleteMessage{item}); markErr != nil {
					return markErr
				}
				if !fallbackQueued {
					s.enqueueTelegramMessageDeleteFallback(ctx, job, reason, err)
					fallbackQueued = true
				}
				continue
			}
			message := fmt.Sprintf("%s，删除TG消息失败，频道:%s，消息:%d，错误:%s", reason, chatId, item.MessageId, err.Error())
			s.appendTelegramJobLog(ctx, job, "delete", "failed", message)
			return gerror.New(message)
		}
		if err = markTelegramMessagesDeleted(ctx, []telegramDeleteMessage{item}); err != nil {
			return err
		}
	}
	return nil
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
