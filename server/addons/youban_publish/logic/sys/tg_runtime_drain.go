package sys

import (
	"context"
	"errors"
	"sync"
	"time"

	"hotgo/utility/telegrambudget"

	"github.com/gogf/gf/v2/frame/g"
)

var errTelegramRuntimeShutdownHandoff = errors.New("发布实例排空超时，交由持久任务恢复")

// telegramRuntimeExecutions separates polling cancellation from admitted sends.
// Redis claim renewal and the existing send lease remain authoritative.
type telegramRuntimeExecutions struct {
	mu      sync.Mutex
	closing bool
	active  map[*telegramRuntimeExecution]struct{}
	idle    chan struct{}
}

type telegramRuntimeExecution struct {
	cancel context.CancelCauseFunc
}

func (r *telegramRuntimeExecutions) admit(parent context.Context) (context.Context, context.CancelFunc, func(), bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing || parent.Err() != nil {
		return nil, nil, nil, false
	}
	if len(r.active) == 0 {
		r.idle = make(chan struct{})
	}
	if r.active == nil {
		r.active = make(map[*telegramRuntimeExecution]struct{})
	}
	// Only the runtime polling context is detached. Job deadlines are applied
	// inside SendTelegramJob; a lost claim still cancels this execution.
	base := context.WithoutCancel(parent)
	var stopDeadline context.CancelFunc = func() {}
	if deadline, ok := parent.Deadline(); ok {
		base, stopDeadline = context.WithDeadline(base, deadline)
	}
	ctx, cancel := context.WithCancelCause(base)
	execution := &telegramRuntimeExecution{cancel: cancel}
	r.active[execution] = struct{}{}
	var once sync.Once
	complete := func() {
		once.Do(func() {
			cancel(context.Canceled)
			stopDeadline()
			r.mu.Lock()
			defer r.mu.Unlock()
			delete(r.active, execution)
			if len(r.active) == 0 {
				close(r.idle)
			}
		})
	}
	return ctx, func() { cancel(context.Canceled) }, complete, true
}

func (r *telegramRuntimeExecutions) closeAdmission() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closing = true
	return len(r.active)
}

func (r *telegramRuntimeExecutions) wait(ctx context.Context) error {
	r.mu.Lock()
	if len(r.active) == 0 {
		r.mu.Unlock()
		return nil
	}
	idle := r.idle
	r.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *telegramRuntimeExecutions) cancelForHandoff() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for execution := range r.active {
		execution.cancel(errTelegramRuntimeShutdownHandoff)
	}
}

func (s *sSysPublish) admitTelegramRuntimeExecution(ctx context.Context) (context.Context, context.CancelFunc, func(), bool) {
	value, ok := telegramFinalizeRegistries.Load(s)
	if !ok || value.(*telegramFinalizeRegistry).executions == nil {
		// Non-runtime callers and isolated tests retain caller cancellation.
		execution, cancel := context.WithCancel(ctx)
		return execution, cancel, cancel, true
	}
	return value.(*telegramFinalizeRegistry).executions.admit(ctx)
}

// Keep finalization inside the existing total shutdown deadline. Configuring a
// longer drain must not consume the time reserved for receipts and token settle.
func telegramRuntimeDrainBudget(total, requested, finalize time.Duration) time.Duration {
	reserve := min(finalize, total)
	return min(requested, max(time.Duration(0), total-reserve))
}

func (s *sSysPublish) drainTelegramRuntimeExecutions(ctx context.Context, registry *telegramFinalizeRegistry) {
	if registry.executions == nil {
		return
	}
	started := time.Now()
	active := registry.executions.closeAdmission()
	remaining := time.Duration(0)
	if deadline, ok := ctx.Deadline(); ok {
		remaining = max(time.Duration(0), time.Until(deadline))
	}
	budget := telegramRuntimeDrainBudget(remaining, telegrambudget.ShutdownDrain(ctx), telegrambudget.Finalize(ctx))
	g.Log().Info(ctx, "发布实例停止领取，等待已领取任务", g.Map{
		"event": "telegram_runtime_drain_started", "activeExecutions": active, "drainBudgetMs": budget.Milliseconds(),
	})
	drainCtx, cancel := context.WithTimeout(ctx, budget)
	err := registry.executions.wait(drainCtx)
	cancel()
	if err != nil {
		registry.executions.cancelForHandoff()
		g.Log().Warning(ctx, "发布排空窗口结束，取消执行并保留恢复凭证", g.Map{
			"event": "telegram_runtime_handoff", "activeExecutions": active, "durationMs": time.Since(started).Milliseconds(),
		})
		// The receipt/attempt finalizers and token settle run before the
		// execution unregisters; wait only within the same total deadline.
		err = registry.executions.wait(ctx)
	}
	result := "drained"
	if err != nil {
		result = "deadline_exhausted"
	}
	g.Log().Info(ctx, "发布执行排空结束", g.Map{
		"event": "telegram_runtime_drain_finished", "result": result, "durationMs": time.Since(started).Milliseconds(),
	})
}
