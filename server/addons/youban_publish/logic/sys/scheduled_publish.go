package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type scheduledPublishTask struct {
	AccountId int64 `json:"accountId"`
	Id        int64 `json:"id"`
	TenantId  int64 `json:"tenantId"`
}

func (s *sSysPublish) runScheduledPublishRuntime(ctx context.Context) {
	interval := g.Cfg().MustGet(ctx, "youbanPublish.scheduled.intervalSeconds", 30).Int()
	if interval <= 0 {
		interval = 30
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := s.submitDuePublishTasks(ctx); err != nil {
				g.Log().Warningf(ctx, "提交到期上架任务失败：%+v", err)
			}
			timer.Reset(time.Duration(interval) * time.Second)
		}
	}
}

func (s *sSysPublish) submitDuePublishTasks(ctx context.Context) error {
	var list []*scheduledPublishTask
	err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id").
		Where("status", sysin.PublishTaskStatusPending).
		Where("tg_push_enabled", 1).
		Where("published_at IS NOT NULL").
		Where("published_at <= ?", gtime.Now()).
		WhereNull("deleted_at").
		OrderAsc("published_at").
		Limit(50).
		Scan(&list)
	if err != nil {
		return gerror.Wrap(err, "读取到期上架任务失败")
	}
	for _, item := range list {
		if item == nil || item.Id <= 0 || item.TenantId <= 0 || item.AccountId <= 0 {
			continue
		}
		if err = s.submitTaskByTenant(ctx, item.Id, item.TenantId, item.AccountId); err != nil {
			g.Log().Warningf(ctx, "提交到期上架任务失败 task:%d err:%+v", item.Id, err)
		}
	}
	return nil
}
