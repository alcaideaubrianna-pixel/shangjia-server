package sys

import (
	"context"
	"errors"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) SendTelegramJob(ctx context.Context, jobId int64) error {
	job, locked, err := s.lockTelegramJob(ctx, jobId)
	if err != nil || !locked {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "publish", "started", "开始推送TG资料")
	if err = s.sendLockedTelegramJob(ctx, job); err != nil {
		return s.handleTelegramJobError(ctx, job, err)
	}
	return s.completeTelegramJob(ctx, job)
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
	msg, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: caption})
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
	retryCount := job.RetryCount + 1
	retryDelay := time.Minute
	var tooMany *tgbot.TooManyRequestsError
	if errors.As(err, &tooMany) && tooMany.RetryAfter > 0 {
		retryDelay = time.Duration(tooMany.RetryAfter) * time.Second
	}
	status := "failed_retry"
	if retryCount >= 10 {
		status = "failed"
	}
	nextRetryAt := gtime.Now().Add(retryDelay)
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":        status,
		"retry_count":   retryCount,
		"next_retry_at": nextRetryAt,
		"error_message": err.Error(),
		"updated_at":    gtime.Now(),
	}).Update()
	s.appendTelegramJobLog(ctx, job, "publish", status, err.Error())
	if status == "failed" {
		_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", job.TaskId).Data(g.Map{
			"status":        sysin.PublishTaskStatusFailed,
			"tg_status":     sysin.PublishTaskStatusFailed,
			"error_message": err.Error(),
			"updated_at":    gtime.Now(),
		}).Update()
		_ = s.markCollectDispatchFailedByTask(ctx, job.TaskId, err.Error())
		return err
	}
	return &tgRetryAfterError{after: retryDelay, err: err}
}

func (s *sSysPublish) completeTelegramJob(ctx context.Context, job telegramJobRecord) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":        "sent",
		"error_message": "",
		"sent_at":       gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG任务状态失败")
	}
	s.appendTelegramJobLog(ctx, job, "publish", "sent", "TG资料推送成功")
	allSent, err := s.allTelegramTaskJobsSent(ctx, job.TaskId)
	if err != nil || !allSent {
		return err
	}
	updated, err := s.markTaskPublishedAfterTelegram(ctx, job.TaskId)
	if err != nil || !updated {
		return err
	}
	if err = s.incrementDailyPublishStat(ctx, job); err != nil {
		return err
	}
	if err = s.markCollectDispatchSentByTask(ctx, job.TaskId); err != nil {
		return err
	}
	return s.scheduleTelegramCycleDelete(ctx, job)
}

func (s *sSysPublish) scheduleTelegramCycleDelete(ctx context.Context, job telegramJobRecord) error {
	if job.CycleEnabled != 1 {
		return nil
	}
	days := defaultCycleDays(job.CycleDays)
	delay := time.Duration(days) * 24 * time.Hour
	if isDevelopMode(ctx) {
		if seconds := g.Cfg().MustGet(ctx, "youbanPublish.cycle.devDelaySeconds", 0).Int(); seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		}
	}
	nextCycleAt := gtime.Now().Add(delay)
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"next_cycle_at": nextCycleAt,
		"updated_at":    gtime.Now(),
	}).Update()
	return s.enqueueTelegramDeleteJob(ctx, job.Id, delay)
}
