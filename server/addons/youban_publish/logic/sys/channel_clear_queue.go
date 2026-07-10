package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const channelQueueClearMessage = "用户清空频道发送队列"

func (s *sSysPublish) AdminChannelClearQueue(ctx context.Context, in *sysin.ChannelClearQueueInp) (res *sysin.ChannelClearQueueModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.ChannelId <= 0 {
		return nil, gerror.New("请选择频道")
	}
	channel, err := s.channelById(ctx, account.TenantId, in.ChannelId)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.Id <= 0 {
		return nil, gerror.New("频道不存在")
	}
	jobs, err := s.channelClearQueueJobs(ctx, account.TenantId, in.ChannelId)
	if err != nil {
		return nil, err
	}
	res = &sysin.ChannelClearQueueModel{ChannelId: in.ChannelId, Cleared: len(jobs)}
	if len(jobs) == 0 {
		return res, nil
	}
	jobIds := make([]int64, 0, len(jobs))
	taskIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		jobIds = append(jobIds, job.Id)
		if job.Status == "sending" {
			res.Sending++
		}
		if job.TaskId > 0 {
			taskIds = append(taskIds, job.TaskId)
		}
	}
	if len(jobIds) == 0 {
		return res, nil
	}
	now := gtime.Now()
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", jobIds).
		Where("tenant_id", account.TenantId).
		Where("channel_id", in.ChannelId).
		WhereIn("status", channelQueueClearStatuses()).
		Data(g.Map{
			"status":              "superseded",
			"dispatch_status":     tgDispatchStatusDone,
			"next_retry_at":       nil,
			"next_cycle_at":       nil,
			"error_message":       channelQueueClearMessage,
			"last_dispatch_error": channelQueueClearMessage,
			"updated_at":          now,
		}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "清空频道发送队列失败")
	}
	affected, _ := result.RowsAffected()
	res.Cleared = int(affected)
	for _, job := range jobs {
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "cleanup", "superseded", channelQueueClearMessage)
	}
	if err = s.markChannelQueueTasksSuperseded(ctx, account.TenantId, uniqueIds(taskIds)); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) channelClearQueueJobs(ctx context.Context, tenantId int64, channelId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("channel_id", channelId).
		WhereIn("status", channelQueueClearStatuses()).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道发送队列失败")
	}
	return jobs, nil
}

func (s *sSysPublish) markChannelQueueTasksSuperseded(ctx context.Context, tenantId int64, taskIds []int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	_, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("id", taskIds).
		WhereIn("tg_status", []string{"pending", "sending", "failed_retry"}).
		Data(g.Map{
			"tg_status":     "superseded",
			"error_message": channelQueueClearMessage,
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "同步资料推送状态失败")
	}
	return nil
}

func channelQueueClearStatuses() []string {
	return []string{"pending", "failed_retry", "sending"}
}
