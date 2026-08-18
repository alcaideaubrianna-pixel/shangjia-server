package publishtenant

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
)

type Resolver func(context.Context) (int64, error)

var (
	resolverMu sync.RWMutex
	resolver   Resolver
)

func RegisterResolver(value Resolver) {
	resolverMu.Lock()
	defer resolverMu.Unlock()
	resolver = value
}

func CurrentID(ctx context.Context) (int64, error) {
	resolverMu.RLock()
	current := resolver
	resolverMu.RUnlock()
	if current == nil {
		return 0, gerror.New("资料租户服务不可用")
	}
	id, err := current(ctx)
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, gerror.New("租户身份无效")
	}
	return id, nil
}
