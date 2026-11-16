package sys

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyCollectMaterialSourceKey(t *testing.T) {
	for _, tt := range []struct {
		identity string
		rule     int64
		want     string
	}{
		{"account:13:source:80:3974614787:group:14288735558729401", 48, "collect:account:13:source:80:3974614787:group:14288735558729401:48"},
		{"bot:54:-1003974614787:group:14288735558729401", 22, "collect:bot:54:-1003974614787:group:14288735558729401:22"},
		{"", 22, ""}, {"bot:54:group:1", 0, ""},
	} {
		if got := legacyCollectMaterialSourceKey(tt.identity, tt.rule); got != tt.want {
			t.Fatalf("key(%q,%d)=%q want %q", tt.identity, tt.rule, got, tt.want)
		}
	}
}

func TestLegacyCollectMaterialLookupRemainsScopedAndBounded(t *testing.T) {
	raw, err := os.ReadFile("collect_material_commit.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func existingLegacyCollectProfile(")
	if start < 0 {
		t.Fatal("legacy lookup not found")
	}
	end := strings.Index(source[start:], "\n// Historical keys")
	if end < 0 {
		t.Fatal("legacy lookup not found")
	}
	body := source[start : start+end]
	for _, forbidden := range []string{" LIKE ", "WhereLike(", "%:"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("substring lookup reintroduced: %s", forbidden)
		}
	}
	for _, required := range []string{
		"Where(e.TenantId, tenantID)", "Where(e.AccountId, accountID)",
		"Where(e.SourceGroupedId, groupedID)", "WhereIn(e.SourceChatId, chatIDs)",
		"Where(d.TenantId, tenantID)", "Where(d.AccountId, accountID)",
		"Limit(candidateLimit + 1)", "len(events) > candidateLimit", "len(dispatches) > candidateLimit",
		"context.WithTimeout(ctx, 5*time.Second)", "ps.tenant_id=? AND ps.account_id=?",
		"collectPublishClientRequestId(identity, nil)", "legacyCollectMaterialSourceKey(",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("missing identity safety contract: %s", required)
		}
	}
}
