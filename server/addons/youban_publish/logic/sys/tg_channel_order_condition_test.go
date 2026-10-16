package sys

import (
	"strings"
	"testing"
)

func TestTelegramChannelBlockingJobExcludesUnknownAndUnavailableAccounts(t *testing.T) {
	condition := telegramChannelBlockingJobSQL("j")
	if strings.Contains(condition, "'unknown'") {
		t.Fatalf("unknown reconciliation jobs must not block normal channel order: %s", condition)
	}
	if !strings.Contains(condition, "status='authorized'") || !strings.Contains(condition, "session_key") {
		t.Fatalf("channel blocking condition must require an available account: %s", condition)
	}
}
