package sys

import (
	"testing"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestHistoryDeliveryWithinQuotaAllowsBotSource(t *testing.T) {
	if !historyDeliveryWithinQuota(0, 351, map[int64]int{}, map[int64]int{}, 8, 4) {
		t.Fatal("bot delivery with account_id=0 must be admitted by source quota")
	}
}

func TestHistoryDeliveryWithinQuotaLimitsProtocolAccounts(t *testing.T) {
	if historyDeliveryWithinQuota(628, 351, map[int64]int{628: 8}, map[int64]int{}, 8, 4) {
		t.Fatal("protocol account at quota must be rejected")
	}
}

func TestHistoryDeliveryWithinQuotaLimitsBotSource(t *testing.T) {
	if historyDeliveryWithinQuota(0, 351, map[int64]int{}, map[int64]int{351: 4}, 8, 4) {
		t.Fatal("bot source at quota must be rejected")
	}
}

func TestHistorySchedulerPoolIsStrictlyHistory(t *testing.T) {
	if historySchedulerPool != "history" {
		t.Fatalf("history scheduler pool = %q, want %q", historySchedulerPool, "history")
	}
}

func TestHistorySchedulerPriorityRange(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		want     bool
	}{
		{name: "normal history", priority: sysin.EventPriorityNormal, want: true},
		{name: "realtime excluded", priority: sysin.EventPriorityRealtime, want: false},
		{name: "urgent excluded from background", priority: sysin.EventPriorityUrgent, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := historySchedulerPriorityEligible(tt.priority); got != tt.want {
				t.Fatalf("priority %d eligible = %v, want %v", tt.priority, got, tt.want)
			}
		})
	}
}
