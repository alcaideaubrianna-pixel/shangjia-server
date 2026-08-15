package sys

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramJobErrorRetryPolicyPermanentForBannedInChannel(t *testing.T) {
	err := assertError("Bad Request: USER_BANNED_IN_CHANNEL")
	policy := telegramJobErrorRetryPolicy(err, 1)
	if !policy.Permanent {
		t.Fatal("expected permanent policy")
	}
	if policy.RetryDelay != 0 {
		t.Fatalf("unexpected retry delay: %s", policy.RetryDelay)
	}
	if got := policy.Message; !strings.Contains(got, "当前账号已被目标频道封禁") {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestTelegramChannelPermissionError(t *testing.T) {
	if !isTelegramChannelPermissionError(assertError("rpcDoRequest: USER_BANNED_IN_CHANNEL")) {
		t.Fatal("USER_BANNED_IN_CHANNEL must be treated as a channel permission error")
	}
	if isTelegramChannelPermissionError(assertError("rpc error: DC is closed")) {
		t.Fatal("DC is closed must remain a transient connection error")
	}
}

func TestTelegramAccountBusyBecomesPermanentAfterThreeRetries(t *testing.T) {
	err := &telegramAccountBusyError{tgAccountId: 16, err: context.DeadlineExceeded}
	policy := telegramJobErrorRetryPolicy(err, telegramRetryMaxCount)
	if !policy.Permanent {
		t.Fatal("account busy must become permanent after retry limit")
	}
	if policy.RetryDelay != 0 {
		t.Fatal("permanent account busy must not have a retry delay")
	}
}

func TestTelegramAccountBusyIsNotAmbiguousDelivery(t *testing.T) {
	err := &telegramAccountBusyError{tgAccountId: 33, err: context.DeadlineExceeded}
	if !isTelegramAmbiguousDeliveryError(err) {
		t.Fatal("wrapped deadline remains ambiguous before account-busy precedence is applied")
	}
	decision := telegramJobFailureNextState(err, 2)
	if decision.Status != "failed_retry" || decision.RetryCount != 3 || decision.RetryDelay <= 0 {
		t.Fatalf("account busy must consume retry count: %+v", decision)
	}
}

func TestTelegramVideoAsPhotoIsPermanent(t *testing.T) {
	err := assertError(`Bad Request: can't use file of type Video as Photo`)
	if !isTelegramPermanentSendError(err) {
		t.Fatal("video as photo must be treated as a permanent media type error")
	}
}

func TestTelegramJobStateUpdateDataUsesSingleTerminalRule(t *testing.T) {
	now := gtime.Now()
	for _, status := range []string{"sent", "failed", "superseded"} {
		data := telegramJobStateUpdateData(status, time.Minute, now)
		if data["dispatch_status"] != tgDispatchStatusDone {
			t.Fatalf("terminal status %s must be done: %+v", status, data)
		}
		if data["next_retry_at"] != nil {
			t.Fatalf("terminal status %s must not retry: %+v", status, data)
		}
	}
	retry := telegramJobStateUpdateData("failed_retry", time.Minute, now)
	if retry["dispatch_status"] != tgDispatchStatusIdle || retry["next_retry_at"] == nil {
		t.Fatalf("retry status must remain idle with next retry: %+v", retry)
	}
}

func TestTelegramJobFailureNextStateUsesTerminalRule(t *testing.T) {
	decision := telegramJobFailureNextState(assertError("Bad Request: chat not found"), 0)
	if decision.Status != "failed_retry" || decision.DispatchStatus != tgDispatchStatusIdle || decision.RetryDelay <= 0 {
		t.Fatalf("chat not found should be retried: %+v", decision)
	}
}

func TestTelegramSlowModeErrorIsDetected(t *testing.T) {
	if !isTelegramSlowModeError(assertError("rpc error: SLOWMODE_WAIT (9)")) {
		t.Fatal("SLOWMODE_WAIT must be detected")
	}
	if isTelegramSlowModeError(assertError("too many requests: retry after 9")) {
		t.Fatal("generic rate limit must not be treated as channel slow mode")
	}
}

func TestTelegramMediaSizeLimitError(t *testing.T) {
	for _, message := range []string{
		"413 Request Entity Too Large",
		"file of size 14851446 bytes is too big for a photo; maximum is 10485760 bytes",
		"Bad Request: file is too big",
	} {
		if !isTelegramMediaSizeLimitError(assertError(message)) {
			t.Fatalf("media size error must trigger account fallback: %q", message)
		}
	}
	for _, message := range []string{
		"Bad Request: chat not found",
		"Too Many Requests: retry after 10",
		"context deadline exceeded",
	} {
		if isTelegramMediaSizeLimitError(assertError(message)) {
			t.Fatalf("non-size error must not trigger account fallback: %q", message)
		}
	}
}

func assertError(message string) error {
	return simpleError(message)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }
