package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/model/input/form"
)

const (
	cloudResourceUsageTable           = "hg_youban_publish_cloud_resource_usage"
	cloudResourceUsageScenePreview    = "preview"
	cloudResourceUsageSceneValidate   = "config_validation"
	cloudResourceUsageRollupTenantId  = int64(-1)
	cloudResourceUsageRollupAccountId = int64(-1)
)

type cloudResourceUsageOwner struct {
	TenantId  int64
	AccountId int64
}

type cloudResourceUsageEvent struct {
	cloudResourceUsageOwner
	ResourceType string
	Scene        string
	Success      bool
	Duration     time.Duration
}

func recordCloudResourceUsage(ctx context.Context, event cloudResourceUsageEvent) {
	if strings.TrimSpace(event.ResourceType) == "" {
		return
	}
	rollupEvent := event
	rollupEvent.TenantId = cloudResourceUsageRollupTenantId
	rollupEvent.AccountId = cloudResourceUsageRollupAccountId
	if err := upsertCloudResourceUsage(ctx, event, rollupEvent); err != nil {
		g.Log().Warningf(ctx, "记录云资源调用统计失败 accountId:%d resource:%s scene:%s err:%+v", event.AccountId, event.ResourceType, event.Scene, err)
	}
}

func upsertCloudResourceUsage(ctx context.Context, events ...cloudResourceUsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	now := gtime.Now()
	args := make([]interface{}, 0, len(events)*12)
	valueGroups := make([]string, 0, len(events))
	for _, event := range events {
		successCount := 0
		failureCount := 1
		if event.Success {
			successCount = 1
			failureCount = 0
		}
		valueGroups = append(valueGroups, "(?,?,?,?,?,?,?,?,?,?,?,?)")
		args = append(args,
			event.TenantId,
			event.AccountId,
			event.ResourceType,
			event.Scene,
			now.Format("Y-m-d"),
			1,
			successCount,
			failureCount,
			max(event.Duration.Milliseconds(), int64(0)),
			now,
			now,
			now,
		)
	}
	valuesSQL := strings.Join(valueGroups, ",")
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := g.DB().Exec(ctx, `
INSERT INTO "hg_youban_publish_cloud_resource_usage"
("tenant_id","account_id","resource_type","scene","usage_date","request_count","success_count","failure_count","total_duration_ms","last_called_at","created_at","updated_at")
VALUES `+valuesSQL+`
ON CONFLICT ("tenant_id","account_id","resource_type","scene","usage_date") DO UPDATE SET
"request_count"="hg_youban_publish_cloud_resource_usage"."request_count"+EXCLUDED."request_count",
"success_count"="hg_youban_publish_cloud_resource_usage"."success_count"+EXCLUDED."success_count",
"failure_count"="hg_youban_publish_cloud_resource_usage"."failure_count"+EXCLUDED."failure_count",
"total_duration_ms"="hg_youban_publish_cloud_resource_usage"."total_duration_ms"+EXCLUDED."total_duration_ms",
"last_called_at"=EXCLUDED."last_called_at",
"updated_at"=EXCLUDED."updated_at"`, args...)
		return err
	}
	_, err := g.DB().Exec(ctx, `
INSERT INTO `+"`"+cloudResourceUsageTable+"`"+`
(`+"`tenant_id`,`account_id`,`resource_type`,`scene`,`usage_date`,`request_count`,`success_count`,`failure_count`,`total_duration_ms`,`last_called_at`,`created_at`,`updated_at`"+`)
VALUES `+valuesSQL+`
ON DUPLICATE KEY UPDATE
`+"`request_count`=`request_count`+VALUES(`request_count`),"+`
`+"`success_count`=`success_count`+VALUES(`success_count`),"+`
`+"`failure_count`=`failure_count`+VALUES(`failure_count`),"+`
`+"`total_duration_ms`=`total_duration_ms`+VALUES(`total_duration_ms`),"+`
`+"`last_called_at`=VALUES(`last_called_at`),`updated_at`=VALUES(`updated_at`)"+``, args...)
	return err
}

func (s *sSysConfig) CloudResourceUsageDashboard(ctx context.Context, in *sysin.CloudResourceUsageDashboardInp) (res *sysin.CloudResourceUsageDashboardModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	rollupModel := cloudResourceUsageFilterQuery(ctx, &in.CloudResourceUsageQueryInp).
		Where("u.account_id", cloudResourceUsageRollupAccountId)
	summary := &sysin.CloudResourceUsageSummaryModel{}
	if err = rollupModel.Clone().Fields(cloudResourceUsageSummaryFields()).Scan(summary); err != nil {
		return nil, gerror.Wrap(err, "读取云资源大盘汇总失败")
	}
	activeUserRow, err := cloudResourceUsageFilterQuery(ctx, &in.CloudResourceUsageQueryInp).
		WhereGT("u.account_id", 0).
		Fields("COUNT(DISTINCT u.account_id) AS active_user_count").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "统计云资源调用用户数失败")
	}
	summary.ActiveUserCount = activeUserRow["active_user_count"].Int64()
	fillCloudResourceUsageSummary(summary)

	trend := make([]*sysin.CloudResourceUsageTrendModel, 0)
	if err = rollupModel.Clone().Fields(cloudResourceUsageTrendFields()).Group("u.usage_date").OrderAsc("u.usage_date").Scan(&trend); err != nil {
		return nil, gerror.Wrap(err, "读取云资源调用趋势失败")
	}
	for _, item := range trend {
		fillCloudResourceUsageTrend(item)
	}

	breakdown := make([]*sysin.CloudResourceUsageBreakdownModel, 0)
	if err = rollupModel.Clone().Fields(cloudResourceUsageBreakdownFields()).Group("u.resource_type").OrderDesc("request_count").Scan(&breakdown); err != nil {
		return nil, gerror.Wrap(err, "读取云资源类型分布失败")
	}
	for _, item := range breakdown {
		fillCloudResourceUsageBreakdown(item)
	}

	topUsers := make([]*sysin.CloudResourceUsageModel, 0, 10)
	if err = cloudResourceUsageAccountQuery(ctx, &in.CloudResourceUsageQueryInp, "", false).
		Fields(cloudResourceUsageListFields()).
		Group("u.account_id").
		OrderDesc("request_count").
		Limit(10).
		Scan(&topUsers); err != nil {
		return nil, gerror.Wrap(err, "读取云资源调用用户排行失败")
	}
	for _, item := range topUsers {
		fillCloudResourceUsageModel(item)
	}
	return &sysin.CloudResourceUsageDashboardModel{
		Summary:   summary,
		Trend:     trend,
		TopUsers:  topUsers,
		Breakdown: breakdown,
	}, nil
}

func (s *sSysConfig) CloudResourceUsageList(ctx context.Context, in *sysin.CloudResourceUsageListInp) (list []*sysin.CloudResourceUsageModel, totalCount int, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	baseModel := cloudResourceUsageAccountQuery(ctx, &in.CloudResourceUsageQueryInp, in.Keyword, true)
	totalRow, err := baseModel.Clone().Fields("COUNT(DISTINCT u.account_id) AS total_count").One()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计云资源调用明细用户数失败")
	}
	totalCount = totalRow["total_count"].Int()
	page, pageSize, offset := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = pageSize
	if err = baseModel.Clone().Fields(cloudResourceUsageListFields()).Group("u.account_id").OrderDesc("request_count").Limit(offset, pageSize).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "读取云资源调用明细失败")
	}
	for _, item := range list {
		fillCloudResourceUsageModel(item)
	}
	return list, totalCount, nil
}

func cloudResourceUsageFilterQuery(ctx context.Context, in *sysin.CloudResourceUsageQueryInp) *gdb.Model {
	model := g.DB().Model(cloudResourceUsageTable+" u").Safe().Ctx(ctx).
		WhereGTE("u.usage_date", in.StartDate).
		WhereLTE("u.usage_date", in.EndDate)
	if in.ResourceType != "" {
		model = model.Where("u.resource_type", in.ResourceType)
	}
	return model
}

func cloudResourceUsageAccountQuery(ctx context.Context, in *sysin.CloudResourceUsageQueryInp, keyword string, includeSystem bool) *gdb.Model {
	accountTable := pdao.YoubanPublishAccount.Table()
	tenantTable := pdao.YoubanPublishTenant.Table()
	vipTable := pdao.YoubanPublishTenantVip.Table()
	model := cloudResourceUsageFilterQuery(ctx, in).
		LeftJoin(accountTable+" a", "a.id=u.account_id").
		LeftJoin(tenantTable+" t", "t.id=u.tenant_id").
		LeftJoin(vipTable+" v", "v.tenant_id=u.tenant_id AND v.deleted_at IS NULL")
	if includeSystem {
		model = model.WhereGTE("u.account_id", 0)
	} else {
		model = model.WhereGT("u.account_id", 0)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		model = model.Where("(a.username LIKE ? OR a.nickname LIKE ? OR t.name LIKE ?)", like, like, like)
	}
	return model
}

func cloudResourceUsageListFields() string {
	return `u.account_id,
MAX(u.tenant_id) AS tenant_id,
COALESCE(MAX(a.username), '') AS username,
COALESCE(MAX(a.nickname), '') AS nickname,
COALESCE(MAX(t.name), '') AS tenant_name,
COALESCE(MAX(v.level), 0) AS vip_level,
COALESCE(MAX(v.status), 0) AS vip_status,
MAX(v.expired_at) AS vip_expired_at,
SUM(u.request_count) AS request_count,
SUM(u.success_count) AS success_count,
SUM(u.failure_count) AS failure_count,
SUM(CASE WHEN u.resource_type='background_matting' THEN u.request_count ELSE 0 END) AS background_matting_count,
SUM(CASE WHEN u.resource_type='face_detection' THEN u.request_count ELSE 0 END) AS face_detection_count,
SUM(CASE WHEN u.scene='config_validation' THEN u.request_count ELSE 0 END) AS validation_count,
SUM(u.total_duration_ms) AS total_duration_ms,
MIN(u.usage_date) AS first_usage_date,
MAX(u.last_called_at) AS last_called_at`
}

func cloudResourceUsageSummaryFields() string {
	return `COALESCE(SUM(u.request_count), 0) AS request_count,
COALESCE(SUM(u.success_count), 0) AS success_count,
COALESCE(SUM(u.failure_count), 0) AS failure_count,
COALESCE(SUM(CASE WHEN u.resource_type='background_matting' THEN u.request_count ELSE 0 END), 0) AS background_matting_count,
COALESCE(SUM(CASE WHEN u.resource_type='face_detection' THEN u.request_count ELSE 0 END), 0) AS face_detection_count,
COALESCE(SUM(CASE WHEN u.scene='config_validation' THEN u.request_count ELSE 0 END), 0) AS validation_count,
COALESCE(SUM(u.total_duration_ms), 0) AS total_duration_ms`
}

func cloudResourceUsageTrendFields() string {
	return `u.usage_date,
SUM(u.request_count) AS request_count,
SUM(u.success_count) AS success_count,
SUM(u.failure_count) AS failure_count,
SUM(u.total_duration_ms) AS total_duration_ms`
}

func cloudResourceUsageBreakdownFields() string {
	return `u.resource_type,
SUM(u.request_count) AS request_count,
SUM(u.success_count) AS success_count,
SUM(u.failure_count) AS failure_count,
SUM(u.total_duration_ms) AS total_duration_ms`
}

func fillCloudResourceUsageModel(item *sysin.CloudResourceUsageModel) {
	if item == nil {
		return
	}
	if item.AccountId == 0 {
		item.Username = "系统调用"
		item.Nickname = "配置验证"
		item.TenantName = "系统"
	}
	if item.RequestCount > 0 {
		item.AvgDurationMs = item.TotalDurationMs / item.RequestCount
	}
}

func fillCloudResourceUsageSummary(summary *sysin.CloudResourceUsageSummaryModel) {
	if summary != nil && summary.RequestCount > 0 {
		summary.AvgDurationMs = summary.TotalDurationMs / summary.RequestCount
	}
}

func fillCloudResourceUsageTrend(item *sysin.CloudResourceUsageTrendModel) {
	if item != nil && item.RequestCount > 0 {
		item.AvgDurationMs = item.TotalDurationMs / item.RequestCount
	}
}

func fillCloudResourceUsageBreakdown(item *sysin.CloudResourceUsageBreakdownModel) {
	if item != nil && item.RequestCount > 0 {
		item.AvgDurationMs = item.TotalDurationMs / item.RequestCount
	}
}
