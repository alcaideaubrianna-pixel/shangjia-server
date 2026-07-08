package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) SendTelegramJob(ctx context.Context, jobId int64) error {
	targetJob, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	lease, ok, err := s.tryTelegramChannelLease(ctx, targetJob.TargetChatId)
	if err != nil {
		return err
	}
	if !ok {
		delay := s.telegramChannelBusyDelay(ctx, jobId)
		return s.requeueTelegramJob(ctx, tgTaskTypePublish, jobId, delay)
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	return s.sendTelegramJobLockedByChannel(ctx, jobId)
}

func (s *sSysPublish) telegramChannelBusyDelay(ctx context.Context, jobId int64) time.Duration {
	base := g.Cfg().MustGet(ctx, "youbanPublish.queue.channelBusyDelaySeconds", 3).Int()
	if base <= 0 {
		base = 3
	}
	jitter := int(jobId % 3)
	return time.Duration(base+jitter) * time.Second
}

func (s *sSysPublish) sendTelegramJobLockedByChannel(ctx context.Context, jobId int64) error {
	job, locked, err := s.lockTelegramJob(ctx, jobId)
	if err != nil || !locked {
		return err
	}
	allowed, err := s.canSendTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "上架任务已下架或不可发布，跳过TG推送")
		return s.markTelegramJobSuperseded(ctx, job.Id)
	}
	if err = s.markTaskPublishingStarted(ctx, job.TaskId, job.OperationNo); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "publish", "started", s.telegramJobPublishMessage(job, "开始推送TG资料"))
	if err = s.sendLockedTelegramJob(ctx, job); err != nil {
		return s.handleTelegramJobError(ctx, job, err)
	}
	return s.completeTelegramJob(ctx, job)
}

func (s *sSysPublish) markTaskPublishingStarted(ctx context.Context, taskId int64, operationNo string) error {
	if taskId <= 0 {
		return nil
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", taskId).
		WhereNull("deleted_at")
	if operationNo != "" {
		mod = mod.Where("tg_operation_no", operationNo)
	}
	_, err := mod.
		Data(g.Map{
			"tg_status":  "sending",
			"updated_at": gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新上架中状态失败")
	}
	return nil
}

func (s *sSysPublish) sendLockedTelegramJob(ctx context.Context, job telegramJobRecord) error {
	job.TargetChatId = normalizeTelegramChannelChatID(job.TargetChatId)
	if strings.TrimSpace(job.TargetChatId) == "" {
		return gerror.New("TG目标频道未配置")
	}
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	caption, err := s.telegramJobText(ctx, job.TaskId)
	if err != nil {
		return err
	}
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		return err
	}
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		return err
	}
	messages, err := s.sendTelegramDisplayPart(ctx, bot, job.TargetChatId, caption, displayMedia)
	if err != nil {
		return gerror.Wrapf(err, "TG展示资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
	}
	verifyMessages, err := s.sendTelegramVerifyPart(ctx, bot, job.TargetChatId, verifyMedia)
	if err != nil {
		return gerror.Wrapf(err, "TG验证资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
	}
	messages = append(messages, verifyMessages...)
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	return s.updateTelegramMediaFileIds(ctx, messages)
}

func (s *sSysPublish) sendTelegramDisplayPart(ctx context.Context, bot *tgbot.Bot, chatId string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) > 0 {
		return s.sendTelegramMediaSet(ctx, bot, chatId, "display", caption, media)
	}
	if strings.TrimSpace(caption) == "" {
		return nil, gerror.New("展示资料和推送文案不能同时为空")
	}
	msg, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: caption, ParseMode: models.ParseModeHTML})
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	return []*telegramSentMessage{{MessageId: int64(msg.ID), Purpose: "display"}}, nil
}

func (s *sSysPublish) sendTelegramVerifyPart(ctx context.Context, bot *tgbot.Bot, chatId string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	return s.sendTelegramMediaSet(ctx, bot, chatId, "verify", "", media)
}

func (s *sSysPublish) handleTelegramJobError(ctx context.Context, job telegramJobRecord, err error) error {
	allowed, allowedErr := s.canSendTelegramJob(ctx, job)
	if allowedErr == nil && !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG推送失败时任务已被新操作覆盖，旧任务已废弃")
		return s.markTelegramJobSuperseded(ctx, job.Id)
	}
	var tooMany *tgbot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		if switched, switchErr := s.switchTelegramJobToNextBot(ctx, job, err); switchErr != nil {
			return switchErr
		} else if switched {
			return &tgRetryAfterError{after: time.Second, err: err}
		}
	}
	retryCount := job.RetryCount + 1
	retryDelay := time.Minute
	if errors.As(err, &tooMany) && tooMany.RetryAfter > 0 {
		retryDelay = time.Duration(tooMany.RetryAfter) * time.Second
	}
	message := telegramJobFriendlyErrorMessage(err, retryDelay, retryCount)
	status := "failed_retry"
	if retryCount >= 10 {
		status = "failed"
	}
	nextRetryAt := gtime.Now().Add(retryDelay)
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":        status,
		"retry_count":   retryCount,
		"next_retry_at": nextRetryAt,
		"error_message": message,
		"updated_at":    gtime.Now(),
	}).Update()
	s.appendTelegramJobLog(ctx, job, "publish", status, message)
	if status == "failed" {
		failMod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", job.TaskId)
		if job.OperationNo != "" {
			failMod = failMod.Where("tg_operation_no", job.OperationNo)
		}
		_, _ = failMod.Data(g.Map{
			"status":        sysin.PublishTaskStatusFailed,
			"tg_status":     sysin.PublishTaskStatusFailed,
			"error_message": message,
			"updated_at":    gtime.Now(),
		}).Update()
		_ = s.markCollectDispatchFailedByTask(ctx, job.TaskId, message)
		return err
	}
	return &tgRetryAfterError{after: retryDelay, err: err}
}

func telegramJobFriendlyErrorMessage(err error, retryDelay time.Duration, retryCount int) string {
	var tooMany *tgbot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		seconds := int(retryDelay.Seconds())
		if tooMany.RetryAfter > 0 {
			seconds = tooMany.RetryAfter
		}
		return fmt.Sprintf("Telegram 发送频率过快，已触发限流；系统会等待 %d 秒后自动重试（第 %d 次）。建议为该频道配置多个可用推送 BOT，或降低全量推送/循环推送频率。", seconds, retryCount)
	}
	return err.Error()
}

func (s *sSysPublish) switchTelegramJobToNextBot(ctx context.Context, job telegramJobRecord, cause error) (bool, error) {
	nextBotId, err := s.nextTelegramChannelBotId(ctx, job)
	if err != nil {
		return false, err
	}
	if nextBotId <= 0 || nextBotId == job.BotId {
		return false, nil
	}
	now := gtime.Now()
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"bot_id":        nextBotId,
		"status":        "pending",
		"retry_count":   job.RetryCount + 1,
		"next_retry_at": nil,
		"error_message": "",
		"updated_at":    now,
	}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "切换备用BOT失败")
	}
	s.appendTelegramJobLog(ctx, job, "publish", "bot_switched", fmt.Sprintf("当前BOT限流，已切换备用BOT：%d，原因：%s", nextBotId, cause.Error()))
	return true, nil
}

func (s *sSysPublish) completeTelegramJob(ctx context.Context, job telegramJobRecord) error {
	allowed, err := s.canSendTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成前任务已下架，开始清理已发送消息")
		if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "任务已下架"); err != nil {
			return err
		}
		return s.markTelegramJobSuperseded(ctx, job.Id)
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":        "sent",
		"error_message": "",
		"sent_at":       gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG任务状态失败")
	}
	s.appendTelegramJobLog(ctx, job, "publish", "sent", s.telegramJobPublishMessage(job, "TG资料推送成功"))
	allSent, err := s.allTelegramTaskJobsSent(ctx, job.TaskId, job.OperationNo)
	if err != nil || !allSent {
		return err
	}
	updated, err := s.markTaskPublishedAfterTelegram(ctx, job.TaskId, job.OperationNo)
	if err != nil {
		return err
	}
	if !updated {
		if err = s.markPublishedTaskTelegramSent(ctx, job.TaskId, job.OperationNo); err != nil {
			return err
		}
	}
	if updated {
		if err = s.incrementDailyPublishStat(ctx, job); err != nil {
			return err
		}
		if err = s.markCollectDispatchSentByTask(ctx, job.TaskId); err != nil {
			return err
		}
	}
	return s.scheduleTelegramCycleDelete(ctx, job)
}

func (s *sSysPublish) telegramJobPublishMessage(job telegramJobRecord, message string) string {
	if job.CycleEnabled == 1 && job.NextCycleAt != nil {
		return "循环上架发布：" + message
	}
	return message
}

func (s *sSysPublish) scheduleTelegramCycleDelete(ctx context.Context, job telegramJobRecord) error {
	return s.ensureCyclePlanForJob(ctx, job)
}

func (s *sSysPublish) canSendTelegramJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	task, err := s.cycleTaskForJob(ctx, job)
	if err != nil {
		return false, err
	}
	if task.IsEmpty() {
		return false, nil
	}
	if job.OperationNo != "" && task["tg_operation_no"].String() != "" && task["tg_operation_no"].String() != job.OperationNo {
		return false, nil
	}
	switch task["status"].String() {
	case sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing, sysin.PublishTaskStatusPublished:
		return true, nil
	default:
		return false, nil
	}
}
