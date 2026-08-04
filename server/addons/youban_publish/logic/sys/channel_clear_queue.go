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
	result, err := base.
		WhereIn("status", channelQueueClearStatuses()).
		Data(g.Map{
			"status":              "superseded",
			"dispatch_status":     tgDispatchStatusDone,
			"next_retry_at":       nil,
			"error_message":       channelQueueClearMessage,
			"last_dispatch_error": channelQueueClearMessage,
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "清空频道发送队列失败")
	}
	affected, _ := result.RowsAffected()
	res.Cleared = int(affected)
	s.invalidateTelegramSchedulerChannelCache(ctx, channelId, "")
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
