package sys

import "testing"

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
