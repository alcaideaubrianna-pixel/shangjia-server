package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/library/cache"
	"time"
)

const collectSourceCircuitThreshold = 5
const collectSourceCircuitTTL = 5 * time.Minute

func collectSourceCircuitKey(p collectProcessQueuePayload) string {
	return fmt.Sprintf("youban_publish:collect:source_circuit:%d:%d:%d", p.TenantId, p.AccountId, p.SourceId)
}
func collectSourceCircuitFailure(ctx context.Context, p collectProcessQueuePayload) (bool, error) {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return false, nil
	}
	key := collectSourceCircuitKey(p)
	value, _ := cache.Instance().Get(ctx, key)
	n := value.Int() + 1
	if err := cache.Instance().Set(ctx, key, n, collectSourceCircuitTTL); err != nil {
		return false, err
	}
	return n >= collectSourceCircuitThreshold, nil
}
func clearCollectSourceCircuit(ctx context.Context, p collectProcessQueuePayload) {
	if cache.Initialized() && g.Cfg().MustGet(ctx, "cache.adapter").String() == "redis" {
		_, _ = cache.Instance().Remove(ctx, collectSourceCircuitKey(p))
	}
}
