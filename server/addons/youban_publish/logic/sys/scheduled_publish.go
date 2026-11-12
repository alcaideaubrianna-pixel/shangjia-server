package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/dao"
)

type scheduledPublishProfile struct {
	StateId   int64       `json:"stateId"`
	AccountId int64       `json:"accountId"`
	ProfileId int64       `json:"profileId"`
	PublishAt *gtime.Time `json:"publishAt"`
	TenantId  int64       `json:"tenantId"`
}

// runScheduledPublishRuntime 每秒分发账户调度；旧引擎定时上架保留原配置频率，互不阻塞账户任务执行。
func (s *sSysPublish) runScheduledPublishRuntime(ctx context.Context) {
	legacyInterval := g.Cfg().MustGet(ctx, "youbanPublish.scheduled.intervalSeconds", 30).Int()
	if legacyInterval <= 0 {
		legacyInterval = 30
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	var lastLegacy time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := s.enqueueAccountSchedules(ctx); err != nil {
				g.Log().Warningf(ctx, "分发账户调度任务失败 err:%+v", err)
			}
			if !globalStreamPushEngineEnabled() && time.Since(lastLegacy) >= time.Duration(legacyInterval)*time.Second {
				lastLegacy = time.Now()
				if err := s.submitDuePublishTasks(ctx); err != nil {
					g.Log().Warningf(ctx, "提交旧引擎到期上架任务失败 err:%+v", err)
				}
			}
			timer.Reset(time.Second)
		}
	}
}

// submitDuePublishTasks 按账户读取到期资料，提交成功后以原时间为条件清除计划，避免覆盖并发修改。
func (s *sSysPublish) submitDuePublishTasks(ctx context.Context, scopes ...publishAccountScope) error {
	if len(scopes) == 0 && globalStreamPushEngineEnabled() {
		return s.enqueueAccountSchedules(ctx)
	}
	var list []*scheduledPublishProfile
	model := g.DB().Model(publishProfileStateTable+" ps").Safe().Ctx(ctx).
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		InnerJoin(publishAccountTable+" a", "a.id=ps.account_id AND a.deleted_at IS NULL AND a.status=1").
		Fields("ps.id AS state_id,ps.profile_id,ps.tenant_id,ps.account_id,ps.publish_at").
		WhereNotNull("ps.publish_at").
		WhereLTE("ps.publish_at", gtime.Now()).
		WhereNull("ps.deleted_at").
		OrderAsc("ps.publish_at")
	err := accountScheduleModel(model, "ps.tenant_id", "ps.account_id", scopes).Limit(50).Scan(&list)
	if err != nil {
		return gerror.Wrap(err, "读取到期上架资料失败")
	}
	for _, item := range list {
		if item == nil || item.StateId <= 0 || item.ProfileId <= 0 || item.TenantId <= 0 || item.AccountId <= 0 || item.PublishAt == nil {
			continue
		}
		operationNo := fmt.Sprintf("scheduled:%d:%d", item.ProfileId, item.PublishAt.TimestampNano())
		if err = s.submitProfilePublish(ctx, item.ProfileId, item.TenantId, item.AccountId, item.AccountId, operationNo, nil, false); err != nil {
			if errors.Is(err, errNoTelegramPublishChannels) || errors.Is(err, errTelegramPublishPermissionPaused) {
				// 计划投影必须保留，等待频道恢复后重试；清空 publish_at 会让资料永久失去循环入口。
				g.Log().Warningf(ctx, "定时上架资料暂未生成任务，保留计划等待重试 tenantId:%d accountId:%d profileId:%d publishAt:%s reason:%s", item.TenantId, item.AccountId, item.ProfileId, item.PublishAt.String(), err.Error())
				continue
			}
			g.Log().Warningf(ctx, "提交到期上架资料失败 profile:%d err:%+v", item.ProfileId, err)
			continue
		}
		// 提交成功后才推进计划；循环资料写入下一次到期时间，单次定时任务才清空。
		nextPublishAt := any(nil)
		var cycleDays int
		if value, cycleErr := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("tenant_id", item.TenantId).Where("status", 1).Where("publish_direction", "up").Where("cycle_publish_enabled", 1).Where("cycle_publish_days > 0").Fields("MIN(cycle_publish_days)").Value(); cycleErr == nil && !value.IsNil() {
			cycleDays = value.Int()
		}
		if cycleDays > 0 {
			nextPublishAt = gtime.Now().Add(time.Duration(cycleDays) * 24 * time.Hour)
		}
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("tenant_id", item.TenantId).Where("account_id", item.AccountId).
			Where("profile_id", item.ProfileId).
			Where("publish_at", item.PublishAt).
			Data(g.Map{"publish_at": nextPublishAt, "updated_at": gtime.Now()}).Update()
	}
	return nil
}

func (s *sSysPublish) clearInvalidScheduledPublish(ctx context.Context, item *scheduledPublishProfile) error {
	_, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("id", item.StateId).Where("publish_at", item.PublishAt).
		WhereNull("deleted_at").
		Data(g.Map{"publish_at": nil, "updated_at": gtime.Now()}).Update()
	return err
}
