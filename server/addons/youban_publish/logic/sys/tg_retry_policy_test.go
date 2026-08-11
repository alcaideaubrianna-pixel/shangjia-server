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

func TestTelegramAccountBusyDoesNotBecomePermanentAfterFiveRetries(t *testing.T) {
	err := &telegramAccountBusyError{tgAccountId: 16, err: context.DeadlineExceeded}
	policy := telegramJobErrorRetryPolicy(err, telegramRetryMaxCount+1)
	if policy.Permanent {
		t.Fatal("account busy must remain retryable")
	}
	if policy.RetryDelay <= 0 {
		t.Fatal("account busy must have a retry delay")
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
	if decision.Status != "failed" || decision.DispatchStatus != tgDispatchStatusDone || decision.RetryDelay != 0 {
		t.Fatalf("unexpected permanent failure decision: %+v", decision)
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

func TestTelegramChannelNextSendDelay(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	lastSentAt := gtime.New(time.Date(2026, 8, 11, 13, 0, 0, 0, location))
	now := gtime.New(time.Date(2026, 8, 11, 13, 0, 20, 0, location))
	if got := telegramChannelNextSendDelay(lastSentAt, now, 30*time.Second); got != 10*time.Second {
		t.Fatalf("expected 10 seconds remaining, got %s", got)
	}
	if got := telegramChannelNextSendDelay(lastSentAt, now, 15*time.Second); got != 0 {
		t.Fatalf("expected no delay after interval elapsed, got %s", got)
	}
}

func TestTelegramChannelNextSendDelayHandlesMissingTime(t *testing.T) {
	if got := telegramChannelNextSendDelay(nil, gtime.Now(), 30*time.Second); got != 0 {
		t.Fatalf("expected no delay when last sent time is missing, got %s", got)
	}
}

func assertError(message string) error {
	return simpleError(message)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }
