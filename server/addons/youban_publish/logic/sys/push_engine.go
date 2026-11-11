package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
)

const pushEngineCacheTTL = 5 * time.Minute

// pushEngineFallbackCacheTTL 用于数据库字段暂不可用时抑制重复查询。
const pushEngineFallbackCacheTTL = time.Minute

// pushEngineCacheKey 返回账户推送引擎配置缓存键。
func pushEngineCacheKey(tenantId, accountId int64) string {
	return fmt.Sprintf(publishconsts.PushEngineCacheKeyPrefix+"%d:%d", tenantId, accountId)
}

const (
	// pushEngineLegacy 保持现网调度实现。
	pushEngineLegacy = "legacy"
	// pushEngineUnified 启用统一频道调度实现。
	pushEngineUnified = "unified"
)

// accountPushEngine 返回账户当前启用的推送引擎；配置缺失或非法时回退到旧实现。
func (s *sSysPublish) accountPushEngine(ctx context.Context, tenantId, accountId int64) string {
	// 全量灰度开关：用于本地验证和线上逐步切换，优先级高于账户字段。
	if strings.EqualFold(strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_PUSH_ENGINE")), pushEngineUnifiedStream) {
		return pushEngineUnifiedStream
	}
	if tenantId <= 0 || accountId <= 0 {
		return pushEngineLegacy
	}
	key := pushEngineCacheKey(tenantId, accountId)
	if cache.Initialized() {
		if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
			engine := strings.ToLower(strings.TrimSpace(value.String()))
			if engine == pushEngineUnified || engine == pushEngineUnifiedStream {
				return engine
			}
			return pushEngineLegacy
		}
	}
	var row struct {
		PushEngine string `orm:"push_engine"`
	}
	err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
		Fields("push_engine").Where("tenant_id", tenantId).
		Where("account_id", accountId).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		// 迁移尚未执行或旧测试表没有新字段时，必须保持旧行为。
		if cache.Initialized() {
			_ = cache.Instance().Set(ctx, key, pushEngineLegacy, pushEngineFallbackCacheTTL)
		}
		return pushEngineLegacy
	}
	engine := strings.ToLower(strings.TrimSpace(row.PushEngine))
	if engine != pushEngineUnified && engine != pushEngineUnifiedStream {
		if cache.Initialized() {
			_ = cache.Instance().Set(ctx, key, pushEngineLegacy, pushEngineCacheTTL)
		}
		return pushEngineLegacy
	}
	if cache.Initialized() {
		_ = cache.Instance().Set(ctx, key, engine, pushEngineCacheTTL)
	}
	return engine
}

// clearPushEngineCache 清理账户推送引擎缓存，使灰度配置即时生效。
func clearPushEngineCache(ctx context.Context, tenantId, accountId int64) {
	if tenantId <= 0 || accountId <= 0 || !cache.Initialized() {
		return
	}
	_, _ = cache.Instance().Remove(ctx, pushEngineCacheKey(tenantId, accountId))
}

// unifiedPushEngineEnabled 判断账户是否启用了统一推送引擎。
func (s *sSysPublish) unifiedPushEngineEnabled(ctx context.Context, tenantId, accountId int64) bool {
	engine := s.accountPushEngine(ctx, tenantId, accountId)
	return engine == pushEngineUnified || engine == pushEngineUnifiedStream
}

// globalStreamPushEngineEnabled 判断是否开启了全量 Redis Stream 灰度。
// 全量灰度时 Outbox 只负责发布就绪事件，不再执行旧的频道队首 SQL。
func globalStreamPushEngineEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_PUSH_ENGINE")), pushEngineUnifiedStream)
}
