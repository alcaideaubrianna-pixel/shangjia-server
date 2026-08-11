package sys

import (
	"strings"
	"testing"
	"time"
)

func TestTelegramSendingRecoveryStartsAfterTaskTimeout(t *testing.T) {
	if telegramSendingJobRecoverAfter <= telegramPublishTaskTimeout {
		t.Fatalf("sending recovery %s must be later than task timeout %s", telegramSendingJobRecoverAfter, telegramPublishTaskTimeout)
	}
}

func TestTelegramChannelBusyDelayBackoff(t *testing.T) {
	initial := telegramChannelBusyDelayDuration(3, 10, 0)
	busy := telegramChannelBusyDelayDuration(3, 10, 20)
	max := telegramChannelBusyDelayDuration(3, 10, 1000)
	if busy <= initial {
		t.Fatalf("busy delay must grow: initial=%s busy=%s", initial, busy)
	}
	if max > 30*time.Second {
		t.Fatalf("busy delay must be capped: %s", max)
	}
}

func TestTelegramActiveChannelConditionSupportsMessagePushTargets(t *testing.T) {
	condition := telegramActiveChannelCondition()

	for _, operation := range []string{"message_push:%", "message_push_plan:%"} {
		if !strings.Contains(condition, "operation_no LIKE '"+operation+"'") {
			t.Fatalf("active channel condition does not cover %s", operation)
		}
	}
	if !strings.Contains(condition, publishTgChannelTable) {
		t.Fatalf("active channel condition does not validate TG channel cache")
	}
	if !strings.Contains(condition, publishChannelTable) {
		t.Fatalf("active channel condition does not validate publish channels")
	}
}

func TestPendingIdleTelegramChannelHeadQueryIsFairPerChannel(t *testing.T) {
	query := pendingIdleTelegramChannelHeadQuery(500)
	for _, fragment := range []string{
		"ROW_NUMBER() OVER",
		"PARTITION BY tenant_id, channel_id",
		"ORDER BY COALESCE(priority, 100) ASC, id ASC",
		"WHERE ranked.recovery_rank = 1",
		"LIMIT 500",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("expected recovery query to contain %q", fragment)
		}
	}
}

func TestPendingIdleTelegramChannelHeadQueryUsesSafeDefaultLimit(t *testing.T) {
	if query := pendingIdleTelegramChannelHeadQuery(0); !strings.Contains(query, "LIMIT 100") {
		t.Fatalf("expected default recovery limit, query: %s", query)
	}
}
