package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/dao"
)

type scheduledPublishProfile struct {
	AccountId int64       `json:"accountId"`
	ProfileId int64       `json:"profileId"`
	PublishAt *gtime.Time `json:"publishAt"`
	TenantId  int64       `json:"tenantId"`
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
	var list []*scheduledPublishProfile
	err := g.DB().Model(publishProfileStateTable+" ps").Safe().Ctx(ctx).
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		InnerJoin(publishAccountTable+" a", "a.id=ps.account_id AND a.deleted_at IS NULL AND a.status=1").
		Fields("ps.profile_id,ps.tenant_id,ps.account_id,ps.publish_at").
		WhereNotNull("ps.publish_at").
		WhereLTE("ps.publish_at", gtime.Now()).
		WhereNull("ps.deleted_at").
		OrderAsc("ps.publish_at").
		Limit(50).
		Scan(&list)
	if err != nil {
		return gerror.Wrap(err, "读取到期上架资料失败")
	}
	for _, item := range list {
		if item == nil || item.ProfileId <= 0 || item.TenantId <= 0 || item.AccountId <= 0 || item.PublishAt == nil {
			continue
		}
		operationNo := fmt.Sprintf("scheduled:%d:%d", item.ProfileId, item.PublishAt.TimestampNano())
		if err = s.submitProfilePublish(ctx, item.ProfileId, item.TenantId, item.AccountId, item.AccountId, operationNo, nil, false); err != nil {
			g.Log().Warningf(ctx, "提交到期上架资料失败 profile:%d err:%+v", item.ProfileId, err)
			continue
		}
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("profile_id", item.ProfileId).
			Where("publish_at", item.PublishAt).
			Data(g.Map{"publish_at": nil, "updated_at": gtime.Now()}).Update()
	}
	return nil
}
