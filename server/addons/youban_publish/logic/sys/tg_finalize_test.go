package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTelegramFinalizePreservesValuesAndCancellationCause(t *testing.T) {
	type ownershipKey struct{}
	parent := context.WithValue(context.Background(), ownershipKey{}, "owner-token")
	parent, stop := context.WithCancelCause(parent)
	cause := errors.New("lease lost")
	stop(cause)
	ctx, cancel := telegramFinalizeContextWithTimeout(parent, "test", time.Second)
	defer cancel()
	if ctx.Err() != nil || ctx.Value(ownershipKey{}) != "owner-token" {
		t.Fatal("finalization must preserve ownership values and detach send cancellation")
	}
	if ctx.Value(telegramFinalizeKey{}).(telegramFinalizeScope).cause != cause.Error() {
		t.Fatal("lost cancellation cause")
	}
}

func TestTelegramFinalizeNestedDeadlineNeverExtends(t *testing.T) {
	parent, cancel := telegramFinalizeContextWithTimeout(context.Background(), "outer", time.Second)
	defer cancel()
	child, stop := telegramFinalizeContextWithTimeout(parent, "inner", time.Minute)
	defer stop()
	first, _ := parent.Deadline()
	second, _ := child.Deadline()
	if !first.Equal(second) {
		t.Fatal("nested finalizer extended original deadline")
	}
	expired := context.WithValue(context.Background(), telegramFinalizeKey{}, telegramFinalizeScope{deadline: time.Now().Add(-time.Second)})
	retry, stopRetry := telegramFinalizeContextWithTimeout(expired, "retry", time.Minute)
	defer stopRetry()
	if !errors.Is(retry.Err(), context.DeadlineExceeded) {
		t.Fatal("expired finalizer received a fresh budget")
	}
}

func TestTelegramFinalizeRegistryDrainClosesAdmission(t *testing.T) {
	registry := new(telegramFinalizeRegistry)
	if !registry.admit() {
		t.Fatal("initial admission rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(registry.drain(ctx), context.Canceled) {
		t.Fatal("active work must respect shutdown deadline")
	}
	if registry.admit() {
		t.Fatal("new background work admitted during shutdown")
	}
	registry.complete()
	if err := registry.drain(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramFinalizeResponseBudgetCannotExtendParent(t *testing.T) {
	parent, cancel := telegramFinalizeContextWithTimeout(context.Background(), "outer", time.Second)
	defer cancel()
	child, stop := telegramFinalizeContextWithTimeout(parent, "response", 5*time.Second)
	defer stop()
	first, _ := parent.Deadline()
	second, _ := child.Deadline()
	if second.After(first) {
		t.Fatal("response persistence extended finalization deadline")
	}
}
