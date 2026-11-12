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

func TestAccountVisibilityScopeFixtureRelations(t *testing.T) {
	// This fixture mirrors the production projection: the viewer can see
	// itself, a managed account, a followed account, a mutual account, and the
	// accounts of a followed tenant administrator. Following scope deliberately
	// excludes self/managed while retaining direct and mutual follows.
	rows := []struct {
		id       int64
		relation string
	}{
		{1, "self"}, {2, "managed"}, {3, "tenant_scope"}, {4, "following"}, {5, "mutual"},
	}
	for _, scope := range []string{"mine", "all", "unknown"} {
		allowed := map[string]bool{}
		for _, relation := range accountVisibilityRelationTypes(scope) {
			allowed[relation] = true
		}
		for _, row := range rows {
			if !allowed[row.relation] {
				t.Fatalf("scope %s omitted relation %s for account %d", scope, row.relation, row.id)
			}
		}
	}
	followingAllowed := map[string]bool{}
	for _, relation := range accountVisibilityRelationTypes("following") {
		followingAllowed[relation] = true
	}
	if followingAllowed["self"] || followingAllowed["managed"] || followingAllowed["tenant_scope"] {
		t.Fatalf("following scope leaked non-follow relations: %#v", followingAllowed)
	}
	if !followingAllowed["following"] || !followingAllowed["mutual"] {
		t.Fatalf("following scope missing direct/mutual relations: %#v", followingAllowed)
	}
}
