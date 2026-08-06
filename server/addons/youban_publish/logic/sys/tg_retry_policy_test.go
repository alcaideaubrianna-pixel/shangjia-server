package sys

import (
	"context"
	"strings"
	"testing"
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

func assertError(message string) error {
	return simpleError(message)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }
