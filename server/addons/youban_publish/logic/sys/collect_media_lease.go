package sys

import (
	"context"
	"errors"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	collectMediaLeaseTTL             = 2 * time.Minute
	collectMediaLeaseRefreshInterval = 20 * time.Second
)

var errCollectMediaLeaseLost = errors.New("采集媒体执行租约已失效")

// Keep renewal bound to the task context, not the lifetime of the process.
// A terminated worker leaves at most one short lease behind. Refresh checks
// the ownership token atomically; uncertainty cancels I/O before takeover.
func maintainCollectMediaLeases(parent context.Context, eventID int64, interval time.Duration, refresh func(context.Context) (bool, error)) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				owned, err := refresh(refreshCtx)
				stop()
				if ctx.Err() != nil {
					return
				}
				if err != nil || !owned {
					g.Log().Warningf(ctx, "采集媒体租约续期失败，取消当前执行 eventId:%d owned:%t err:%v", eventID, owned, err)
					cancel(errCollectMediaLeaseLost)
					return
				}
			}
		}
	}()
	return ctx, func() {
		cancel(context.Canceled)
		<-done
	}
}
