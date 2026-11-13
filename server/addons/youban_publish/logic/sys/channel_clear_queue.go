package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

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
	return s.clearTelegramChannelQueue(ctx, account.TenantId, in.ChannelId)
}

func (s *sSysPublish) clearTelegramChannelQueue(ctx context.Context, tenantId int64, channelId int64) (*sysin.ChannelClearQueueModel, error) {
	res := &sysin.ChannelClearQueueModel{ChannelId: channelId}
	if tenantId <= 0 {
		return nil, gerror.New("账号归属不能为空")
	}
	base := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("tenant_id", tenantId)
	if channelId > 0 {
		base = base.Where("channel_id", channelId)
	}
	sending, err := base.Clone().Where("status", "sending").Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计频道发送中任务失败")
	}
	res.Sending = sending

	// 清空队列必须复用统一状态迁移入口，确保批量废弃同时收口
	// Outbox、Redis 频道索引并唤醒下一条任务，避免清空后频道继续卡死。
	values, err := base.Clone().
		WhereIn("status", channelQueueClearStatuses()).
		Fields("id").Array()
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道待清理任务失败")
	}
	jobIDs := make([]int64, 0, len(values))
	for _, value := range values {
		if id := value.Int64(); id > 0 {
			jobIDs = append(jobIDs, id)
		}
	}
	affected, err := s.transitionTelegramJobs(ctx, jobIDs, telegramJobTransition{
		From:         channelQueueClearStatuses(),
		To:           telegramJobStatusSuperseded,
		ErrorMessage: channelQueueClearMessage,
		Additional:   g.Map{"last_dispatch_error": channelQueueClearMessage},
	})
	if err != nil {
		return nil, gerror.Wrap(err, "清空频道发送队列失败")
	}
	res.Cleared = int(affected)
	return res, nil
}

func (s *sSysPublish) channelClearQueueCount(ctx context.Context, tenantId int64, channelId int64) (int, error) {
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("status", channelQueueClearStatuses())
	if channelId > 0 {
		mod = mod.Where("channel_id", channelId)
	}
	count, err := mod.Count()
	if err != nil {
		return 0, gerror.Wrap(err, "统计频道发送队列失败")
	}
	return count, nil
}

func channelQueueClearStatuses() []string {
	return []string{"pending", "failed_retry", "unknown"}
}
