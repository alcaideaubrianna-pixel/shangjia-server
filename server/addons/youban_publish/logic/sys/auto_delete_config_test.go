package sys

import (
	"reflect"
	"testing"

	"hotgo/addons/youban_publish/model"
)

func TestMergeAutoDeleteStrings(t *testing.T) {
	got := mergeAutoDeleteStrings(
		[]string{"系统默认", "重复词"},
		[]string{" 自定义 ", "重复词", "系统默认"},
	)
	want := []string{"系统默认", "重复词", "自定义"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeAutoDeleteStrings() = %#v, want %#v", got, want)
	}
}

func TestDecodeAutoDeleteInt64JSON(t *testing.T) {
	got := decodeAutoDeleteInt64JSON(`[8,6,8,0,-1]`)
	want := []int64{8, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeAutoDeleteInt64JSON() = %#v, want %#v", got, want)
	}
}

func TestAutoDeleteBotAllowedRequiresExplicitTenantConfig(t *testing.T) {
	if autoDeleteBotAllowed(8, nil) {
		t.Fatal("empty tenant Bot configuration must not authorize a Bot")
	}
	if autoDeleteBotAllowed(8, []int64{6}) {
		t.Fatal("foreign Bot must not be authorized")
	}
	if !autoDeleteBotAllowed(6, []int64{6}) {
		t.Fatal("configured Bot should be authorized")
	}
}

func TestTenantAutoDeleteConfigFromLegacyKeepsOnlyTenantBots(t *testing.T) {
	legacy := &model.AutoDeleteConfig{Enabled: 1, BotIds: []int64{8, 6}}
	tenantTwo := tenantAutoDeleteConfigFromLegacy(legacy, []int64{6})
	tenantNine := tenantAutoDeleteConfigFromLegacy(legacy, []int64{8})
	if !reflect.DeepEqual(tenantTwo.BotIds, []int64{6}) || tenantTwo.Enabled != 1 {
		t.Fatalf("tenant 2 migration = %#v", tenantTwo)
	}
	if !reflect.DeepEqual(tenantNine.BotIds, []int64{8}) || tenantNine.Enabled != 1 {
		t.Fatalf("tenant 9 migration = %#v", tenantNine)
	}
	emptyTenant := tenantAutoDeleteConfigFromLegacy(legacy, nil)
	if emptyTenant.Enabled != 0 || len(emptyTenant.BotIds) != 0 {
		t.Fatalf("tenant without owned Bots must stay disabled: %#v", emptyTenant)
	}
}
