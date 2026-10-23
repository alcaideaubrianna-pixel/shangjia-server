package sys

import (
	"strings"
	"testing"
)

func TestPublicProfileScopeRequiresEnabledAccountForOpenAPI(t *testing.T) {
	condition := publicProfileScopeExistsCondition("id", true)
	for _, fragment := range []string{
		"hg_youban_publish_profile_state yps",
		"hg_youban_publish_account ypa",
		"ypa.id=yps.account_id",
		"ypa.tenant_id=yps.tenant_id",
		"ypa.status=1",
		"ypa.deleted_at IS NULL",
		"yps.deleted_at IS NULL",
	} {
		if !strings.Contains(condition, fragment) {
			t.Fatalf("enabled-account scope is missing %q: %s", fragment, condition)
		}
	}
}

func TestPublicProfileScopeKeepsLegacyTenantOnlyBehavior(t *testing.T) {
	condition := publicProfileScopeExistsCondition("id", false)
	if strings.Contains(condition, "hg_youban_publish_account") {
		t.Fatalf("ordinary tenant scope unexpectedly filters publish accounts: %s", condition)
	}
}
