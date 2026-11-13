package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type collectStatsAggregate struct {
	BlockedCount        int `orm:"blocked_count"`
	CollectingCount     int `orm:"collecting_count"`
	EventCount          int `orm:"event_count"`
	FailedDispatchCount int `orm:"failed_dispatch_count"`
	PendingReviewCount  int `orm:"pending_review_count"`
	RuleCount           int `orm:"rule_count"`
	SentCount           int `orm:"sent_count"`
	TodayPushedCount    int `orm:"today_pushed_count"`
}

func (s *sSysPublish) CollectStats(ctx context.Context) (*sysin.CollectStatsModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	aggregate, err := loadCollectStatsAggregate(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	stats := &sysin.CollectStatsModel{
		BlockedCount:        aggregate.BlockedCount,
		CollectingCount:     aggregate.CollectingCount,
		EventCount:          aggregate.EventCount,
		FailedDispatchCount: aggregate.FailedDispatchCount,
		PendingReviewCount:  aggregate.PendingReviewCount,
		RuleCount:           aggregate.RuleCount,
		TodayPushedCount:    aggregate.TodayPushedCount,
	}
	totalDispatch := aggregate.SentCount + aggregate.FailedDispatchCount
	if totalDispatch > 0 {
		stats.PushSuccessRate = aggregate.SentCount * 100 / totalDispatch
	}
	return stats, nil
}

func loadCollectStatsAggregate(ctx context.Context, tenantId, accountId int64) (*collectStatsAggregate, error) {
	cacheKey := fmt.Sprintf("youban_publish:collect:stats:%d:%d", tenantId, accountId)
	if cached, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !cached.IsNil() {
		var value collectStatsAggregate
		if cached.Scan(&value) == nil {
			return &value, nil
		}
	}
	var snapshot collectStatsAggregate
	if err := g.DB().Model("hg_youban_publish_collect_stats_snapshot").Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).Scan(&snapshot); err == nil && snapshot.EventCount > 0 {
		_ = cache.Instance().Set(ctx, cacheKey, snapshot, 30*time.Second)
		return &snapshot, nil
	}
	sourceCols := pdao.YoubanPublishCollectSource.Columns()
	ruleCols := pdao.YoubanPublishCollectRule.Columns()
	reviewCols := pdao.YoubanPublishCollectReview.Columns()
	dispatchCols := pdao.YoubanPublishCollectDispatch.Columns()
	query := fmt.Sprintf(`
SELECT
    source_stats.collecting_count,
    rule_stats.rule_count,
    source_stats.event_count,
    source_stats.blocked_count,
    review_stats.pending_review_count,
    dispatch_stats.failed_dispatch_count,
    dispatch_stats.sent_count,
    dispatch_stats.today_pushed_count
FROM (
    SELECT COUNT(*) FILTER (WHERE %s=1 AND %s=1) AS collecting_count,
           COALESCE(SUM(event_total),0) AS event_count,
           COALESCE(SUM(blocked_total),0) AS blocked_count
    FROM %s
    WHERE %s=? AND %s=? AND %s IS NULL
) source_stats
CROSS JOIN (
    SELECT COUNT(*) AS rule_count
    FROM %s
    WHERE %s=? AND %s=? AND %s IS NULL
) rule_stats
CROSS JOIN (
    SELECT COALESCE(SUM(CASE WHEN %s=? THEN 1 ELSE 0 END), 0) AS pending_review_count
    FROM %s
    WHERE %s=? AND %s=?
) review_stats
CROSS JOIN (
    SELECT COALESCE(SUM(CASE WHEN %s=? THEN 1 ELSE 0 END), 0) AS failed_dispatch_count,
           COALESCE(SUM(CASE WHEN %s=? THEN 1 ELSE 0 END), 0) AS sent_count,
           COALESCE(SUM(CASE WHEN %s=? AND %s>=? THEN 1 ELSE 0 END), 0) AS today_pushed_count
    FROM %s
    WHERE %s=? AND %s=?
) dispatch_stats`,
		sourceCols.CollectEnabled, sourceCols.Status, pdao.YoubanPublishCollectSource.Table(), sourceCols.TenantId, sourceCols.AccountId, sourceCols.DeletedAt,
		pdao.YoubanPublishCollectRule.Table(), ruleCols.TenantId, ruleCols.AccountId, ruleCols.DeletedAt,
		reviewCols.Status, pdao.YoubanPublishCollectReview.Table(), reviewCols.TenantId, reviewCols.AccountId,
		dispatchCols.Status, dispatchCols.Status, dispatchCols.Status, dispatchCols.FinishedAt, pdao.YoubanPublishCollectDispatch.Table(), dispatchCols.TenantId, dispatchCols.AccountId,
	)
	startOfDay := gtime.NewFromTime(time.Now()).StartOfDay()
	var aggregate collectStatsAggregate
	if err := g.DB().GetScan(ctx, &aggregate, query,
		tenantId, accountId,
		tenantId, accountId,
		tenantId, accountId,
		sysin.CollectReviewStatusPending, tenantId, accountId,
		sysin.CollectDispatchStatusFailed,
		sysin.CollectDispatchStatusSent,
		sysin.CollectDispatchStatusSent, startOfDay,
		tenantId, accountId,
	); err != nil {
		// Statistics are auxiliary to the collection workflow. Do not make a
		// database-pressure spike surface as an API failure and block the page;
		// return a safe empty snapshot while the next request retries.
		g.Log().Warningf(ctx, "统计采集数据失败，降级返回空统计 err:%+v", err)
		return &collectStatsAggregate{}, nil
	}
	_ = cache.Instance().Set(ctx, cacheKey, aggregate, 10*time.Second)
	_, _ = g.DB().Model("hg_youban_publish_collect_stats_snapshot").Ctx(ctx).
		Data(g.Map{"tenant_id": tenantId, "account_id": accountId,
			"collecting_count": aggregate.CollectingCount, "rule_count": aggregate.RuleCount,
			"event_count": aggregate.EventCount, "blocked_count": aggregate.BlockedCount,
			"pending_review_count": aggregate.PendingReviewCount, "failed_dispatch_count": aggregate.FailedDispatchCount,
			"sent_count": aggregate.SentCount, "today_pushed_count": aggregate.TodayPushedCount,
			"updated_at": gtime.Now()}).
		OnConflict("tenant_id,account_id").Save()
	return &aggregate, nil
}
