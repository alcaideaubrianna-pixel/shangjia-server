package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	// pushEngineLegacy 保持现网调度实现。
	pushEngineLegacy = "legacy"
	// pushEngineUnified 启用统一频道调度实现。
	pushEngineUnified = "unified"
)

// accountPushEngine 返回账户当前启用的推送引擎；配置缺失或非法时回退到旧实现。
func (s *sSysPublish) accountPushEngine(ctx context.Context, tenantId, accountId int64) string {
	if tenantId <= 0 || accountId <= 0 {
		return pushEngineLegacy
	}
	var row struct {
		PushEngine string `orm:"push_engine"`
	}
	err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
		Fields("push_engine").Where("tenant_id", tenantId).
		Where("account_id", accountId).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		// 迁移尚未执行或旧测试表没有新字段时，必须保持旧行为。
		return pushEngineLegacy
	}
	engine := strings.ToLower(strings.TrimSpace(row.PushEngine))
	if engine != pushEngineUnified {
		return pushEngineLegacy
	}
	return engine
}

// unifiedPushEngineEnabled 判断账户是否启用了统一推送引擎。
func (s *sSysPublish) unifiedPushEngineEnabled(ctx context.Context, tenantId, accountId int64) bool {
	return s.accountPushEngine(ctx, tenantId, accountId) == pushEngineUnified
}
