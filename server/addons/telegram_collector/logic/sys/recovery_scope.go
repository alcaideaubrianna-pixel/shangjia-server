package sys

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/internal/library/cache"
)

var errCollectorRecoveryRedisUnavailable = errors.New("采集恢复 Redis 不可用，停止本轮数据库恢复")

// Zero account/source IDs are valid for raw Bot updates before source resolution.
type collectorRecoveryScope struct{ TenantID, AccountID, SourceID int64 }

func (s collectorRecoveryScope) valid() bool {
	return s.TenantID > 0 && s.AccountID >= 0 && s.SourceID >= 0
}
func (s collectorRecoveryScope) key() string {
	return fmt.Sprintf("%d:%d:%d", s.TenantID, s.AccountID, s.SourceID)
}
func parseCollectorRecoveryScope(member string) (collectorRecoveryScope, error) {
	parts := strings.Split(member, ":")
	if len(parts) != 3 {
		return collectorRecoveryScope{}, gerror.New("采集恢复 scope 格式无效")
	}
	values := make([]int64, 3)
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return collectorRecoveryScope{}, err
		}
		values[i] = value
	}
	scope := collectorRecoveryScope{values[0], values[1], values[2]}
	if !scope.valid() {
		return scope, gerror.New("采集恢复 scope 无效")
	}
	return scope, nil
}
func markCollectorRecoveryScope(ctx context.Context, scope collectorRecoveryScope) error {
	if !scope.valid() {
		return gerror.New("采集恢复 scope 无效")
	}
	return cache.SetAdd(ctx, consts.RecoveryScopeKey, scope.key())
}
func collectorRecoveryScopes(ctx context.Context, limit int) ([]collectorRecoveryScope, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	members, err := cache.SetRotate(ctx, consts.RecoveryScopeKey, limit)
	if err != nil {
		return nil, err
	}
	var scopes []collectorRecoveryScope
	for _, member := range members {
		scope, err := parseCollectorRecoveryScope(member)
		if err == nil {
			scopes = append(scopes, scope)
		} else if removeErr := cache.SetRemove(ctx, consts.RecoveryScopeKey, member); removeErr != nil {
			return nil, removeErr
		}
	}
	return scopes, nil
}
