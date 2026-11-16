package sys

import (
	"context"
	"errors"
	"sync"
	"time"

	"hotgo/utility/telegrambudget"

	"github.com/gogf/gf/v2/frame/g"
)

type telegramFinalizeKey struct{}
type telegramFinalizeScope struct {
	deadline time.Time
	original time.Time
	source   string
	cause    string
}

// telegramFinalizeContext is for evidence and local state only. It preserves
// ownership values, but callers must retain their existing SQL/lease fences.
// Nested calls share the original deadline, including after it has expired.
// The returned context must never be used to start another Telegram send.
func telegramFinalizeContext(ctx context.Context, stage string) (context.Context, context.CancelFunc) {
	return telegramFinalizeContextWithTimeout(ctx, stage, telegrambudget.Finalize(ctx))
}

func telegramFinalizeContextWithTimeout(ctx context.Context, stage string, budget time.Duration) (context.Context, context.CancelFunc) {
	scope, ok := ctx.Value(telegramFinalizeKey{}).(telegramFinalizeScope)
	if !ok {
		scope = telegramFinalizeScope{deadline: time.Now().Add(budget), source: "finalize"}
		scope.original, _ = ctx.Deadline()
		if cause := context.Cause(ctx); cause != nil {
			scope.cause = telegramPublishLogError(cause)
		}
	}
	if shorter := time.Now().Add(budget); shorter.Before(scope.deadline) {
		scope.deadline = shorter
	}
	base := context.WithValue(context.WithoutCancel(ctx), telegramFinalizeKey{}, scope)
	bounded, cancel := context.WithDeadline(base, scope.deadline)
	logTelegramFinalize(bounded, stage, "entered", nil)
	return bounded, cancel
}

func logTelegramFinalize(ctx context.Context, stage, verdict string, err error) {
	fields := g.Map{"event": "telegram_finalize", "stage": stage, "handoff": "local_state_only", "verdict": verdict}
	if base, ok := ctx.Value(telegramPublishTraceContextKey{}).(g.Map); ok {
		for key, value := range base {
			fields[key] = value
		}
	}
	if scope, ok := ctx.Value(telegramFinalizeKey{}).(telegramFinalizeScope); ok {
		fields["deadlineSource"] = scope.source
		fields["effectiveDeadline"] = scope.deadline.UTC().Format(time.RFC3339Nano)
		fields["stageRemainingMs"] = time.Until(scope.deadline).Milliseconds()
		fields["cancelCause"] = scope.cause
		if !scope.original.IsZero() {
			fields["sourceDeadline"] = scope.original.UTC().Format(time.RFC3339Nano)
		}
	}
	if err != nil {
		fields["error"] = telegramPublishLogError(err)
	}
	telegramPublishStageDeadlineFields(ctx, fields)
	g.Log().Info(ctx, "TG独立收尾", fields)
}

func telegramFinalizeAttemptContext(ctx context.Context, attempt telegramDeliveryAttempt) context.Context {
	return telegramPublishTraceContext(ctx, attempt.JobId, g.Map{
		"attemptId": attempt.Id, "channelId": attempt.ChannelId,
		"targetChatId": attempt.TargetChatId, "phase": attempt.Phase,
	})
}

// The registry uses a count and generation channel rather than WaitGroup so
// admission can be closed atomically with shutdown, without Add/Wait races.
type telegramFinalizeRegistry struct {
	mu      sync.Mutex
	closing bool
	active  int
	idle    chan struct{}
}

func (r *telegramFinalizeRegistry) admit() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return false
	}
	if r.active == 0 {
		r.idle = make(chan struct{})
	}
	r.active++
	return true
}

func (r *telegramFinalizeRegistry) complete() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	if r.active == 0 {
		close(r.idle)
	}
}

func (r *telegramFinalizeRegistry) drain(ctx context.Context) error {
	r.mu.Lock()
	r.closing = true
	idle, active := r.idle, r.active
	r.mu.Unlock()
	if active == 0 {
		return nil
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *telegramFinalizeRegistry) closeAdmission() {
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
}

var telegramFinalizeRegistries sync.Map

func (s *sSysPublish) telegramFinalizeRegistry() *telegramFinalizeRegistry {
	registry, _ := telegramFinalizeRegistries.LoadOrStore(s, &telegramFinalizeRegistry{})
	return registry.(*telegramFinalizeRegistry)
}

// runTelegramFinalizeAsync returns false during drain. Evidence must be saved
// synchronously before handing optional projections to this method.
func (s *sSysPublish) runTelegramFinalizeAsync(ctx context.Context, stage string, fn func(context.Context) error) bool {
	registry := s.telegramFinalizeRegistry()
	if !registry.admit() {
		logTelegramFinalize(ctx, stage, "shutdown_rejected", nil)
		return false
	}
	bounded, cancel := telegramFinalizeContext(ctx, stage)
	go func() {
		defer registry.complete()
		defer cancel()
		if err := bounded.Err(); err != nil {
			logTelegramFinalize(bounded, stage, "deadline_expired", err)
			return
		}
		err := fn(bounded)
		verdict := "completed"
		if err != nil {
			verdict = "recovery_required"
		}
		logTelegramFinalize(bounded, stage, verdict, err)
	}()
	return true
}

func telegramFinalizeShutdownGrace() time.Duration {
	return telegrambudget.Shutdown(context.Background())
}

var errTelegramFinalizeShutdown = errors.New("Telegram收尾等待超过退出宽限期")
