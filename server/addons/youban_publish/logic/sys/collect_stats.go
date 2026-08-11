package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
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
	sourceCols := pdao.YoubanPublishCollectSource.Columns()
	ruleCols := pdao.YoubanPublishCollectRule.Columns()
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	reviewCols := pdao.YoubanPublishCollectReview.Columns()
	dispatchCols := pdao.YoubanPublishCollectDispatch.Columns()
	query := fmt.Sprintf(`
SELECT
    source_stats.collecting_count,
    rule_stats.rule_count,
    event_stats.event_count,
    event_stats.blocked_count,
    review_stats.pending_review_count,
    dispatch_stats.failed_dispatch_count,
    dispatch_stats.sent_count,
    dispatch_stats.today_pushed_count
FROM (
    SELECT COUNT(*) AS collecting_count
    FROM %s
    WHERE %s=? AND %s=? AND %s=1 AND %s=1 AND %s IS NULL
) source_stats
CROSS JOIN (
    SELECT COUNT(*) AS rule_count
    FROM %s
    WHERE %s=? AND %s=? AND %s=1 AND %s IS NULL
) rule_stats
CROSS JOIN (
    SELECT COUNT(*) AS event_count,
           COALESCE(SUM(CASE WHEN %s=? THEN 1 ELSE 0 END), 0) AS blocked_count
    FROM %s
    WHERE %s=? AND %s=?
) event_stats
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
		pdao.YoubanPublishCollectSource.Table(), sourceCols.TenantId, sourceCols.AccountId, sourceCols.CollectEnabled, sourceCols.Status, sourceCols.DeletedAt,
		pdao.YoubanPublishCollectRule.Table(), ruleCols.TenantId, ruleCols.AccountId, ruleCols.Status, ruleCols.DeletedAt,
		eventCols.Status, pdao.YoubanPublishCollectEvent.Table(), eventCols.TenantId, eventCols.AccountId,
		reviewCols.Status, pdao.YoubanPublishCollectReview.Table(), reviewCols.TenantId, reviewCols.AccountId,
		dispatchCols.Status, dispatchCols.Status, dispatchCols.Status, dispatchCols.FinishedAt, pdao.YoubanPublishCollectDispatch.Table(), dispatchCols.TenantId, dispatchCols.AccountId,
	)
	startOfDay := gtime.NewFromTime(time.Now()).StartOfDay()
	var aggregate collectStatsAggregate
	if err := g.DB().GetScan(ctx, &aggregate, query,
		tenantId, accountId,
		tenantId, accountId,
		sysin.CollectEventStatusIgnored, tenantId, accountId,
		sysin.CollectReviewStatusPending, tenantId, accountId,
		sysin.CollectDispatchStatusFailed,
		sysin.CollectDispatchStatusSent,
		sysin.CollectDispatchStatusSent, startOfDay,
		tenantId, accountId,
	); err != nil {
		return nil, gerror.Wrap(err, "统计采集数据失败")
	}
	return &aggregate, nil
}
