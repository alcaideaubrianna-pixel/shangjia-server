// Package ingresspolicy moves only known rejected work off the realtime budget.
package ingresspolicy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gerror"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	publishconsts "hotgo/addons/youban_publish/consts"
)

type Store interface {
	Get(context.Context, any) (*gvar.Var, error)
	SetIfNotExist(context.Context, any, any, time.Duration) (bool, error)
}

func Version(ctx context.Context, store Store, versionKey string) (string, error) {
	v, err := store.Get(ctx, versionKey)
	if err != nil {
		return "", err
	}
	if v == nil || v.IsNil() {
		return "", nil
	}
	return v.String(), nil
}

func EnsureVersion(ctx context.Context, store Store, versionKey string) (string, error) {
	v, err := Version(ctx, store, versionKey)
	if err != nil || v != "" {
		return v, err
	}
	if _, err = store.SetIfNotExist(ctx, versionKey, strconv.FormatInt(time.Now().UnixNano(), 10), 24*time.Hour); err != nil {
		return "", err
	}
	return Version(ctx, store, versionKey)
}

func key(scope collectorin.BotIngressScope, version string) string {
	chat := strings.TrimPrefix(strconv.FormatInt(int64(scope.ChatID), 10), "-100")
	return fmt.Sprintf("%s%d:%d:%s:%s", publishconsts.CollectIngressDeferredKeyPrefix, scope.TenantID, scope.SourceID, chat, version)
}

func Lookup(ctx context.Context, store Store, versionKey string, scope collectorin.BotIngressScope) (collectorin.IngressDeferralReason, error) {
	if !scope.Valid() {
		return collectorin.IngressDeferralNone, nil
	}
	v, err := Version(ctx, store, versionKey)
	if err != nil || v == "" {
		return collectorin.IngressDeferralNone, err
	}
	value, err := store.Get(ctx, key(scope, v))
	if err != nil {
		return collectorin.IngressDeferralNone, err
	}
	if value == nil || value.IsNil() {
		return collectorin.IngressDeferralNone, nil
	}
	switch value.String() {
	case string(collectorin.IngressDeferralApproval):
		return collectorin.IngressDeferralApproval, nil
	case string(collectorin.IngressDeferralSelfLoop):
		return collectorin.IngressDeferralSelfLoop, nil
	default:
		return collectorin.IngressDeferralNone, gerror.New("采集入口缓存决定无效")
	}
}

// The caller captures the generation before its DB checks. A stale decision
// cannot be written into the generation published by a concurrent config edit.
func Remember(ctx context.Context, store Store, scope collectorin.BotIngressScope, version string, reason collectorin.IngressDeferralReason) error {
	if !scope.Valid() || version == "" || !reason.Valid() {
		return nil
	}
	_, err := store.SetIfNotExist(ctx, key(scope, version), string(reason), time.Minute)
	return err
}
