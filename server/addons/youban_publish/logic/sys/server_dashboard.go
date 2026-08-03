package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
)

const serverDashboardCacheTTL = time.Minute

func (s *sSysPublish) ServerDashboard(ctx context.Context, in *sysin.TrendInp) (*sysin.ServerDashboardModel, error) {
	dateRange, err := resolveTrendDateRange(in)
	if err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("youban_publish:server_dashboard:%s:%s", dateRange.Start, dateRange.End)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		var cached sysin.ServerDashboardModel
		if scanErr := value.Scan(&cached); scanErr == nil {
			return &cached, nil
		}
	}
	taskTrend, taskCounts, err := s.serverDashboardPublishTaskTrend(ctx, dateRange)
	if err != nil {
		return nil, err
	}
	profileTrend, err := s.serverDashboardProfileTrend(ctx, dateRange)
	if err != nil {
		return nil, err
	}
	basicCounts, err := s.serverDashboardBasicCounts(ctx)
	if err != nil {
		return nil, err
	}
	tgOnline, tgTotal, err := s.serverDashboardTgCounts(ctx)
	if err != nil {
		return nil, err
	}
	todos, err := s.serverDashboardTodos(ctx)
	if err != nil {
		return nil, err
	}
	failureTop, err := s.serverDashboardPublishFailureTop(ctx, dateRange)
	if err != nil {
		return nil, err
	}
	profileTop, err := s.serverDashboardProfilePublishTop(ctx, dateRange)
	if err != nil {
		return nil, err
	}
	result := &sysin.ServerDashboardModel{
		Stats:             serverDashboardStats(basicCounts, taskCounts, tgOnline),
		TaskTrend:         taskTrend,
		ProfileTrend:      profileTrend,
		Health:            serverDashboardHealth(tgOnline, tgTotal, taskCounts[sysin.PublishTaskStatusFailed], basicCounts["channels"]),
		Todos:             todos,
		PublishFailureTop: failureTop,
		ProfilePublishTop: profileTop,
		StartDate:         dateRange.Start,
		EndDate:           dateRange.End,
		UpdatedAt:         gtime.Now().String(),
	}
	_ = cache.Instance().Set(ctx, cacheKey, result, serverDashboardCacheTTL)
	return result, nil
}

func serverDashboardStats(basic map[string]int, tasks map[string]int, tgOnline int) []*sysin.ServerDashboardStat {
	successRate := dashboardSuccessRate(tasks[sysin.PublishTaskStatusPublished], tasks[sysin.PublishTaskStatusFailed])
	return []*sysin.ServerDashboardStat{
		{Key: "tenants", Title: "账号归属", Value: basic["tenants"]},
		{Key: "accounts", Title: "上架账号", Value: basic["accounts"]},
		{Key: "tasks", Title: "范围内任务", Value: tasks["total"]},
		{Key: "published", Title: "发布成功", Value: tasks[sysin.PublishTaskStatusPublished]},
		{Key: "failed", Title: "发布失败", Value: tasks[sysin.PublishTaskStatusFailed]},
		{Key: "tgOnline", Title: "在线协议号", Value: tgOnline},
		{Key: "channels", Title: "频道数量", Value: basic["channels"]},
		{Key: "successRate", Title: "发布成功率", Value: int(successRate), Suffix: "%", Rate: successRate},
	}
}

func (s *sSysPublish) serverDashboardPublishTaskTrend(ctx context.Context, dateRange *trendDateRange) ([]*sysin.ServerDashboardTrendPoint, map[string]int, error) {
	var rows []struct {
		Date   string `json:"date"`
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	err := g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		Fields("DATE(created_at) AS date", "status", "COUNT(*) AS count").
		WhereGTE("created_at", dateRange.Start+" 00:00:00").
		WhereLT("created_at", dashboardRangeEndExclusive(dateRange.End)).
		Group("DATE(created_at),status").
		Scan(&rows)
	if err != nil {
		return nil, nil, gerror.Wrap(err, "统计后台任务趋势失败")
	}
	points := makeDashboardTaskTrendPoints(dateRange)
	index := make(map[string]*sysin.ServerDashboardTrendPoint, len(points))
	counts := map[string]int{"total": 0}
	for _, point := range points {
		index[point.Date] = point
	}
	for _, row := range rows {
		point := index[row.Date]
		if point == nil {
			continue
		}
		point.Created += row.Count
		counts["total"] += row.Count
		switch row.Status {
		case "success", "sent":
			point.Published += row.Count
			counts[sysin.PublishTaskStatusPublished] += row.Count
		case "failed":
			point.Failed += row.Count
			counts[sysin.PublishTaskStatusFailed] += row.Count
		case "canceled", "superseded":
			point.Canceled += row.Count
			counts[sysin.PublishTaskStatusCanceled] += row.Count
		}
	}
	return points, counts, nil
}

func (s *sSysPublish) serverDashboardProfileTrend(ctx context.Context, dateRange *trendDateRange) ([]*sysin.ServerDashboardProfileTrendPoint, error) {
	points := makeDashboardProfileTrendPoints(dateRange)
	index := make(map[string]*sysin.ServerDashboardProfileTrendPoint, len(points))
	for _, point := range points {
		index[point.Date] = point
	}
	var createdRows []struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	if err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("DATE(created_at) AS date", "COUNT(*) AS count").
		WhereGTE("created_at", dateRange.Start+" 00:00:00").
		WhereLT("created_at", dashboardRangeEndExclusive(dateRange.End)).
		Group("DATE(created_at)").Scan(&createdRows); err != nil {
		return nil, gerror.Wrap(err, "统计新增资料趋势失败")
	}
	for _, row := range createdRows {
		if point := index[row.Date]; point != nil {
			point.Created = row.Count
		}
	}
	var statRows []struct {
		Date      string `json:"date"`
		Published int    `json:"published"`
		Down      int    `json:"down"`
	}
	if err := g.DB().Model(publishDailyStatTable).Safe().Ctx(ctx).
		Fields("stat_date AS date", "SUM(published_count) AS published", "SUM(down_count) AS down").
		WhereBetween("stat_date", dateRange.Start, dateRange.End).
		Group("stat_date").Scan(&statRows); err != nil {
		return nil, gerror.Wrap(err, "统计资料上架趋势失败")
	}
	for _, row := range statRows {
		if point := index[row.Date]; point != nil {
			point.Published = row.Published
			point.Down = row.Down
		}
	}
	return points, nil
}

func (s *sSysPublish) serverDashboardPublishFailureTop(ctx context.Context, dateRange *trendDateRange) ([]*sysin.ServerDashboardRank, error) {
	var rows []struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	err := g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		Fields("message", "COUNT(*) AS count").
		Where("status", "failed").
		WhereNot("message", "").
		WhereGTE("created_at", dateRange.Start+" 00:00:00").
		WhereLT("created_at", dashboardRangeEndExclusive(dateRange.End)).
		Group("message").OrderDesc("count").Limit(10).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计发布失败Top10失败")
	}
	items := make([]*sysin.ServerDashboardRank, 0, len(rows))
	for index, row := range rows {
		items = append(items, &sysin.ServerDashboardRank{Key: fmt.Sprintf("failure-%d", index), Name: dashboardTrim(row.Message, 42), Value: row.Count, Desc: "失败次数", Status: "error"})
	}
	return items, nil
}

func (s *sSysPublish) serverDashboardProfilePublishTop(ctx context.Context, dateRange *trendDateRange) ([]*sysin.ServerDashboardRank, error) {
	var rows []struct {
		ProfileId int64  `json:"profileId"`
		ProfileNo string `json:"profileNo"`
		Title     string `json:"title"`
		Count     int    `json:"count"`
	}
	err := g.DB().Model(publishSuccessRecordTable+" r").Safe().Ctx(ctx).
		LeftJoin(dao.ContentProfile.Table()+" p", "p.id=r.profile_id").
		Fields("r.profile_id", "p.profile_no", "p.title", "COUNT(*) AS count").
		WhereIn("r.status", []string{"success", "sent"}).WhereGT("r.profile_id", 0).
		WhereGTE("r.created_at", dateRange.Start+" 00:00:00").
		WhereLT("r.created_at", dashboardRangeEndExclusive(dateRange.End)).
		Group("r.profile_id,p.profile_no,p.title").OrderDesc("count").Limit(10).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计资料发布Top10失败")
	}
	items := make([]*sysin.ServerDashboardRank, 0, len(rows))
	for _, row := range rows {
		name := dashboardText(strings.TrimSpace(row.ProfileNo), fmt.Sprintf("资料 %d", row.ProfileId))
		if title := strings.TrimSpace(row.Title); title != "" {
			name += " · " + dashboardTrim(title, 24)
		}
		items = append(items, &sysin.ServerDashboardRank{Key: fmt.Sprintf("profile-%d", row.ProfileId), Name: name, Value: row.Count, Desc: "发布次数", Status: "success"})
	}
	return items, nil
}

func makeDashboardTaskTrendPoints(dateRange *trendDateRange) []*sysin.ServerDashboardTrendPoint {
	points := make([]*sysin.ServerDashboardTrendPoint, 0, dateRange.Days)
	start, _ := time.Parse(trendDateLayout, dateRange.Start)
	for i := 0; i < dateRange.Days; i++ {
		points = append(points, &sysin.ServerDashboardTrendPoint{Date: start.AddDate(0, 0, i).Format(trendDateLayout)})
	}
	return points
}

func makeDashboardProfileTrendPoints(dateRange *trendDateRange) []*sysin.ServerDashboardProfileTrendPoint {
	points := make([]*sysin.ServerDashboardProfileTrendPoint, 0, dateRange.Days)
	start, _ := time.Parse(trendDateLayout, dateRange.Start)
	for i := 0; i < dateRange.Days; i++ {
		points = append(points, &sysin.ServerDashboardProfileTrendPoint{Date: start.AddDate(0, 0, i).Format(trendDateLayout)})
	}
	return points
}

func dashboardRangeEndExclusive(endDate string) string {
	end, _ := time.Parse(trendDateLayout, endDate)
	return end.AddDate(0, 0, 1).Format(trendDateLayout) + " 00:00:00"
}

func serverDashboardHealth(tgOnline int, tgTotal int, failed int, channels int) []*sysin.ServerDashboardHealth {
	tgStatus := "success"
	if tgTotal == 0 || tgOnline == 0 {
		tgStatus = "error"
	}
	queueStatus := "success"
	if failed > 0 {
		queueStatus = "warning"
	}
	channelStatus := "success"
	if channels == 0 {
		channelStatus = "error"
	}
	return []*sysin.ServerDashboardHealth{
		{Key: "tg", Title: "协议号池", Status: tgStatus, Value: fmt.Sprintf("%d/%d 在线", tgOnline, tgTotal), Message: "影响 Telegram 推送可用性"},
		{Key: "queue", Title: "发布队列", Status: queueStatus, Value: fmt.Sprintf("%d 个失败", failed), Message: "失败任务需要排查或重试"},
		{Key: "channel", Title: "频道配置", Status: channelStatus, Value: fmt.Sprintf("%d 个频道", channels), Message: "频道为资料分发目标"},
	}
}

func (s *sSysPublish) serverDashboardBasicCounts(ctx context.Context) (map[string]int, error) {
	tables := map[string]string{
		"tenants":  publishTenantTable,
		"accounts": publishAccountTable,
		"profiles": dao.ContentProfile.Table(),
		"channels": publishChannelTable,
		"bots":     publishBotTable,
	}
	counts := make(map[string]int, len(tables))
	for key, table := range tables {
		count, err := g.DB().Model(table).Safe().Ctx(ctx).WhereNull("deleted_at").Count()
		if err != nil {
			return nil, gerror.Wrap(err, "统计后台基础数据失败")
		}
		counts[key] = count
	}
	return counts, nil
}

func (s *sSysPublish) serverDashboardTgCounts(ctx context.Context) (online int, total int, err error) {
	total, err = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).WhereNull("deleted_at").Count()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "统计后台协议号失败")
	}
	online, err = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("status", sysin.PublishTgAccountStatusAuthorized).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "统计后台在线协议号失败")
	}
	return
}

func (s *sSysPublish) serverDashboardTodos(ctx context.Context) ([]*sysin.ServerDashboardTodo, error) {
	var rows []*collectTaskSummary
	err := s.collectTaskSummaryModel(ctx).
		WhereIn("t.status", []string{sysin.CollectDispatchStatusFailed, sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		OrderDesc("t.updated_at").
		Limit(dashboardTodoLimit).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "获取后台待办失败")
	}
	todos := make([]*sysin.ServerDashboardTodo, 0, len(rows))
	for _, row := range rows {
		todos = append(todos, &sysin.ServerDashboardTodo{
			Key:       fmt.Sprintf("task-%d", row.Id),
			Title:     dashboardText(row.Title, "未命名任务"),
			Desc:      serverDashboardTodoDesc(row),
			Status:    row.Status,
			UpdatedAt: dashboardTime(row.UpdatedAt),
		})
	}
	return todos, nil
}

func serverDashboardTodoDesc(row *collectTaskSummary) string {
	parts := []string{
		dashboardText(row.TenantName, "未知归属"),
		dashboardText(row.AccountNickname, row.AccountUsername),
		row.City,
	}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := strings.TrimSpace(part); text != "" {
			filtered = append(filtered, text)
		}
	}
	return strings.Join(filtered, " · ")
}
