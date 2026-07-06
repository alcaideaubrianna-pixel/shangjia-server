package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type dashboardTrendRow struct {
	Date   string `json:"date"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

func (s *sSysPublish) dashboardTrend(ctx context.Context, in *sysin.TrendInp, tenantId int64, accountId int64) (*sysin.DashboardTrendModel, error) {
	profile, err := s.profileStats(ctx, in, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	publishTrend, err := s.dashboardPublishTrend(ctx, in, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	return &sysin.DashboardTrendModel{ProfileTrend: profile.Trend, PublishTrend: publishTrend}, nil
}

func (s *sSysPublish) dashboardTaskCounts(ctx context.Context, tenantId int64, accountId int64) (map[string]int, error) {
	var rows []struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("status", "COUNT(*) AS count").
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Group("status")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "统计任务状态失败")
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Status] = row.Count
	}
	return counts, nil
}

func (s *sSysPublish) dashboardSimpleCount(ctx context.Context, table string, tenantId int64, accountId int64) (int, error) {
	mod := g.DB().Model(table).Safe().Ctx(ctx).Where("tenant_id", tenantId).WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	count, err := mod.Count()
	if err != nil {
		return 0, gerror.Wrap(err, "统计工作台数量失败")
	}
	return count, nil
}

func (s *sSysPublish) dashboardTgAccountCounts(ctx context.Context, tenantId int64) (online int, total int, err error) {
	total, err = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "统计协议号失败")
	}
	online, err = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("status", sysin.PublishTgAccountStatusAuthorized).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "统计在线协议号失败")
	}
	return
}

func (s *sSysPublish) dashboardPublishTrend(ctx context.Context, in *sysin.TrendInp, tenantId int64, accountId int64) ([]*sysin.DashboardTrendPoint, error) {
	// 趋势查询限制最大 90 天，后续可平滑替换为日统计表。
	days := normalizeTrendDays(in)
	start := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")
	var rows []dashboardTrendRow
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("DATE(created_at) AS date", "status", "COUNT(*) AS count").
		Where("tenant_id", tenantId).
		WhereGTE("created_at", start+" 00:00:00").
		WhereNull("deleted_at").
		Group("DATE(created_at),status")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "统计发布趋势失败")
	}
	return buildDashboardPublishTrend(days, rows), nil
}

func buildDashboardPublishTrend(days int, rows []dashboardTrendRow) []*sysin.DashboardTrendPoint {
	points := make([]*sysin.DashboardTrendPoint, 0, days)
	index := make(map[string]*sysin.DashboardTrendPoint, days)
	for i := days - 1; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		point := &sysin.DashboardTrendPoint{Date: date}
		points = append(points, point)
		index[date] = point
	}
	for _, row := range rows {
		point := index[row.Date]
		if point == nil {
			continue
		}
		switch row.Status {
		case sysin.PublishTaskStatusPublished:
			point.Success += row.Count
		case sysin.PublishTaskStatusFailed:
			point.Failed += row.Count
		case sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing:
			point.Pending += row.Count
		}
	}
	return points
}

func normalizeTrendDays(in *sysin.TrendInp) int {
	days := 7
	if in != nil && in.Days > 0 {
		days = in.Days
	}
	if days > 90 {
		days = 90
	}
	return days
}
