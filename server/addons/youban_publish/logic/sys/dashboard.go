package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const dashboardTodoLimit = 6

func (s *sSysPublish) AdminDashboardOverview(ctx context.Context) (*sysin.DashboardOverviewModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardOverview(ctx, account.TenantId, 0, true)
}

func (s *sSysPublish) MyDashboardOverview(ctx context.Context) (*sysin.DashboardOverviewModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardOverview(ctx, account.TenantId, account.Id, false)
}

func (s *sSysPublish) AdminDashboardTrend(ctx context.Context, in *sysin.TrendInp) (*sysin.DashboardTrendModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardTrend(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) MyDashboardTrend(ctx context.Context, in *sysin.TrendInp) (*sysin.DashboardTrendModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardTrend(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminDashboardTodo(ctx context.Context) (*sysin.DashboardTodoModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardTodo(ctx, account.TenantId, 0, true)
}

func (s *sSysPublish) MyDashboardTodo(ctx context.Context) (*sysin.DashboardTodoModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardTodo(ctx, account.TenantId, account.Id, false)
}

func (s *sSysPublish) AdminDashboardRank(ctx context.Context) (*sysin.DashboardRankModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.dashboardRank(ctx, account.TenantId)
}

func (s *sSysPublish) dashboardOverview(ctx context.Context, tenantId int64, accountId int64, admin bool) (*sysin.DashboardOverviewModel, error) {
	// 首屏只做轻量聚合，避免工作台打开时扫描大列表。
	profile, err := s.profileStats(ctx, &sysin.TrendInp{Days: 7}, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	counts, err := s.dashboardTaskCounts(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	tgOnline, tgTotal, err := s.dashboardTgAccountCounts(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	channels, err := s.dashboardSimpleCount(ctx, publishChannelTable, tenantId, 0)
	if err != nil {
		return nil, err
	}
	accounts := 0
	if admin {
		accounts, err = s.dashboardSimpleCount(ctx, publishAccountTable, tenantId, 0)
		if err != nil {
			return nil, err
		}
	}
	stats := dashboardStats(profile, counts, tgOnline, channels, accounts, admin)
	return &sysin.DashboardOverviewModel{
		Stats:      stats,
		Health:     dashboardHealth(tgOnline, tgTotal, counts[sysin.PublishTaskStatusFailed], channels),
		QuickLinks: dashboardQuickLinks(counts, admin),
		Profile:    profile,
	}, nil
}

func dashboardStats(profile *sysin.ProfileStatsModel, counts map[string]int, tgOnline int, channels int, accounts int, admin bool) []*sysin.DashboardStatModel {
	stats := []*sysin.DashboardStatModel{
		{Key: "profiles", Title: "资料总数", Value: profile.Total},
		{Key: "published", Title: "已上架", Value: profile.UpCount},
		{Key: "pending", Title: "待上架事件", Value: counts[sysin.PublishTaskStatusPending]},
		{Key: "failed", Title: "发布失败", Value: counts[sysin.PublishTaskStatusFailed]},
	}
	if admin {
		stats = append(stats,
			&sysin.DashboardStatModel{Key: "accounts", Title: "上架账号", Value: accounts},
			&sysin.DashboardStatModel{Key: "tgOnline", Title: "在线协议号", Value: tgOnline},
			&sysin.DashboardStatModel{Key: "channels", Title: "可用频道", Value: channels},
			&sysin.DashboardStatModel{Key: "successRate", Title: "发布成功率", Value: int(dashboardSuccessRate(counts[sysin.PublishTaskStatusPublished], counts[sysin.PublishTaskStatusFailed])), Suffix: "%"},
		)
	}
	return stats
}

func dashboardSuccessRate(success int, failed int) float64 {
	total := success + failed
	if total == 0 {
		return 0
	}
	return float64(success*1000/total) / 10
}

func (s *sSysPublish) dashboardTodo(ctx context.Context, tenantId int64, accountId int64, admin bool) (*sysin.DashboardTodoModel, error) {
	// 待办只返回少量记录，完整处理仍跳转到对应业务列表。
	items, err := s.dashboardTaskTodos(ctx, tenantId, accountId, admin)
	if err != nil {
		return nil, err
	}
	if admin {
		tgItems, err := s.dashboardTgTodos(ctx, tenantId)
		if err != nil {
			return nil, err
		}
		items = append(items, tgItems...)
	}
	if len(items) > dashboardTodoLimit {
		items = items[:dashboardTodoLimit]
	}
	return &sysin.DashboardTodoModel{Items: items}, nil
}

func (s *sSysPublish) dashboardTaskTodos(ctx context.Context, tenantId int64, accountId int64, admin bool) ([]*sysin.DashboardTodoItemModel, error) {
	var rows []*sysin.TaskModel
	mod := s.taskListModel(ctx).
		Where("t.tenant_id", tenantId).
		WhereIn("t.status", []string{sysin.PublishTaskStatusFailed, sysin.PublishTaskStatusPending}).
		WhereNull("t.deleted_at")
	if accountId > 0 {
		mod = mod.Where("t.account_id", accountId)
	}
	if err := mod.Limit(dashboardTodoLimit).OrderDesc("t.updated_at").OrderDesc("t.id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取工作台待办失败")
	}
	items := make([]*sysin.DashboardTodoItemModel, 0, len(rows))
	for _, row := range rows {
		path := "/collector/upload"
		if admin {
			path = "/admin/notes"
		}
		items = append(items, &sysin.DashboardTodoItemModel{
			Key:       fmt.Sprintf("task-%d", row.Id),
			Type:      "task",
			Title:     dashboardText(row.Title, "未命名资料"),
			Desc:      dashboardTaskDesc(row),
			Status:    row.Status,
			Path:      path,
			UpdatedAt: dashboardTime(row.UpdatedAt),
		})
	}
	return items, nil
}

func (s *sSysPublish) dashboardTgTodos(ctx context.Context, tenantId int64) ([]*sysin.DashboardTodoItemModel, error) {
	var rows []struct {
		Id          int64  `json:"id"`
		DisplayName string `json:"displayName"`
		Phone       string `json:"telegramPhone"`
		Status      string `json:"status"`
		UpdatedAt   string `json:"updatedAt"`
	}
	err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id,display_name,telegram_phone,status,updated_at").
		Where("tenant_id", tenantId).
		WhereIn("status", []string{sysin.PublishTgAccountStatusExpired, sysin.PublishTgAccountStatusFailed, sysin.PublishTgAccountStatusPassword}).
		WhereNull("deleted_at").
		Limit(3).
		OrderDesc("updated_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "获取协议号待办失败")
	}
	items := make([]*sysin.DashboardTodoItemModel, 0, len(rows))
	for _, row := range rows {
		items = append(items, &sysin.DashboardTodoItemModel{
			Key:       fmt.Sprintf("tg-%d", row.Id),
			Type:      "tg_account",
			Title:     dashboardText(row.DisplayName, row.Phone),
			Desc:      "协议号需要重新登录或检查权限",
			Status:    row.Status,
			Path:      "/admin/accounts/protocol",
			UpdatedAt: row.UpdatedAt,
		})
	}
	return items, nil
}

func (s *sSysPublish) dashboardRank(ctx context.Context, tenantId int64) (*sysin.DashboardRankModel, error) {
	publishers, err := s.dashboardPublisherRank(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	channels, err := s.dashboardChannelRank(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	errors, err := s.dashboardErrorRank(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	return &sysin.DashboardRankModel{Publishers: publishers, Channels: channels, Errors: errors}, nil
}

func dashboardHealth(tgOnline int, tgTotal int, failed int, channels int) []*sysin.DashboardHealthModel {
	tgStatus := "success"
	if tgTotal == 0 || tgOnline == 0 {
		tgStatus = "error"
	}
	failedStatus := "success"
	if failed > 0 {
		failedStatus = "warning"
	}
	channelStatus := "success"
	if channels == 0 {
		channelStatus = "error"
	}
	return []*sysin.DashboardHealthModel{
		{Key: "tg", Title: "协议号", Status: tgStatus, Value: fmt.Sprintf("%d/%d 在线", tgOnline, tgTotal), Message: "用于 Telegram 发布链路"},
		{Key: "queue", Title: "发布队列", Status: failedStatus, Value: fmt.Sprintf("%d 个失败", failed), Message: "失败任务需要重试或排查"},
		{Key: "channel", Title: "频道配置", Status: channelStatus, Value: fmt.Sprintf("%d 个频道", channels), Message: "频道用于资料分发"},
	}
}

func dashboardQuickLinks(counts map[string]int, admin bool) []*sysin.DashboardQuickLinkModel {
	if admin {
		return []*sysin.DashboardQuickLinkModel{
			{Key: "notes", Title: "资料列表", Path: "/admin/notes", Badge: counts[sysin.PublishTaskStatusPending]},
			{Key: "records", Title: "发布记录", Path: "/admin/logs", Badge: counts[sysin.PublishTaskStatusFailed]},
			{Key: "channels", Title: "频道配置", Path: "/admin/channels"},
			{Key: "accounts", Title: "协议号", Path: "/admin/accounts/protocol"},
		}
	}
	return []*sysin.DashboardQuickLinkModel{
		{Key: "upload", Title: "上传资料", Path: "/collector/upload"},
		{Key: "content", Title: "我的资料", Path: "/collector/content", Badge: counts[sysin.PublishTaskStatusPending]},
		{Key: "records", Title: "发布记录", Path: "/collector/records", Badge: counts[sysin.PublishTaskStatusFailed]},
	}
}

func (s *sSysPublish) dashboardPublisherRank(ctx context.Context, tenantId int64) ([]*sysin.DashboardRankItemModel, error) {
	var rows []struct {
		AccountId int64  `json:"accountId"`
		Nickname  string `json:"nickname"`
		Username  string `json:"username"`
		Count     int    `json:"count"`
	}
	err := g.DB().Model(publishTaskTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		Fields("t.account_id,a.nickname,a.username,COUNT(*) AS count").
		Where("t.tenant_id", tenantId).
		Where("t.status", sysin.PublishTaskStatusPublished).
		WhereNull("t.deleted_at").
		Group("t.account_id,a.nickname,a.username").
		OrderDesc("count").
		Limit(5).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计上架账号排行失败")
	}
	items := make([]*sysin.DashboardRankItemModel, 0, len(rows))
	for _, row := range rows {
		items = append(items, &sysin.DashboardRankItemModel{
			Key:    fmt.Sprintf("publisher-%d", row.AccountId),
			Name:   dashboardText(row.Nickname, row.Username),
			Value:  row.Count,
			Desc:   "已发布资料",
			Status: "success",
		})
	}
	return items, nil
}

func (s *sSysPublish) dashboardChannelRank(ctx context.Context, tenantId int64) ([]*sysin.DashboardRankItemModel, error) {
	var rows []struct {
		ChannelId int64  `json:"channelId"`
		Name      string `json:"name"`
		Count     int    `json:"count"`
	}
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Fields("j.channel_id,c.channel_title AS name,COUNT(*) AS count").
		Where("j.tenant_id", tenantId).
		Where("j.status", sysin.PublishTaskStatusPublished).
		Group("j.channel_id,c.channel_title").
		OrderDesc("count").
		Limit(5).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计频道排行失败")
	}
	items := make([]*sysin.DashboardRankItemModel, 0, len(rows))
	for _, row := range rows {
		items = append(items, &sysin.DashboardRankItemModel{
			Key:    fmt.Sprintf("channel-%d", row.ChannelId),
			Name:   dashboardText(row.Name, "未命名频道"),
			Value:  row.Count,
			Desc:   "发布成功",
			Status: "success",
		})
	}
	return items, nil
}

func (s *sSysPublish) dashboardErrorRank(ctx context.Context, tenantId int64) ([]*sysin.DashboardRankItemModel, error) {
	var rows []struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	err := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).
		Fields("message", "COUNT(*) AS count").
		Where("tenant_id", tenantId).
		Where("status", sysin.PublishTaskStatusFailed).
		WhereNot("message", "").
		Group("message").
		OrderDesc("count").
		Limit(5).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "统计失败原因失败")
	}
	items := make([]*sysin.DashboardRankItemModel, 0, len(rows))
	for index, row := range rows {
		items = append(items, &sysin.DashboardRankItemModel{
			Key:    fmt.Sprintf("error-%d", index),
			Name:   dashboardTrim(row.Message, 24),
			Value:  row.Count,
			Desc:   "失败次数",
			Status: "error",
		})
	}
	return items, nil
}

func dashboardTaskDesc(row *sysin.TaskModel) string {
	owner := dashboardText(row.AccountNickname, row.AccountUsername)
	if owner == "" {
		return row.City
	}
	if row.City == "" {
		return owner
	}
	return owner + " · " + row.City
}

func dashboardTime(t interface{}) string {
	if t == nil {
		return ""
	}
	return fmt.Sprint(t)
}

func dashboardText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func dashboardTrim(text string, max int) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "..."
}
