package sys

import (
	"context"
	"time"
)

// Queue probes fail closed for retirement, while ownership renewal fails closed
// for execution: an uncertain lease must never permit continued consumption.
func waitCollectSourceQueueWorker(ctx context.Context, interval, idleTimeout time.Duration, renew func(context.Context) (bool, error), hasWork func(context.Context) (bool, error)) {
	if interval <= 0 {
		return
	}
	idleSince := time.Time{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		checkCtx, checkCancel := context.WithTimeout(ctx, min(2*time.Second, interval))
		renewed, err := renew(checkCtx)
		checkCancel()
		if err != nil || !renewed {
			return
		}
		checkCtx, checkCancel = context.WithTimeout(ctx, 2*time.Second)
		pending, err := hasWork(checkCtx)
		checkCancel()
		if err != nil || pending {
			idleSince = time.Time{}
			continue
		}
		if idleSince.IsZero() {
			idleSince = time.Now()
		} else if time.Since(idleSince) >= idleTimeout {
			return
		}
	}
}
