package sys

import (
	"context"
	"hotgo/addons/youban_publish/model/input/sysin"
	"reflect"
	"testing"
)

func TestAccountSettingPatchPreservesPermissions(t *testing.T) {
	current := &sysin.AccountSettingModel{AccountId: 42, EnableSuffix: 1, SuffixContent: "old", EnableTitleMark: 1, MarkMode: "custom", CustomMarkText: "prefix", NumberSource: "sequence", MarkPosition: "top", ProfileDetailViewEnabled: 1, SharedResourceEnabled: 1, TelegramBindingEnabled: 1, GroupPushEnabled: 1}
	save := accountSettingSaveInput(current)
	expected := *save
	expected.SuffixContent = "new"
	patch := &sysin.AccountSettingPatchInp{AccountId: 42, Field: "suffixContent", Value: "new"}
	if err := patch.Filter(context.Background()); err != nil {
		t.Fatal(err)
	}
	applyAccountSettingPatch(save, patch)
	if !reflect.DeepEqual(*save, expected) {
		t.Fatalf("unrelated fields changed: %+v", save)
	}
}

func TestAccountSettingPatchRejectsOwnershipFields(t *testing.T) {
	for _, field := range []string{"tenantId", "accountType", "deletedAt"} {
		in := &sysin.AccountSettingPatchInp{AccountId: 42, Field: field, Value: "1"}
		if in.Filter(context.Background()) == nil {
			t.Errorf("accepted forbidden field %s", field)
		}
	}
	for _, in := range []*sysin.AccountSettingPatchInp{
		{AccountId: 42, Field: "enableSuffix", Value: "2"},
		{AccountId: 42, Field: "markPosition", Value: "invalid"},
		{AccountId: 0, Field: "markMode", Value: "custom"},
	} {
		if in.Filter(context.Background()) == nil {
			t.Errorf("accepted invalid input %+v", in)
		}
	}
}

func TestAccountNotificationCannotDisableMembershipReminder(t *testing.T) {
	in := &sysin.AccountNotificationSettingInp{AccountId: 42, Kind: "vip_expiry", Enabled: 0}
	if in.Filter(context.Background()) == nil {
		t.Fatal("membership reminder must not be configurable")
	}
}

func TestAccountSettingPatchPermissionWhitelist(t *testing.T) {
	for _, field := range []string{"profileDetailViewEnabled", "sharedResourceEnabled", "telegramBindingEnabled", "groupPushEnabled"} {
		for _, value := range []string{"0", "1"} {
			in := &sysin.AccountSettingPatchInp{AccountId: 42, Field: field, Value: value}
			if err := in.Filter(context.Background()); err != nil {
				t.Fatal(err)
			}
			save := accountSettingSaveInput(&sysin.AccountSettingModel{AccountId: 42, SuffixContent: "unchanged"})
			applyAccountSettingPatch(save, in)
			if save.SuffixContent != "unchanged" {
				t.Fatal("permission patch overwrote suffix")
			}
			got := map[string]int{"profileDetailViewEnabled": save.ProfileDetailViewEnabled, "sharedResourceEnabled": save.SharedResourceEnabled, "telegramBindingEnabled": save.TelegramBindingEnabled, "groupPushEnabled": save.GroupPushEnabled}
			for key, actual := range got {
				want := 0
				if key == field && value == "1" {
					want = 1
				}
				if actual != want {
					t.Fatalf("field %s changed %s to %d", field, key, actual)
				}
			}
		}
		if (&sysin.AccountSettingPatchInp{AccountId: 42, Field: field, Value: "2"}).Filter(context.Background()) == nil {
			t.Fatal("nonboolean permission accepted")
		}
	}
}
