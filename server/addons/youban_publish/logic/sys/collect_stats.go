package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectStats(ctx context.Context) (*sysin.CollectStatsModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	stats := &sysin.CollectStatsModel{}
	if stats.CollectingCount, err = collectCount(ctx, pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id).
		Where("collect_enabled", 1).Where("status", 1).WhereNull("deleted_at")); err != nil {
		return nil, err
	}
	if stats.RuleCount, err = collectCount(ctx, pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id).
		Where("status", 1).WhereNull("deleted_at")); err != nil {
		return nil, err
	}
	if stats.EventCount, err = collectCount(ctx, pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id)); err != nil {
		return nil, err
	}
	if stats.BlockedCount, err = collectCount(ctx, pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id).
		Where("status", sysin.CollectEventStatusIgnored)); err != nil {
		return nil, err
	}
	if stats.PendingReviewCount, err = collectCount(ctx, pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id).
		Where("status", sysin.CollectReviewStatusPending)); err != nil {
		return nil, err
	}
	if stats.FailedDispatchCount, err = collectCount(ctx, pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("account_id", account.Id).
		Where("status", sysin.CollectDispatchStatusFailed)); err != nil {
		return nil, err
	}
	if stats.TodayPushedCount, err = s.collectTodayPushedCount(ctx, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	stats.PushSuccessRate, err = s.collectPushSuccessRate(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func collectCount(ctx context.Context, mod *gdb.Model) (int, error) {
	count, err := mod.Count()
	if err != nil {
		return 0, gerror.Wrap(err, "统计采集数据失败")
	}
	return count, nil
}

func (s *sSysPublish) collectTodayPushedCount(ctx context.Context, tenantId int64, accountId int64) (int, error) {
	start := gtime.NewFromTime(time.Now()).StartOfDay()
	return collectCount(ctx, pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).
		Where("status", sysin.CollectDispatchStatusSent).
		WhereGTE("finished_at", start))
}

func (s *sSysPublish) collectPushSuccessRate(ctx context.Context, tenantId int64, accountId int64) (int, error) {
	sent, err := collectCount(ctx, pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).
		Where("status", sysin.CollectDispatchStatusSent))
	if err != nil {
		return 0, err
	}
	failed, err := collectCount(ctx, pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).
		Where("status", sysin.CollectDispatchStatusFailed))
	if err != nil {
		return 0, err
	}
	total := sent + failed
	if total == 0 {
		return 0, nil
	}
	return sent * 100 / total, nil
}
