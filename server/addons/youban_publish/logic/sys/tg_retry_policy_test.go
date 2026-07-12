package sys

import (
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

func assertError(message string) error {
	return simpleError(message)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }
