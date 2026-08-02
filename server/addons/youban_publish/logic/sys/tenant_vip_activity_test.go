package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestTenantVipExtensionExpiredAtStacksRemainingTime(t *testing.T) {
	now := gtime.NewFromStr("2026-08-01 10:00:00")
	currentExpiredAt := gtime.NewFromStr("2026-08-21 10:00:00")

	got := tenantVipExtensionExpiredAt(now, currentExpiredAt, 3)
	want := "2026-08-24 10:00:00"
	if got.Format("Y-m-d H:i:s") != want {
		t.Fatalf("tenantVipExtensionExpiredAt() = %s, want %s", got.Format("Y-m-d H:i:s"), want)
	}
}

func TestTenantVipExtensionExpiredAtStartsFromNowWhenExpired(t *testing.T) {
	now := gtime.NewFromStr("2026-08-01 10:00:00")
	currentExpiredAt := gtime.NewFromStr("2026-07-31 10:00:00")

	got := tenantVipExtensionExpiredAt(now, currentExpiredAt, 1)
	want := "2026-08-02 10:00:00"
	if got.Format("Y-m-d H:i:s") != want {
		t.Fatalf("tenantVipExtensionExpiredAt() = %s, want %s", got.Format("Y-m-d H:i:s"), want)
	}
}

func TestTenantVipActivityTriggerEligible(t *testing.T) {
	if tenantVipActivityTriggerEligible(gtime.NewFromStr("2026-07-31 23:59:59"), "2026-08-01 00:00:00") {
		t.Fatal("event before activity enabled time should not be eligible")
	}
	if !tenantVipActivityTriggerEligible(gtime.NewFromStr("2026-08-01 00:00:00"), "2026-08-01 00:00:00") {
		t.Fatal("event at activity enabled time should be eligible")
	}
}

func TestTenantVipEventNotifyEnabled(t *testing.T) {
	cfg := &model.YoubanPublishVipActivityConfig{
		GiftNotifyEnabled: true,
		PayNotifyEnabled:  false,
	}
	if !tenantVipEventNotifyEnabled(tenantVipEventInviteFirstPay, cfg) {
		t.Fatal("gift notification should be enabled")
	}
	if tenantVipEventNotifyEnabled(tenantVipEventPay, cfg) {
		t.Fatal("pay notification should be disabled")
	}
}

func TestTenantVipActivityEventKey(t *testing.T) {
	baseKey := "bind_gift:100"
	if got := tenantVipActivityEventKey(baseKey, 1); got != baseKey {
		t.Fatalf("generation 1 key = %s, want %s", got, baseKey)
	}
	if got := tenantVipActivityEventKey(baseKey, 2); got != "bind_gift:100:g2" {
		t.Fatalf("generation 2 key = %s, want bind_gift:100:g2", got)
	}
}

func TestActivityAdminHandlersHaveUniqueCodes(t *testing.T) {
	seen := make(map[string]struct{})
	for _, handler := range activityAdminHandlers() {
		if handler == nil || handler.code == "" {
			t.Fatal("activity handler code must not be empty")
		}
		if _, exists := seen[handler.code]; exists {
			t.Fatalf("duplicate activity handler code: %s", handler.code)
		}
		seen[handler.code] = struct{}{}
	}
	if len(seen) != 3 {
		t.Fatalf("registered activity count = %d, want 3", len(seen))
	}
}

func TestApplyTenantVipBindActivityStatusUsesCurrentBinding(t *testing.T) {
	item := &sysin.TenantVipActivityModel{RewardCount: 1}
	applyTenantVipBindActivityStatus(item, false)
	if item.Completed {
		t.Fatal("historical reward must not be treated as current Telegram binding")
	}
	if item.StatusText != "奖励已领取，当前未绑定" {
		t.Fatalf("status text = %s", item.StatusText)
	}

	applyTenantVipBindActivityStatus(item, true)
	if !item.Completed {
		t.Fatal("current Telegram binding should be completed")
	}
	if item.StatusText != "已绑定，奖励已到账" {
		t.Fatalf("status text = %s", item.StatusText)
	}
}
