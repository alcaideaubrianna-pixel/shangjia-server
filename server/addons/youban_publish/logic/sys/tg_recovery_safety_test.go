package sys

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestTelegramRecoveryAndDispatchLocksUseFixedLeases(t *testing.T) {
	for _, file := range []string{"tg_job_recovery.go", "tg_channel_order.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), "WithoutWatchDog()") {
			t.Fatalf("%s must not renew a recovery or dispatch lease indefinitely", file)
		}
	}
}

func TestTelegramJobClaimRejectsExpiredDeadline(t *testing.T) {
	source, err := os.ReadFile("tg_send_repo.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "NotExpiredAt: now") {
		t.Fatal("job claim must fence queue tasks whose durable send deadline expired")
	}
}

func TestTelegramTimeoutCleanupClaimsLegacyNullSchedule(t *testing.T) {
	if !strings.Contains(telegramLegacyTimeoutCleanupClaimSQL, "timeout_cleanup_at IS NULL") {
		t.Fatal("timeout cleanup must include an indexed legacy NULL lane")
	}
	if telegramChannelDispatchLeaseTTL != 30*time.Second {
		t.Fatalf("unexpected dispatch lease TTL: %s", telegramChannelDispatchLeaseTTL)
	}
}
