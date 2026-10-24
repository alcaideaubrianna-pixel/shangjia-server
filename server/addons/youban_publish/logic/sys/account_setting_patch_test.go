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

func TestAccountSettingPatchRejectsPrivilegeFields(t *testing.T) {
	for _, field := range []string{"tenantId", "accountType", "sharedResourceEnabled", "telegramBindingEnabled", "deletedAt"} {
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
