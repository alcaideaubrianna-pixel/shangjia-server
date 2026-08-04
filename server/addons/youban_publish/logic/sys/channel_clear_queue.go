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
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		jobIds = append(jobIds, job.Id)
		if job.Status == "sending" {
			res.Sending++
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
	return res, nil
}

func (s *sSysPublish) channelClearQueueJobs(ctx context.Context, tenantId int64, channelId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("status", channelQueueClearStatuses())
	if channelId > 0 {
		mod = mod.Where("channel_id", channelId)
	}
	err := mod.OrderAsc("id").Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道发送队列失败")
	}
	return jobs, nil
}

func channelQueueClearStatuses() []string {
	return []string{"pending", "failed_retry", "sending", "unknown"}
}
