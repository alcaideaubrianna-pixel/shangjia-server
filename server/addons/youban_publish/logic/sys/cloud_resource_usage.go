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
	cloudResourceUsageTable         = "hg_youban_publish_cloud_resource_usage"
	cloudResourceUsageScenePreview  = "preview"
	cloudResourceUsageSceneValidate = "config_validation"
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
	now := gtime.Now()
	successCount := 0
	failureCount := 1
	if event.Success {
		successCount = 1
		failureCount = 0
	}
	args := []interface{}{
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
	}
	var err error
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err = g.DB().Exec(ctx, `
INSERT INTO "hg_youban_publish_cloud_resource_usage"
("tenant_id","account_id","resource_type","scene","usage_date","request_count","success_count","failure_count","total_duration_ms","last_called_at","created_at","updated_at")
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT ("tenant_id","account_id","resource_type","scene","usage_date") DO UPDATE SET
"request_count"="hg_youban_publish_cloud_resource_usage"."request_count"+EXCLUDED."request_count",
"success_count"="hg_youban_publish_cloud_resource_usage"."success_count"+EXCLUDED."success_count",
"failure_count"="hg_youban_publish_cloud_resource_usage"."failure_count"+EXCLUDED."failure_count",
"total_duration_ms"="hg_youban_publish_cloud_resource_usage"."total_duration_ms"+EXCLUDED."total_duration_ms",
"last_called_at"=EXCLUDED."last_called_at",
"updated_at"=EXCLUDED."updated_at"`, args...)
	} else {
		_, err = g.DB().Exec(ctx, `
INSERT INTO `+"`"+cloudResourceUsageTable+"`"+`
(`+"`tenant_id`,`account_id`,`resource_type`,`scene`,`usage_date`,`request_count`,`success_count`,`failure_count`,`total_duration_ms`,`last_called_at`,`created_at`,`updated_at`"+`)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
`+"`request_count`=`request_count`+VALUES(`request_count`),"+`
`+"`success_count`=`success_count`+VALUES(`success_count`),"+`
`+"`failure_count`=`failure_count`+VALUES(`failure_count`),"+`
`+"`total_duration_ms`=`total_duration_ms`+VALUES(`total_duration_ms`),"+`
`+"`last_called_at`=VALUES(`last_called_at`),`updated_at`=VALUES(`updated_at`)"+``, args...)
	}
	if err != nil {
		g.Log().Warningf(ctx, "记录云资源调用统计失败 accountId:%d resource:%s scene:%s err:%+v", event.AccountId, event.ResourceType, event.Scene, err)
	}
}

func (s *sSysConfig) CloudResourceUsageList(ctx context.Context, in *sysin.CloudResourceUsageListInp) (list []*sysin.CloudResourceUsageModel, totalCount int, summary *sysin.CloudResourceUsageSummaryModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, 0, nil, err
	}
	baseModel := cloudResourceUsageQuery(ctx, in)
	totalRow, err := baseModel.Clone().Fields("COUNT(DISTINCT u.account_id) AS total_count").One()
	if err != nil {
		return nil, 0, nil, gerror.Wrap(err, "统计云资源调用用户数失败")
	}
	totalCount = totalRow["total_count"].Int()
	summary = &sysin.CloudResourceUsageSummaryModel{}
	if err = baseModel.Clone().Fields(cloudResourceUsageSummaryFields()).Scan(summary); err != nil {
		return nil, 0, nil, gerror.Wrap(err, "读取云资源调用汇总失败")
	}
	fillCloudResourceUsageSummary(summary)
	page, pageSize, offset := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = pageSize
	query := baseModel.Clone().Fields(cloudResourceUsageListFields()).Group("u.account_id").OrderDesc("request_count")
	if in.Pagination {
		query = query.Limit(offset, pageSize)
	}
	if err = query.Scan(&list); err != nil {
		return nil, 0, nil, gerror.Wrap(err, "读取云资源调用统计失败")
	}
	for _, item := range list {
		fillCloudResourceUsageModel(item)
	}
	return list, totalCount, summary, nil
}

func cloudResourceUsageQuery(ctx context.Context, in *sysin.CloudResourceUsageListInp) *gdb.Model {
	accountTable := pdao.YoubanPublishAccount.Table()
	tenantTable := pdao.YoubanPublishTenant.Table()
	vipTable := pdao.YoubanPublishTenantVip.Table()
	model := g.DB().Model(cloudResourceUsageTable+" u").Safe().Ctx(ctx).
		LeftJoin(accountTable+" a", "a.id=u.account_id").
		LeftJoin(tenantTable+" t", "t.id=u.tenant_id").
		LeftJoin(vipTable+" v", "v.tenant_id=u.tenant_id AND v.deleted_at IS NULL").
		WhereGTE("u.usage_date", in.StartDate).
		WhereLTE("u.usage_date", in.EndDate)
	if in.ResourceType != "" {
		model = model.Where("u.resource_type", in.ResourceType)
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
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
	return `COUNT(DISTINCT CASE WHEN u.account_id > 0 THEN u.account_id END) AS active_user_count,
COALESCE(SUM(u.request_count), 0) AS request_count,
COALESCE(SUM(u.success_count), 0) AS success_count,
COALESCE(SUM(u.failure_count), 0) AS failure_count,
COALESCE(SUM(CASE WHEN u.resource_type='background_matting' THEN u.request_count ELSE 0 END), 0) AS background_matting_count,
COALESCE(SUM(CASE WHEN u.resource_type='face_detection' THEN u.request_count ELSE 0 END), 0) AS face_detection_count,
COALESCE(SUM(CASE WHEN u.scene='config_validation' THEN u.request_count ELSE 0 END), 0) AS validation_count,
COALESCE(SUM(u.total_duration_ms), 0) AS total_duration_ms`
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
