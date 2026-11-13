package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"

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
		Where("tenant_id", tenantId).Where("account_id", accountId).Scan(&snapshot); err != nil {
		g.Log().Warningf(ctx, "读取采集统计快照失败 err:%+v", err)
		return &collectStatsAggregate{}, nil
	}
	_ = cache.Instance().Set(ctx, cacheKey, snapshot, 30*time.Second)
	return &snapshot, nil
}
