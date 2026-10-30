package sys

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"golang.org/x/sync/semaphore"
)

type antiScanProviderGate struct {
	mu       sync.Mutex
	limit    int
	gate     *semaphore.Weighted
	interval time.Duration
	nextAt   time.Time
}

var antiScanProviderGates = map[string]*antiScanProviderGate{}
var antiScanProviderGatesMu sync.Mutex

func antiScanProviderIntEnv(name string, fallback, max int) int {
	value := fallback
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			value = parsed
		}
	}
	if value > max {
		value = max
	}
	return value
}

func acquireAntiScanProvider(ctx context.Context, provider string) (func(), error) {
	limit, interval := 1, time.Duration(0)
	switch provider {
	case "aliyun":
		limit = antiScanProviderIntEnv("YOUBAN_ALIYUN_MATTING_CONCURRENCY", 5, 20)
	case "tencent":
		limit = antiScanProviderIntEnv("YOUBAN_TENCENT_MATTING_CONCURRENCY", 4, 20)
	case "fapihub":
		limit = antiScanProviderIntEnv("YOUBAN_FAPIHUB_CONCURRENCY", 2, 20)
		perMinute := antiScanProviderIntEnv("YOUBAN_FAPIHUB_RATE_LIMIT_PER_MINUTE", 150, 6000)
		interval = time.Minute / time.Duration(perMinute)
	default:
		return func() {}, nil
	}
	antiScanProviderGatesMu.Lock()
	gate := antiScanProviderGates[provider]
	if gate == nil {
		gate = &antiScanProviderGate{}
		antiScanProviderGates[provider] = gate
	}
	antiScanProviderGatesMu.Unlock()
	gate.mu.Lock()
	if gate.gate == nil || gate.limit != limit || gate.interval != interval {
		gate.limit, gate.interval, gate.gate = limit, interval, semaphore.NewWeighted(int64(limit))
	}
	sem := gate.gate
	gate.mu.Unlock()
	if err := sem.Acquire(ctx, 1); err != nil {
		return nil, gerror.Wrapf(err, "等待%s防扫图限流失败", provider)
	}
	if interval > 0 {
		gate.mu.Lock()
		wait := time.Until(gate.nextAt)
		if wait < 0 {
			wait = 0
		}
		gate.nextAt = time.Now().Add(wait + interval)
		gate.mu.Unlock()
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				sem.Release(1)
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return func() { sem.Release(1) }, nil
}
