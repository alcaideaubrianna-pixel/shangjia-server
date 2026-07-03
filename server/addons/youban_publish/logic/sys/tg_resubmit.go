package sys

import (
	"context"
	"fmt"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) prepareTelegramTaskForResubmit(ctx context.Context, task gdb.Record, channels []telegramJobChannel) error {
	selected := make(map[int64]struct{}, len(channels))
	for _, channel := range channels {
		selected[channel.Id] = struct{}{}
	}
	jobs, err := s.telegramTaskReusableJobs(ctx, task["id"].Int64())
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if job.Status == "sent" {
			if err = s.deleteTelegramJobMessagesForResubmit(ctx, job); err != nil {
				return err
			}
		}
		if _, ok := selected[job.ChannelId]; ok {
			if err = s.resetTelegramJobForResubmit(ctx, job.Id); err != nil {
				return err
			}
		} else {
			s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "delete", "superseded", "编辑资料后频道已取消，旧消息已删除")
			if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sSysPublish) telegramTaskReusableJobs(ctx context.Context, taskId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{"pending", "failed_retry", "failed", "sent"}).
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG历史任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) deleteTelegramJobMessagesForResubmit(ctx context.Context, job telegramResubmitJob) error {
	record := job.telegramJobRecord()
	messages, err := s.telegramJobActiveMessages(ctx, record)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		s.appendTelegramJobLog(ctx, record, "delete", "skipped", "编辑资料前未找到可删除的TG旧消息")
		return nil
	}
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, record, "delete", "started", "编辑资料，开始删除TG旧消息")
	for _, item := range messages {
		chatId := normalizeTelegramChannelChatID(item.TargetChatId)
		if item.MessageId <= 0 || chatId == "" {
			continue
		}
		_, err = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: int(item.MessageId)})
		if err != nil {
			message := fmt.Sprintf("删除TG旧消息失败，频道:%s，消息:%d，错误:%s", chatId, item.MessageId, err.Error())
			s.appendTelegramJobLog(ctx, record, "delete", "failed", message)
			return gerror.New(message)
		}
		_, _ = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			Data(g.Map{"status": "deleted", "deleted_at": gtime.Now(), "updated_at": gtime.Now()}).
			Update()
	}
	s.appendTelegramJobLog(ctx, record, "delete", "success", "编辑资料，TG旧消息删除成功")
	return nil
}

func (s *sSysPublish) resetTelegramJobForResubmit(ctx context.Context, jobId int64) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Data(g.Map{
			"status":        "pending",
			"retry_count":   0,
			"next_retry_at": nil,
			"next_cycle_at": nil,
			"error_message": "",
			"sent_at":       nil,
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "重置TG推送任务失败")
	}
	return nil
}

func (s *sSysPublish) markTelegramJobSuperseded(ctx context.Context, jobId int64) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Data(g.Map{
			"status":        "superseded",
			"next_retry_at": nil,
			"next_cycle_at": nil,
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "废弃TG推送任务失败")
	}
	return nil
}

type telegramResubmitJob struct {
	Id           int64  `json:"id"`
	TaskId       int64  `json:"taskId"`
	TenantId     int64  `json:"tenantId"`
	AccountId    int64  `json:"accountId"`
	ProfileId    int64  `json:"profileId"`
	ChannelId    int64  `json:"channelId"`
	BotId        int64  `json:"botId"`
	TargetChatId string `json:"targetChatId"`
	Status       string `json:"status"`
}

func (job telegramResubmitJob) telegramJobRecord() telegramJobRecord {
	return telegramJobRecord{
		Id:           job.Id,
		TaskId:       job.TaskId,
		TenantId:     job.TenantId,
		AccountId:    job.AccountId,
		ProfileId:    job.ProfileId,
		ChannelId:    job.ChannelId,
		BotId:        job.BotId,
		TargetChatId: job.TargetChatId,
	}
}
