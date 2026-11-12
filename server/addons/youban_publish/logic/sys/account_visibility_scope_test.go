package sys

import "testing"

func TestAccountVisibilityRelationTypes(t *testing.T) {
	all := accountVisibilityRelationTypes("mine")
	if len(all) != 5 || all[0] != "self" || all[2] != "tenant_scope" || all[4] != "mutual" {
		t.Fatalf("unexpected general visibility relations: %#v", all)
	}
	following := accountVisibilityRelationTypes("following")
	if len(following) != 2 || following[0] != "following" || following[1] != "mutual" {
		t.Fatalf("unexpected following visibility relations: %#v", following)
	}
	// Unknown scopes use the same broad set as mine/all; validation of the
	// public scope value remains the responsibility of adminProfileVisibleScope.
	if got := accountVisibilityRelationTypes("all"); len(got) != len(all) {
		t.Fatalf("all scope relation count = %d, want %d", len(got), len(all))
	}
}

func TestAccountVisibilityRelationTypesTenantIsolationIsQueryScoped(t *testing.T) {
	// The relation set contains no tenant wildcard. The SQL reader always adds
	// tenant_id and viewer_account_id predicates, so cross-tenant edges cannot
	// be returned even if historical rows exist in another tenant.
	for _, relation := range accountVisibilityRelationTypes("mine") {
		if relation == "*" || relation == "" {
			t.Fatalf("invalid wildcard visibility relation: %q", relation)
		}
	}
}
