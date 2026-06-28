package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (s *sSysPublish) consumeTelegramJobs(ctx context.Context) error {
	var rows []gdb.Record
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("status", "pending").
		Where("next_retry_at IS NULL OR next_retry_at <= ?", gtime.Now()).
		OrderAsc("id").
		Limit(10).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取TG发布任务失败")
	}
	for _, row := range rows {
		if err := s.consumeTelegramJob(ctx, row); err != nil {
			g.Log().Warningf(ctx, "处理TG发布任务失败 job:%d err:%+v", row["id"].Int64(), err)
		}
	}
	return nil
}

func (s *sSysPublish) consumeTelegramJob(ctx context.Context, job gdb.Record) error {
	if job.IsEmpty() {
		return nil
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job["id"].Int64()).Data(g.Map{
		"status":     "sending",
		"updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "锁定TG发布任务失败")
	}
	botId := job["bot_id"].Int64()
	targetChatId := strings.TrimSpace(job["target_chat_id"].String())
	conf, err := service.SysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	if targetChatId == "" {
		targetChatId = conf.DefaultTargetChat
	}
	if targetChatId == "" {
		return s.failTelegramJob(ctx, job, "Telegram目标Chat ID未配置")
	}
	botToken, err := s.telegramJobBotToken(ctx, botId, job["tenant_id"].Int64())
	if err != nil {
		return s.failTelegramJob(ctx, job, err.Error())
	}
	text, err := s.telegramJobText(ctx, job["task_id"].Int64())
	if err != nil {
		return s.failTelegramJob(ctx, job, err.Error())
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return s.failTelegramJob(ctx, job, err.Error())
	}
	msg, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: targetChatId,
		Text:   text,
	})
	if err != nil {
		return s.failTelegramJob(ctx, job, err.Error())
	}
	messageId := int64(0)
	if msg != nil {
		messageId = int64(msg.ID)
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job["id"].Int64()).Data(g.Map{
		"tg_message_id": messageId,
		"status":        "sent",
		"error_message": "",
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG发布任务失败")
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", job["task_id"].Int64()).Data(g.Map{
		"tg_status":  "sent",
		"updated_at": gtime.Now(),
	}).Update()
	return nil
}

func (s *sSysPublish) telegramJobBotToken(ctx context.Context, botId int64, tenantId int64) (string, error) {
	if botId > 0 {
		bot, err := s.getBotById(ctx, botId, tenantId)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(bot.BotToken) == "" {
			return "", gerror.New("Bot Token未配置")
		}
		return strings.TrimSpace(bot.BotToken), nil
	}
	bots, err := s.enabledBots(ctx, tenantId)
	if err != nil {
		return "", err
	}
	if len(bots) == 0 && tenantId > 0 {
		bots, err = s.enabledBots(ctx, 0)
		if err != nil {
			return "", err
		}
	}
	if len(bots) == 0 || bots[0] == nil || strings.TrimSpace(bots[0].BotToken) == "" {
		return "", gerror.New("未配置可用Bot")
	}
	return strings.TrimSpace(bots[0].BotToken), nil
}

func (s *sSysPublish) telegramJobText(ctx context.Context, taskId int64) (string, error) {
	row, err := s.getTask(ctx, taskId, 0)
	if err != nil {
		return "", err
	}
	lines := []string{
		strings.TrimSpace(row["title"].String()),
	}
	region := strings.TrimSpace(row["province"].String() + " " + row["city"].String())
	if region != "" {
		lines = append(lines, "地区："+region)
	}
	if text := strings.TrimSpace(row["plain_text"].String()); text != "" {
		lines = append(lines, "", text)
	}
	if row["profile_id"].Int64() > 0 {
		lines = append(lines, "", fmt.Sprintf("资料ID：%d", row["profile_id"].Int64()))
	}
	return strings.Join(lines, "\n"), nil
}

func (s *sSysPublish) failTelegramJob(ctx context.Context, job gdb.Record, message string) error {
	retryCount := job["retry_count"].Int() + 1
	status := "pending"
	nextRetryAt := gtime.Now().Add(time.Minute)
	if retryCount >= 3 {
		status = "failed"
		nextRetryAt = nil
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job["id"].Int64()).Data(g.Map{
		"status":        status,
		"retry_count":   retryCount,
		"next_retry_at": nextRetryAt,
		"error_message": message,
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG任务失败")
	}
	if status == "failed" {
		_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", job["task_id"].Int64()).Data(g.Map{
			"tg_status":     sysin.PublishTaskStatusFailed,
			"error_message": message,
			"updated_at":    gtime.Now(),
		}).Update()
	}
	return gerror.New(message)
}
