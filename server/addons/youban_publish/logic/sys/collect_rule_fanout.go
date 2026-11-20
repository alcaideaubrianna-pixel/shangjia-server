package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

type collectRuleBranchOutcome string

const (
	collectRuleBranchSubmitted collectRuleBranchOutcome = "submitted"
	collectRuleBranchSkipped   collectRuleBranchOutcome = "skipped"
	collectRuleBranchWaiting   collectRuleBranchOutcome = "waiting"
	collectRuleBranchFailed    collectRuleBranchOutcome = "failed"
)

type collectRuleBranchResult struct {
	RuleID  int64
	Outcome collectRuleBranchOutcome
	Reason  string
	Err     error
}

type collectRuleFanoutResult struct {
	Branches []collectRuleBranchResult
	Matched  bool
	Reasons  []string
	Err      error
}

// The event lease still serializes a material group. A branch has only a share
// of the remaining execution budget, so an early slow rule cannot spend the
// whole event deadline. Existing dispatch identities own idempotent retries.
func executeCollectRuleBranches(ctx context.Context, rules []gdb.Record, dispatch func(context.Context, gdb.Record) (bool, string, error)) collectRuleFanoutResult {
	result := collectRuleFanoutResult{Branches: make([]collectRuleBranchResult, 0, len(rules))}
	var failures []error
	var wait time.Duration
	for index, rule := range rules {
		budget := 10 * time.Second
		if deadline, ok := ctx.Deadline(); ok {
			budget = min(budget, time.Until(deadline)/time.Duration(len(rules)-index))
		}
		branchCtx, cancel := context.WithTimeout(ctx, budget)
		matched, reason, err := executeCollectRuleBranch(branchCtx, rule, dispatch)
		cancel()
		branch := collectRuleBranchResult{RuleID: rule["id"].Int64(), Reason: reason, Err: err}
		var retry *collectProcessRetryError
		realFailure := collectRuleRealFailure(err)
		switch {
		case err == nil && matched:
			branch.Outcome = collectRuleBranchSubmitted
			result.Matched = true
		case err == nil:
			branch.Outcome = collectRuleBranchSkipped
			if reason != "" {
				result.Reasons = append(result.Reasons, reason)
			}
		case realFailure == nil && errors.As(err, &retry):
			branch.Outcome = collectRuleBranchWaiting
			if wait == 0 || retry.delay < wait {
				wait = retry.delay
			}
		default:
			branch.Outcome = collectRuleBranchFailed
			failures = append(failures, gerror.Wrapf(realFailure, "采集规则%d分发失败", branch.RuleID))
		}
		result.Branches = append(result.Branches, branch)
	}
	// Do not join waiting errors into real failures: callers use errors.As to
	// select retry scheduling, which must not conceal a hard branch failure.
	if len(failures) > 0 {
		result.Err = errors.Join(failures...)
	} else if wait > 0 {
		result.Err = newCollectProcessRetryError(wait, "部分采集规则等待重试")
	}
	return result
}

func collectRuleRealFailure(err error) error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var failures []error
		for _, child := range joined.Unwrap() {
			if failure := collectRuleRealFailure(child); failure != nil {
				failures = append(failures, failure)
			}
		}
		return errors.Join(failures...)
	}
	if child := errors.Unwrap(err); child != nil {
		return collectRuleRealFailure(child)
	}
	if _, waiting := err.(*collectProcessRetryError); waiting {
		return nil
	}
	return err
}

func executeCollectRuleBranch(ctx context.Context, rule gdb.Record, dispatch func(context.Context, gdb.Record) (bool, string, error)) (matched bool, reason string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("采集规则执行异常: %v", recovered)
		}
	}()
	if err = ctx.Err(); err != nil {
		return false, "", err
	}
	matched, reason, err = dispatch(ctx, rule)
	if err == nil {
		err = ctx.Err()
	}
	return
}

// Stop polling only after every branch has settled. A successful sibling keeps
// its dispatch identity; temporary errors and dependency waits still retry.
func (r collectRuleFanoutResult) terminalMediaValidationFailure() bool {
	hasFailure := false
	for _, branch := range r.Branches {
		if branch.Outcome == collectRuleBranchWaiting {
			return false
		}
		if branch.Err != nil {
			if !collectOnlyMediaValidationErrors(branch.Err) {
				return false
			}
			hasFailure = true
		}
	}
	return hasFailure
}

func collectOnlyMediaValidationErrors(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !collectOnlyMediaValidationErrors(child) {
				return false
			}
		}
		return true
	}
	if child := errors.Unwrap(err); child != nil {
		return collectOnlyMediaValidationErrors(child)
	}
	return isProfileMediaValidationError(err)
}
