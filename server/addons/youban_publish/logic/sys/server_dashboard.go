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
)

func (s *sSysPublish) ServerDashboard(ctx context.Context, in *sysin.TrendInp) (*sysin.ServerDashboardModel, error) {
	days := normalizeTrendDays(in)
	taskCounts, err := s.serverDashboardTaskCounts(ctx)
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
	trend, err := s.serverDashboardTaskTrend(ctx, days)
	if err != nil {
		return nil, err
	}
	todos, err := s.serverDashboardTodos(ctx)
	if err != nil {
		return nil, err
	}
	tenantRank, err := s.serverDashboardTenantRank(ctx)
	if err != nil {
		return nil, err
	}
	errorRank, err := s.serverDashboardErrorRank(ctx)
	if err != nil {
		return nil, err
	}
	return &sysin.ServerDashboardModel{
		Stats:      serverDashboardStats(basicCounts, taskCounts, tgOnline),
		TaskTrend:  trend,
		Health:     serverDashboardHealth(tgOnline, tgTotal, taskCounts[sysin.PublishTaskStatusFailed], basicCounts["channels"]),
		Todos:      todos,
		TenantRank: tenantRank,
		ErrorRank:  errorRank,
		UpdatedAt:  gtime.Now().String(),
	}, nil
}

func serverDashboardStats(basic map[string]int, tasks map[string]int, tgOnline int) []*sysin.ServerDashboardStat {
	successRate := dashboardSuccessRate(tasks[sysin.PublishTaskStatusPublished], tasks[sysin.PublishTaskStatusFailed])
	return []*sysin.ServerDashboardStat{
		{Key: "tenants", Title: "账号归属", Value: basic["tenants"]},
		{Key: "accounts", Title: "上架账号", Value: basic["accounts"]},
		{Key: "tasks", Title: "任务总数", Value: basic["tasks"]},
		{Key: "published", Title: "发布成功", Value: tasks[sysin.PublishTaskStatusPublished]},
		{Key: "failed", Title: "发布失败", Value: tasks[sysin.PublishTaskStatusFailed]},
		{Key: "tgOnline", Title: "在线协议号", Value: tgOnline},
		{Key: "channels", Title: "频道数量", Value: basic["channels"]},
		{Key: "successRate", Title: "发布成功率", Value: int(successRate), Suffix: "%", Rate: successRate},
	}
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

func (s *sSysPublish) serverDashboardTaskCounts(ctx context.Context) (map[string]int, error) {
	var rows []struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("status", "COUNT(*) AS count").
		WhereNull("deleted_at").
		Group("status").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计后台任务状态失败")
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Status] = row.Count
	}
	return counts, nil
}

func (s *sSysPublish) serverDashboardBasicCounts(ctx context.Context) (map[string]int, error) {
	tables := map[string]string{
		"tenants":  publishTenantTable,
		"accounts": publishAccountTable,
		"tasks":    publishTaskTable,
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

func (s *sSysPublish) serverDashboardTaskTrend(ctx context.Context, days int) ([]*sysin.ServerDashboardTrendPoint, error) {
	start := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")
	var rows []struct {
		Date   string `json:"date"`
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("DATE(created_at) AS date", "status", "COUNT(*) AS count").
		WhereGTE("created_at", start+" 00:00:00").
		WhereNull("deleted_at").
		Group("DATE(created_at),status").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计后台任务趋势失败")
	}
	points := make([]*sysin.ServerDashboardTrendPoint, 0, days)
	index := make(map[string]*sysin.ServerDashboardTrendPoint, days)
	for i := days - 1; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		point := &sysin.ServerDashboardTrendPoint{Date: date}
		index[date] = point
		points = append(points, point)
	}
	for _, row := range rows {
		point := index[row.Date]
		if point == nil {
			continue
		}
		point.Created += row.Count
		switch row.Status {
		case sysin.PublishTaskStatusPublished:
			point.Published += row.Count
		case sysin.PublishTaskStatusFailed:
			point.Failed += row.Count
		case sysin.PublishTaskStatusCanceled:
			point.Canceled += row.Count
		}
	}
	return points, nil
}

func (s *sSysPublish) serverDashboardTodos(ctx context.Context) ([]*sysin.ServerDashboardTodo, error) {
	var rows []*sysin.TaskModel
	err := s.taskListModel(ctx).
		WhereIn("t.status", []string{sysin.PublishTaskStatusFailed, sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing}).
		WhereNull("t.deleted_at").
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

func (s *sSysPublish) serverDashboardTenantRank(ctx context.Context) ([]*sysin.ServerDashboardRank, error) {
	var rows []struct {
		TenantId int64  `json:"tenantId"`
		Name     string `json:"name"`
		Count    int    `json:"count"`
	}
	err := g.DB().Model(publishTaskTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishTenantTable+" m", "m.id=t.tenant_id").
		Fields("t.tenant_id,m.name,COUNT(*) AS count").
		Where("t.status", sysin.PublishTaskStatusPublished).
		WhereNull("t.deleted_at").
		Group("t.tenant_id,m.name").
		OrderDesc("count").
		Limit(8).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计账号归属排行失败")
	}
	list := make([]*sysin.ServerDashboardRank, 0, len(rows))
	for _, row := range rows {
		list = append(list, &sysin.ServerDashboardRank{
			Key:    fmt.Sprintf("tenant-%d", row.TenantId),
			Name:   dashboardText(row.Name, fmt.Sprintf("归属 %d", row.TenantId)),
			Value:  row.Count,
			Desc:   "发布成功",
			Status: "success",
		})
	}
	return list, nil
}

func (s *sSysPublish) serverDashboardErrorRank(ctx context.Context) ([]*sysin.ServerDashboardRank, error) {
	var rows []struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	err := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).
		Fields("message", "COUNT(*) AS count").
		Where("status", sysin.PublishTaskStatusFailed).
		WhereNot("message", "").
		Group("message").
		OrderDesc("count").
		Limit(8).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计后台失败原因失败")
	}
	list := make([]*sysin.ServerDashboardRank, 0, len(rows))
	for index, row := range rows {
		list = append(list, &sysin.ServerDashboardRank{
			Key:    fmt.Sprintf("error-%d", index),
			Name:   dashboardTrim(row.Message, 28),
			Value:  row.Count,
			Desc:   "失败次数",
			Status: "error",
		})
	}
	return list, nil
}

func serverDashboardTodoDesc(row *sysin.TaskModel) string {
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
