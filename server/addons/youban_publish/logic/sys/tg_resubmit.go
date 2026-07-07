package sys

import (
	"context"

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
	return s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), "编辑资料")
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
