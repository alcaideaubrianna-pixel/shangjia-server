package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTelegramRuntimeDrainKeepsAdmittedSendAlive(t *testing.T) {
	parent, stopPolling := context.WithCancel(context.Background())
	defer stopPolling()
	registry := &telegramRuntimeExecutions{}
	execution, _, complete, ok := registry.admit(parent)
	if !ok {
		t.Fatal("running instance rejected execution")
	}
	defer complete()
	if active := registry.closeAdmission(); active != 1 {
		t.Fatalf("active=%d", active)
	}
	stopPolling()
	if execution.Err() != nil {
		t.Fatal("stopping polling canceled an admitted send")
	}
	if _, _, _, ok := registry.admit(context.Background()); ok {
		t.Fatal("draining instance admitted new work")
	}
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(registry.wait(stopped), context.Canceled) {
		t.Fatal("drain did not wait for admitted send")
	}
	complete()
	complete() // Unregister and close must be idempotent.
	if err := registry.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramRuntimeDrainPreservesLeaseLossCancellation(t *testing.T) {
	registry := &telegramRuntimeExecutions{}
	execution, loseLease, complete, ok := registry.admit(context.Background())
	if !ok {
		t.Fatal("execution rejected")
	}
	defer complete()
	loseLease()
	if !errors.Is(execution.Err(), context.Canceled) {
		t.Fatal("lost claim did not cancel send")
	}
	if errors.Is(context.Cause(execution), errTelegramRuntimeShutdownHandoff) {
		t.Fatal("lease loss was mislabeled as a shutdown handoff")
	}
}

func TestTelegramRuntimeDrainForcedHandoffHasDistinctCause(t *testing.T) {
	registry := &telegramRuntimeExecutions{}
	execution, cancelLease, complete, ok := registry.admit(context.Background())
	if !ok {
		t.Fatal("execution rejected")
	}
	defer complete()
	registry.closeAdmission()
	registry.cancelForHandoff()
	cancelLease()
	if !errors.Is(context.Cause(execution), errTelegramRuntimeShutdownHandoff) {
		t.Fatal("forced handoff lost its cancellation cause")
	}
	complete()
	if err := registry.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramRuntimeDrainRejectsCanceledAdmission(t *testing.T) {
	registry := &telegramRuntimeExecutions{}
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, admitted := registry.admit(parent); admitted {
		t.Fatal("canceled polling context admitted a new send")
	}
}

func TestTelegramRuntimeDrainRetainsCallerDeadline(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()
	registry := &telegramRuntimeExecutions{}
	execution, _, complete, ok := registry.admit(parent)
	if !ok {
		t.Fatal("execution rejected")
	}
	defer complete()
	before, _ := parent.Deadline()
	after, ok := execution.Deadline()
	if !ok || !after.Equal(before) {
		t.Fatal("drain detached an explicit execution deadline")
	}
}

func TestTelegramRuntimeDrainBudgetReservesFinalization(t *testing.T) {
	for _, test := range []struct {
		total, requested, finalize, want time.Duration
	}{
		{30 * time.Second, 20 * time.Second, 10 * time.Second, 20 * time.Second},
		{30 * time.Second, time.Minute, 10 * time.Second, 20 * time.Second},
		{5 * time.Second, time.Minute, 10 * time.Second, 0},
		{0, time.Minute, time.Second, 0},
	} {
		if got := telegramRuntimeDrainBudget(test.total, test.requested, test.finalize); got != test.want {
			t.Fatalf("budget(%v,%v,%v)=%v want %v", test.total, test.requested, test.finalize, got, test.want)
		}
	}
}

func TestTelegramShutdownHandoffDeadlineIsBoundedAndStrict(t *testing.T) {
	now := time.UnixMilli(1794880000000)
	for _, marker := range []string{"", "awaiting_account_fallback:verify", telegramShutdownHandoffPrefix,
		telegramShutdownHandoffPrefix + "-1", telegramShutdownHandoffPrefix + "invalid",
		telegramShutdownHandoffPrefix + "999999999999999999999"} {
		if _, ok := telegramShutdownHandoffDeadline(marker); ok {
			t.Fatalf("accepted invalid marker %q", marker)
		}
	}
	job := telegramJobRecord{LastDispatchError: telegramShutdownHandoffPrefix + "1794880000000"}
	if telegramShutdownHandoffExpired(job, now.Add(-time.Millisecond)) {
		t.Fatal("handoff expired before confirmation deadline")
	}
	if !telegramShutdownHandoffExpired(job, now) {
		t.Fatal("handoff did not expire at confirmation deadline")
	}
}
