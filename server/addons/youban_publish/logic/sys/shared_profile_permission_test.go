package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMarkSharedProfilePermissionAllowsEditing(t *testing.T) {
	capability := &sysin.AccountCapabilityModel{AccountId: 9, TenantId: 3, SharedResourceEnabled: 1}
	profile := &sysin.ProfileModel{AccountId: 3, TenantId: 3}

	markSharedProfilePermission(profile, capability)

	if profile.Permission != sysin.ProfilePermissionShared || !profile.CanEdit {
		t.Fatalf("shared profile permission=%q canEdit=%v", profile.Permission, profile.CanEdit)
	}
}

func TestMarkSharedProfilePermissionKeepsVisitorReadonly(t *testing.T) {
	capability := &sysin.AccountCapabilityModel{AccountId: 9, TenantId: 3, SharedResourceEnabled: 0}
	profile := &sysin.ProfileModel{AccountId: 3, TenantId: 3}

	markSharedProfilePermission(profile, capability)

	if profile.Permission != sysin.ProfilePermissionVisitor || profile.CanEdit {
		t.Fatalf("visitor profile permission=%q canEdit=%v", profile.Permission, profile.CanEdit)
	}
}
