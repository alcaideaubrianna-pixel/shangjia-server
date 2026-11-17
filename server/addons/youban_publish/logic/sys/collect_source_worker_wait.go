package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/utility/sourcequeue"
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

func waitCollectSourceRuntime(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Await only this job, not its media dependencies or the whole source. The
// renewer is joined before settlement; uncertain ownership cancels business
// work and leaves the claim for bounded recovery rather than acknowledging it.
func awaitCollectSourceRuntimeClaim(ctx context.Context, queue collectSourceRuntimeQueue, claim *sourcequeue.Claim, lease time.Duration, work func(context.Context) error) (workErr, leaseErr error) {
	timeout := 2 * time.Minute
	if claim.Lane == sourcequeue.History {
		timeout = 5 * time.Second
	}
	execution, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan struct{})
	renewed := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(max(time.Millisecond, lease/3))
		defer ticker.Stop()
		for {
			select {
			case <-done:
				renewed <- nil
				return
			case <-execution.Done():
				renewed <- nil
				return
			case <-ticker.C:
			}
			renewCtx, stop := context.WithTimeout(execution, min(2*time.Second, lease/3))
			err := queue.Renew(renewCtx, claim, lease)
			stop()
			if err != nil {
				cancel()
				renewed <- err
				return
			}
		}
	}()
	workErr = runCollectSourceRuntimeWork(execution, work)
	if workErr == nil && execution.Err() != nil {
		workErr = execution.Err()
	}
	close(done)
	leaseErr = <-renewed
	return workErr, leaseErr
}

func runCollectSourceRuntimeWork(ctx context.Context, work func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = gerror.Newf("采集来源任务执行异常: %v", recovered)
		}
	}()
	return work(ctx)
}
