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
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
)

const tenantFeaturePermissionTable = "hg_youban_publish_tenant_feature_permission"

var tenantFeaturePermissionOptions = []*sysin.TenantFeaturePermissionItem{
	{
		Code:        sysin.TenantVipFeatureTextObfuscation,
		Name:        "文本混淆",
		Description: "允许用户端配置频道文本混淆；仍需有效 VIP 才会实际生效。",
	},
}

func (s *sSysPublish) AdminTenantFeaturePermissionView(ctx context.Context, in *sysin.TenantFeaturePermissionViewInp) (*sysin.TenantFeaturePermissionViewModel, error) {
	if in == nil || in.TenantId <= 0 {
		return nil, gerror.New("请选择账号归属")
	}
	enabled, err := s.tenantFeaturePermissions(ctx, in.TenantId)
	if err != nil {
		return nil, err
	}
	list := make([]*sysin.TenantFeaturePermissionItem, 0, len(tenantFeaturePermissionOptions))
	for _, option := range tenantFeaturePermissionOptions {
		item := *option
		item.Enabled = enabled[item.Code]
		list = append(list, &item)
	}
	return &sysin.TenantFeaturePermissionViewModel{TenantId: in.TenantId, List: list}, nil
}

func (s *sSysPublish) AdminTenantFeaturePermissionSave(ctx context.Context, in *sysin.TenantFeaturePermissionSaveInp) error {
	if in == nil || in.TenantId <= 0 {
		return gerror.New("请选择账号归属")
	}
	if err := ensureTenantFeaturePermissionTable(ctx); err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(tenantFeaturePermissionOptions))
	for _, option := range tenantFeaturePermissionOptions {
		allowed[option.Code] = struct{}{}
	}
	enabled := make(map[string]struct{}, len(in.Features))
	for _, feature := range in.Features {
		feature = strings.TrimSpace(feature)
		if _, ok := allowed[feature]; ok {
			enabled[feature] = struct{}{}
		}
	}
	now := gtime.Now()
	for code := range allowed {
		status := consts.StatusDisable
		if _, ok := enabled[code]; ok {
			status = consts.StatusEnabled
		}
		_, err := g.DB().Model(tenantFeaturePermissionTable).Safe().Ctx(ctx).Data(g.Map{
			"tenant_id":    in.TenantId,
			"feature_code": code,
			"status":       status,
			"created_at":   now,
			"updated_at":   now,
		}).OnConflict("tenant_id,feature_code").OnDuplicate("status,updated_at").Save()
		if err != nil {
			return gerror.Wrap(err, "保存账号归属功能权限失败")
		}
	}
	_, _ = cache.Instance().Remove(ctx, tenantFeaturePermissionCacheKey(in.TenantId))
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(in.TenantId))
	return nil
}

func (s *sSysPublish) tenantFeaturePermissions(ctx context.Context, tenantId int64) (map[string]bool, error) {
	result := make(map[string]bool)
	if tenantId <= 0 {
		return result, nil
	}
	key := tenantFeaturePermissionCacheKey(tenantId)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		if scanErr := value.Scan(&result); scanErr == nil {
			return result, nil
		}
	}
	if err := ensureTenantFeaturePermissionTable(ctx); err != nil {
		return nil, err
	}
	var rows []struct {
		FeatureCode string `json:"featureCode" orm:"feature_code"`
		Status      int    `json:"status"`
	}
	if err := g.DB().Model(tenantFeaturePermissionTable).Safe().Ctx(ctx).
		Fields("feature_code,status").Where("tenant_id", tenantId).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取账号归属功能权限失败")
	}
	for _, row := range rows {
		result[row.FeatureCode] = row.Status == consts.StatusEnabled
	}
	_ = cache.Instance().Set(ctx, key, result, 10*time.Minute)
	return result, nil
}

func tenantFeaturePermissionCacheKey(tenantId int64) string {
	return fmt.Sprintf("youban_publish:tenant_features:%d", tenantId)
}

func ensureTenantFeaturePermissionTable(ctx context.Context) error {
	var statement string
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		statement = `CREATE TABLE IF NOT EXISTS "hg_youban_publish_tenant_feature_permission" ("id" BIGSERIAL PRIMARY KEY,"tenant_id" bigint NOT NULL DEFAULT 0,"feature_code" varchar(64) NOT NULL DEFAULT '',"status" smallint NOT NULL DEFAULT 2,"created_at" timestamp DEFAULT NULL,"updated_at" timestamp DEFAULT NULL,UNIQUE("tenant_id","feature_code"))`
	} else {
		statement = "CREATE TABLE IF NOT EXISTS `hg_youban_publish_tenant_feature_permission` (`id` bigint unsigned NOT NULL AUTO_INCREMENT,`tenant_id` bigint NOT NULL DEFAULT '0',`feature_code` varchar(64) NOT NULL DEFAULT '',`status` tinyint NOT NULL DEFAULT '2',`created_at` datetime DEFAULT NULL,`updated_at` datetime DEFAULT NULL,PRIMARY KEY (`id`),UNIQUE KEY `uk_ybp_tenant_feature` (`tenant_id`,`feature_code`)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	}
	_, err := g.DB().Exec(ctx, statement)
	if err != nil {
		return gerror.Wrap(err, "初始化账号归属功能权限表失败")
	}
	return nil
}
