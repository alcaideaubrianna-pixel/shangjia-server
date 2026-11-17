package sys

import (
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
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
		"Where(e.TenantId, tenantID)", "WhereIn(e.AccountId, accountScope)", "Where(e.SourceId, sourceID)",
		"Where(e.SourceGroupedId, strings.TrimSpace", "WhereIn(e.SourceChatId, tgChannelCacheLookupIds(chatID))",
		"Where(d.TenantId, tenantID)", "WhereIn(d.AccountId, accountScope)", "Where(d.SourceId, sourceID)", "Where(d.RuleId, ruleID)",
		"Limit(candidateLimit + 1)", "len(events) > candidateLimit", "len(dispatches) > candidateLimit",
		"context.WithTimeout(ctx, 5*time.Second)", "Where(\"tenant_id\", tenantID).Where(\"account_id\", accountID)",
		"legacyCollectProfileKeys(event, ruleID)", "selectLegacyCollectProfile(rows)",
		"WhereIn(\"o.profile_id\", profileIDs)", "Group(\"o.profile_id\")",
		"for _, id := range mappedIDs", "if !candidateIDs[id]",
		"accountScope := collectEventAccountScope(event)", "legacyCollectProfileKeys(storageEvent, ruleID)",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("missing identity safety contract: %s", required)
		}
	}
	candidateRead := strings.Index(body, "rows, err := model.OrderAsc(\"p.id\")")
	candidateIDs := strings.Index(body, "profileIDs := make([]int64, 0, len(rows))")
	originRead := strings.Index(body, "origins, readErr :=")
	if candidateRead < 0 || candidateIDs <= candidateRead || originRead <= candidateIDs {
		t.Fatal("origin evidence must be queried only after bounded profile candidates")
	}
	beforeCandidates := body[:candidateIDs]
	if strings.Contains(beforeCandidates, "YoubanPublishProfileOrigin.Table()") {
		t.Fatal("unbounded source/event origin lookup before profile candidates")
	}
}

func TestLegacyCollectProfileRequiresExplicitUnsharedBranch(t *testing.T) {
	row := func(id int64, mapped, foreign bool) gdb.Record {
		return gdb.Record{"id": gvar.New(id), "legacy_mapped": gvar.New(mapped), "legacy_foreign": gvar.New(foreign)}
	}
	for _, tc := range []struct {
		name    string
		rows    []gdb.Record
		wantID  int64
		wantErr bool
	}{
		{"new branch", nil, 0, false},
		{"explicit mapping", []gdb.Record{row(1, true, false)}, 1, false},
		{"other source or rule only", []gdb.Record{row(1, false, true)}, 0, false},
		{"unmapped old key refuses duplicate creation", []gdb.Record{row(1, false, false)}, 0, true},
		{"shared old profile refuses overwrite", []gdb.Record{row(1, true, true)}, 0, true},
		{"ambiguous same branch", []gdb.Record{row(1, true, false), row(2, true, false)}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectLegacyCollectProfile(tc.rows)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if err == nil && got["id"].Int64() != tc.wantID {
				t.Fatalf("id=%d want=%d", got["id"].Int64(), tc.wantID)
			}
		})
	}
	deleted := row(1, true, false)
	deleted["deleted_at"] = gvar.New("2026-11-17 00:00:00")
	if _, err := selectLegacyCollectProfile([]gdb.Record{deleted}); err == nil {
		t.Fatal("deleted legacy profile must not be automatically restored")
	}
	sent := row(1, true, false)
	sent["legacy_sent"] = gvar.New(true)
	got, err := selectLegacyCollectProfile([]gdb.Record{sent})
	if err != nil || !got["legacy_sent"].Bool() {
		t.Fatal("confirmed branch must preserve its terminal marker")
	}
}

func TestLegacyCollectProfileKeysRetainPreV4Aliases(t *testing.T) {
	event := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(6), "source_id": gvar.New(80),
		"source_type": gvar.New("bot"), "bot_id": gvar.New(54),
		"source_chat_id": gvar.New("3974614787"), "source_grouped_id": gvar.New("14288735558729401"),
		"source_unique_key": gvar.New("bot:54:-1003974614787:group:14288735558729401"),
	}
	keys := legacyCollectProfileKeys(event, 48)
	for _, want := range []string{
		"collect:bot:54:-1003974614787:group:14288735558729401:48",
		"collect:v2:5:6:-1003974614787:group:14288735558729401",
		"collect:v2:5:6:3974614787:group:14288735558729401",
		"collect:v3:5:6:bot:54:-1003974614787:group:14288735558729401",
	} {
		found := false
		for _, key := range keys {
			found = found || key == want
		}
		if !found {
			t.Fatalf("legacy key lost: %s", want)
		}
	}
}
