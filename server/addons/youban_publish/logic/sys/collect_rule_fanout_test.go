package sys

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectRuleFanoutContinuesAfterFailureAndPanic(t *testing.T) {
	failure := errors.New("one rule failed")
	rules := []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}, {"id": g.NewVar(3)}, {"id": g.NewVar(4)}}
	var visited []int64
	got := executeCollectRuleBranches(context.Background(), rules, func(ctx context.Context, rule gdb.Record) (bool, string, error) {
		id := rule["id"].Int64()
		visited = append(visited, id)
		switch id {
		case 1:
			return false, "", failure
		case 2:
			panic("bad rule")
		case 3:
			return false, "not matched", nil
		default:
			return true, "", nil
		}
	})
	if !reflect.DeepEqual(visited, []int64{1, 2, 3, 4}) || !got.Matched || !errors.Is(got.Err, failure) {
		t.Fatalf("failed branch blocked later rules: visited=%v result=%+v", visited, got)
	}
	outcomes := []collectRuleBranchOutcome{}
	for _, branch := range got.Branches {
		outcomes = append(outcomes, branch.Outcome)
	}
	if !reflect.DeepEqual(outcomes, []collectRuleBranchOutcome{
		collectRuleBranchFailed, collectRuleBranchFailed, collectRuleBranchSkipped, collectRuleBranchSubmitted,
	}) {
		t.Fatalf("incorrect branch outcomes: %v", outcomes)
	}
}

func TestCollectRuleFanoutRealErrorPrecedesWaiting(t *testing.T) {
	failure := errors.New("database failed")
	rules := []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}
	got := executeCollectRuleBranches(context.Background(), rules, func(ctx context.Context, rule gdb.Record) (bool, string, error) {
		if rule["id"].Int64() == 1 {
			return false, "", newCollectProcessRetryError(time.Second, "wait")
		}
		return false, "", failure
	})
	var retry *collectProcessRetryError
	if !errors.Is(got.Err, failure) || errors.As(got.Err, &retry) {
		t.Fatalf("waiting branch concealed a real failure: %v", got.Err)
	}
}

func TestCollectRuleFanoutMixedErrorInsideOneBranchKeepsFailure(t *testing.T) {
	failure := errors.New("database failed")
	mixed := gerror.Wrap(errors.Join(newCollectProcessRetryError(time.Second, "wait"), failure), "branch failed")
	got := executeCollectRuleBranches(context.Background(), []gdb.Record{{"id": g.NewVar(1)}},
		func(context.Context, gdb.Record) (bool, string, error) { return false, "", mixed })
	var retry *collectProcessRetryError
	if !errors.Is(got.Err, failure) || errors.As(got.Err, &retry) || got.Branches[0].Outcome != collectRuleBranchFailed {
		t.Fatalf("mixed branch failure was reduced to a wait: %+v", got)
	}
}

func TestCollectRuleFanoutWaitsWithoutAcknowledgingEvent(t *testing.T) {
	rules := []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}
	got := executeCollectRuleBranches(context.Background(), rules, func(ctx context.Context, rule gdb.Record) (bool, string, error) {
		return false, "", newCollectProcessRetryError(time.Duration(rule["id"].Int64())*time.Second, "wait")
	})
	var retry *collectProcessRetryError
	if got.Matched || !errors.As(got.Err, &retry) || retry.delay != time.Second || len(got.Branches) != 2 {
		t.Fatalf("waiting branches incorrectly completed: %+v", got)
	}
}

func TestCollectRuleFanoutReservesBudgetForLaterRules(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	rules := []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}
	secondRan := false
	got := executeCollectRuleBranches(ctx, rules, func(branchCtx context.Context, rule gdb.Record) (bool, string, error) {
		if rule["id"].Int64() == 1 {
			deadline, ok := branchCtx.Deadline()
			if !ok || time.Until(deadline) > 310*time.Millisecond {
				t.Fatal("first rule received the whole event deadline")
			}
			<-branchCtx.Done()
			return false, "", branchCtx.Err()
		}
		secondRan = true
		return true, "", nil
	})
	if !secondRan || !got.Matched || !errors.Is(got.Err, context.DeadlineExceeded) {
		t.Fatalf("slow branch spent the later rule's budget: %+v", got)
	}
}

func TestCollectRuleFanoutCancellationDoesNotPretendSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := executeCollectRuleBranches(ctx, []gdb.Record{{"id": g.NewVar(1)}}, func(context.Context, gdb.Record) (bool, string, error) {
		t.Fatal("canceled event invoked rule")
		return true, "", nil
	})
	if got.Matched || !errors.Is(got.Err, context.Canceled) {
		t.Fatalf("canceled event was acknowledged: %+v", got)
	}
}

func TestCollectRuleFanoutTerminalMediaValidation(t *testing.T) {
	validation := gerror.Wrap(errProfileMediaValidation, "展示媒体实际1个")
	transient := errors.New("database timeout")
	for _, tc := range []struct {
		name string
		errs []error
		want bool
	}{
		{"validation_and_success", []error{validation, nil}, true},
		{"validation_and_wait", []error{validation, newCollectProcessRetryError(time.Second, "waiting")}, false},
		{"validation_and_database", []error{validation, transient}, false},
		{"joined_database_not_hidden", []error{errors.Join(validation, transient)}, false},
		{"all_validation", []error{validation, gerror.Wrap(validation, "second rule")}, true},
		{"success", []error{nil}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := make([]gdb.Record, len(tc.errs))
			for i := range rules {
				rules[i] = gdb.Record{"id": g.NewVar(i + 1)}
			}
			calls := 0
			result := executeCollectRuleBranches(context.Background(), rules, func(_ context.Context, rule gdb.Record) (bool, string, error) {
				calls++
				err := tc.errs[rule["id"].Int()-1]
				return err == nil, "", err
			})
			if calls != len(rules) || result.terminalMediaValidationFailure() != tc.want {
				t.Fatalf("calls=%d result=%+v wantTerminal=%t", calls, result, tc.want)
			}
		})
	}
}
