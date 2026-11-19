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

// runScheduledPublishRuntime 账户目录兜底和旧引擎到期发布独立计时。
func (s *sSysPublish) runScheduledPublishRuntime(ctx context.Context) {
	legacyInterval := g.Cfg().MustGet(ctx, "youbanPublish.scheduled.intervalSeconds", 30).Int()
	if legacyInterval <= 0 {
		legacyInterval = 30
	}
	accountInterval := g.Cfg().MustGet(ctx, "youbanPublish.queue.accountScheduleIntervalSeconds", 300).Int()
	if accountInterval <= 0 {
		accountInterval = 300
	}
	interval := time.Duration(accountInterval) * time.Second
	timer := time.NewTimer(interval)
	defer timer.Stop()
	legacyTimer := time.NewTimer(time.Duration(legacyInterval) * time.Second)
	defer legacyTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := s.enqueueAccountSchedules(ctx); err != nil {
				g.Log().Warningf(ctx, "分发账户调度任务失败 err:%+v", err)
			}
			timer.Reset(interval)
		case <-legacyTimer.C:
			if !globalStreamPushEngineEnabled() {
				if err := s.submitDuePublishTasks(ctx); err != nil {
					g.Log().Warningf(ctx, "提交旧引擎到期上架任务失败 err:%+v", err)
				}
			}
			legacyTimer.Reset(time.Duration(legacyInterval) * time.Second)
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
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=ps.profile_id AND p.deleted_at IS NULL AND p.status=1 AND p.visibility='public'").
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
		// 定时上架必须在最终提交阶段再次校验资料在线状态，防止资料在
		// 到期扫描与创建 TG Job 之间被下架后仍被推送。
		if err = s.submitProfilePublish(ctx, item.ProfileId, item.TenantId, item.AccountId, item.AccountId, operationNo, nil, true); err != nil {
			if errors.Is(err, errNoTelegramPublishChannels) || errors.Is(err, errTelegramPublishPermissionPaused) {
				// 计划投影必须保留，等待频道恢复后重试；清空 publish_at 会让资料永久失去循环入口。
				g.Log().Warningf(ctx, "定时上架资料暂未生成任务，保留计划等待重试 tenantId:%d accountId:%d profileId:%d publishAt:%s reason:%s", item.TenantId, item.AccountId, item.ProfileId, item.PublishAt.String(), err.Error())
				continue
			}
			g.Log().Warningf(ctx, "提交到期上架资料失败 profile:%d err:%+v", item.ProfileId, err)
			continue
		}
		// submitProfilePublish 可能在迁移/权限/频道映射异常时没有创建任何可执行 Job；
		// 没有 Job 就不能推进 publish_at，否则资料会永久丢失循环入口。
		createdCount, countErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("tenant_id", item.TenantId).Where("account_id", item.AccountId).
			Where("profile_id", item.ProfileId).Where("operation_no", operationNo).
			WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusSent, telegramJobStatusSending, telegramJobStatusConfirming}).Count()
		if countErr != nil || createdCount == 0 {
			g.Log().Warningf(ctx, "定时上架未生成有效Job，保留publish_at tenantId:%d accountId:%d profileId:%d operationNo:%s count:%d err:%+v", item.TenantId, item.AccountId, item.ProfileId, operationNo, createdCount, countErr)
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
