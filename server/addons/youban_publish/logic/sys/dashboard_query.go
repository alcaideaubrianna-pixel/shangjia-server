package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
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
	mod := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("CASE WHEN p.status = 1 THEN 'published' ELSE 'pending' END AS status", "COUNT(*) AS count").
		Where("ps.tenant_id", tenantId).
		WhereNull("p.deleted_at").
		Group("CASE WHEN p.status = 1 THEN 'published' ELSE 'pending' END")
	if accountId > 0 {
		mod = mod.Where("ps.account_id", accountId)
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
	// 统一解析 days 或 startDate/endDate，确保趋势查询窗口不超过 90 天。
	dateRange, err := resolveTrendDateRange(in)
	if err != nil {
		return nil, err
	}
	var rows []dashboardTrendRow
	mod := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("DATE(p.created_at) AS date", "CASE WHEN p.status = 1 THEN 'published' ELSE 'pending' END AS status", "COUNT(*) AS count").
		Where("ps.tenant_id", tenantId).
		WhereGTE("p.created_at", dateRange.Start+" 00:00:00").
		WhereLTE("p.created_at", dateRange.End+" 23:59:59").
		WhereNull("p.deleted_at").
		Group("DATE(p.created_at),CASE WHEN p.status = 1 THEN 'published' ELSE 'pending' END")
	if accountId > 0 {
		mod = mod.Where("ps.account_id", accountId)
	}
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "统计发布趋势失败")
	}
	return buildDashboardPublishTrend(dateRange, rows), nil
}

func buildDashboardPublishTrend(dateRange *trendDateRange, rows []dashboardTrendRow) []*sysin.DashboardTrendPoint {
	points := make([]*sysin.DashboardTrendPoint, 0, dateRange.Days)
	index := make(map[string]*sysin.DashboardTrendPoint, dateRange.Days)
	start, _ := parseTrendDate(dateRange.Start, "开始日期")
	for i := 0; i < dateRange.Days; i++ {
		date := start.AddDate(0, 0, i).Format(trendDateLayout)
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
