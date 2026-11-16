package sys

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestTelegramRecoveryResumesAfterExhaustedBudget(t *testing.T) {
	steps := make([]telegramRecoveryStep, 16)
	for i := range steps {
		steps[i].name = fmt.Sprint(i)
	}
	var visited []string
	next := 0
	// Model six slow steps consuming a cycle's budget. Later recovery lanes
	// must run in subsequent rounds even when every round exhausts its budget.
	for round := 0; round < 3; round++ {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		runTelegramRecoverySteps(ctx, steps, &next, func(ctx context.Context, step telegramRecoveryStep) {
			if ctx.Err() != nil {
				t.Fatal("started a step after cycle cancellation")
			}
			visited = append(visited, step.name)
			calls++
			if calls == 6 {
				cancel()
			}
		})
		cancel()
	}
	for i := 0; i < 18; i++ {
		if visited[i] != fmt.Sprint(i%16) {
			t.Fatalf("recovery starved a lane: %v", visited)
		}
	}
	if next != 2 {
		t.Fatalf("next step=%d, want 2", next)
	}
}

func TestTelegramRecoveryStepFailureDoesNotStopOtherLanes(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var visited []int
	steps := make([]telegramRecoveryStep, 3)
	for i := range steps {
		steps[i] = telegramRecoveryStep{name: fmt.Sprint(i), message: "测试恢复步骤", run: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			parentDeadline, _ := parent.Deadline()
			if !ok || deadline.After(parentDeadline) {
				t.Fatal("step extended the total recovery budget")
			}
			visited = append(visited, i)
			if i == 0 {
				return errors.New("isolated database failure")
			}
			return nil
		}}
	}
	next := 0
	new(sSysPublish).runTelegramRecoveryStep(parent, telegramRecoveryStep{name: "timeout", run: func(ctx context.Context) error {
		return context.DeadlineExceeded
	}})
	runTelegramRecoverySteps(parent, steps, &next, new(sSysPublish).runTelegramRecoveryStep)
	if !reflect.DeepEqual(visited, []int{0, 1, 2}) || parent.Err() != nil {
		t.Fatalf("failed step blocked independent lanes: visited=%v parent=%v", visited, parent.Err())
	}
}

func TestTelegramRecoveryRotatesCompleteRoundsAndHonorsCancellation(t *testing.T) {
	steps := []telegramRecoveryStep{{name: "a"}, {name: "b"}, {name: "c"}}
	next := 0
	var visited []string
	run := func(_ context.Context, step telegramRecoveryStep) { visited = append(visited, step.name) }
	for range 3 {
		runTelegramRecoverySteps(context.Background(), steps, &next, run)
	}
	if !reflect.DeepEqual(visited, []string{"a", "b", "c", "b", "c", "a", "c", "a", "b"}) {
		t.Fatalf("first-step budget did not rotate: %v", visited)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runTelegramRecoverySteps(ctx, steps, &next, func(context.Context, telegramRecoveryStep) { t.Fatal("cancelled cycle ran a step") })
	if next != 0 {
		t.Fatalf("cancelled cycle advanced progress: %d", next)
	}
	runTelegramRecoverySteps(context.Background(), nil, &next, run)
}
