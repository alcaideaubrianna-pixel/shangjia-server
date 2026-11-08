package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/library/cache"
)

const profileCycleEngineProfile = "profile"
const profileCycleEngineLegacy = "legacy"
const profileCycleEngineCacheTTL = 5 * time.Minute

func profileCycleEngineCacheKey(tenantID, accountID int64) string {
	return "youban_publish:cycle_engine:" + gvarInt64(tenantID) + ":" + gvarInt64(accountID)
}

// clearProfileCycleEngineCache 清理账户循环引擎缓存，使后台配置修改立即对调度器生效。
func clearProfileCycleEngineCache(ctx context.Context, tenantID, accountID int64) {
	if tenantID <= 0 || accountID <= 0 || !cache.Initialized() {
		return
	}
	if _, err := cache.Instance().Remove(ctx, profileCycleEngineCacheKey(tenantID, accountID)); err != nil {
		g.Log().Warningf(ctx, "清理账户循环引擎缓存失败 tenant:%d account:%d err:%+v", tenantID, accountID, err)
	}
}

// accountCycleEngine 读取账户循环调度引擎；迁移字段缺失或配置异常时回退旧逻辑。
func (s *sSysPublish) accountCycleEngine(ctx context.Context, tenantID, accountID int64) string {
	if tenantID <= 0 || accountID <= 0 {
		return profileCycleEngineLegacy
	}
	key := profileCycleEngineCacheKey(tenantID, accountID)
	if cache.Initialized() {
		if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
			if strings.EqualFold(strings.TrimSpace(value.String()), profileCycleEngineProfile) {
				return profileCycleEngineProfile
			}
			return profileCycleEngineLegacy
		}
	}
	var row struct {
		CycleEngine string `orm:"cycle_engine"`
	}
	err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
		Fields("cycle_engine").Where("tenant_id", tenantID).Where("account_id", accountID).
		WhereNull("deleted_at").Scan(&row)
	engine := profileCycleEngineLegacy
	if err == nil && strings.EqualFold(strings.TrimSpace(row.CycleEngine), profileCycleEngineProfile) {
		engine = profileCycleEngineProfile
	}
	if cache.Initialized() {
		_ = cache.Instance().Set(ctx, key, engine, profileCycleEngineCacheTTL)
	}
	return engine
}

func gvarInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
