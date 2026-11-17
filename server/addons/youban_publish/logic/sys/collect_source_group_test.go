package sys

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectSourceGroupKeyNormalizesTelegramChatID(t *testing.T) {
	grouped := gdb.Record{
		"source_chat_id":    gvar.New("-1003982091537"),
		"source_grouped_id": gvar.New("14309746313903389"),
		"source_message_id": gvar.New(int64(3179)),
	}
	if got, want := collectSourceGroupKey(grouped), "3982091537:group:14309746313903389"; got != want {
		t.Fatalf("collectSourceGroupKey() = %q, want %q", got, want)
	}
	grouped["source_chat_id"] = gvar.New("3982091537")
	if got, want := collectSourceGroupKey(grouped), "3982091537:group:14309746313903389"; got != want {
		t.Fatalf("collectSourceGroupKey() without prefix = %q, want %q", got, want)
	}
}

func TestCollectRuleSourceIDSeparatesIngestAndRuleSources(t *testing.T) {
	event := gdb.Record{"source_id": gvar.New(int64(141))}
	rule := gdb.Record{collectRuleSourceIDField: gvar.New(int64(162))}
	if got := collectRuleSourceID(event, rule); got != 162 {
		t.Fatalf("rule source = %d, want 162", got)
	}
	if got := collectRuleSourceID(event, gdb.Record{}); got != 141 {
		t.Fatalf("fallback source = %d, want 141", got)
	}
}

func TestCollectRuleBindingDefaultsOnUntilExplicitlyDisabled(t *testing.T) {
	for _, status := range []int{-1, 0, 1} {
		if !collectRuleBindingEnabled(status) {
			t.Fatalf("legacy/default rule status %d should remain enabled", status)
		}
	}
	if collectRuleBindingEnabled(2) {
		t.Fatal("explicitly disabled rule status must not dispatch")
	}
}

func TestCollectEventRulesCacheVersionIsAccountScoped(t *testing.T) {
	first := collectEventRulesCacheAccountVersionKey(43, 520)
	second := collectEventRulesCacheAccountVersionKey(43, 521)
	if first == second {
		t.Fatalf("account cache versions must be isolated: %s", first)
	}
}

func TestMergeGlobalCollectTextPolicyDoesNotCreateDispatchRule(t *testing.T) {
	bound := gdb.Record{
		"delete_lines":               gvar.New([]string{"local-line"}),
		"delete_texts":               gvar.New([]string{"local-text"}),
		"replace_from":               gvar.New([]string{"local-from"}),
		"replace_to":                 gvar.New([]string{"local-to"}),
		"truncate_intro_fee_enabled": gvar.New(false),
		"intro_fee_suffix":           gvar.New(""),
	}
	global := gdb.Record{
		"delete_lines":               gvar.New([]string{"global-line"}),
		"delete_texts":               gvar.New([]string{"global-text"}),
		"replace_from":               gvar.New([]string{"global-from"}),
		"replace_to":                 gvar.New([]string{"global-to"}),
		"truncate_intro_fee_enabled": gvar.New(true),
		"intro_fee_suffix":           gvar.New("来风"),
		"header_enabled":             gvar.New(1),
		"header_markdown":            gvar.New("全局前置"),
		"footer_enabled":             gvar.New(1),
		"footer_markdown":            gvar.New("全局后置"),
	}
	mergeGlobalCollectTextPolicy([]gdb.Record{bound}, []gdb.Record{global})
	if got, want := collectRuleStrings(bound, "delete_lines"), []string{"global-line", "local-line"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("delete lines = %#v, want %#v", got, want)
	}
	if got, want := collectRuleStrings(bound, "delete_texts"), []string{"global-text", "local-text"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("delete texts = %#v, want %#v", got, want)
	}
	replacements := collectRuleReplacements(bound)
	if len(replacements) != 2 || replacements[0].From != "global-from" || replacements[1].From != "local-from" {
		t.Fatalf("replacements = %#v", replacements)
	}
	if !bound["truncate_intro_fee_enabled"].Bool() {
		t.Fatal("global text policy must enable intro fee truncation")
	}
	if got := bound["intro_fee_suffix"].String(); got != "来风" {
		t.Fatalf("intro fee suffix = %q, want 来风", got)
	}
	if got := bound["header_markdown"].String(); got != "全局前置" {
		t.Fatalf("header = %q", got)
	}
	if got := bound["footer_markdown"].String(); got != "全局后置" {
		t.Fatalf("footer = %q", got)
	}
}

func TestMergeGlobalCollectTextPolicyKeepsBoundSuffixOverride(t *testing.T) {
	bound := gdb.Record{"intro_fee_suffix": gvar.New("来源专用")}
	global := gdb.Record{"intro_fee_suffix": gvar.New("来风")}

	mergeGlobalCollectTextPolicy([]gdb.Record{bound}, []gdb.Record{global})

	if got := bound["intro_fee_suffix"].String(); got != "来源专用" {
		t.Fatalf("intro fee suffix = %q, want 来源专用", got)
	}
}

func TestCachedCollectRuleKeepsResolvedGlobalTextPolicy(t *testing.T) {
	resolved := gdb.Record{
		"delete_texts":     gvar.New([]string{"global", "local"}),
		"intro_fee_suffix": gvar.New("source-only"),
	}
	rows := collectEventRuleMapsToRecords([]g.Map{resolved.Map()})
	if len(rows) != 1 {
		t.Fatalf("cached rows = %d, want 1", len(rows))
	}
	if got, want := collectRuleStrings(rows[0], "delete_texts"), []string{"global", "local"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cached delete texts = %#v, want %#v", got, want)
	}
	if got := rows[0]["intro_fee_suffix"].String(); got != "source-only" {
		t.Fatalf("cached source suffix = %q", got)
	}
}

func TestCollectSourceGroupClaimsDoNotConsumeOtherRules(t *testing.T) {
	makeRule := func(id int64, channels ...int64) gdb.Record {
		return gdb.Record{"id": gvar.New(id), "target_channel_ids": gvar.New(channels)}
	}
	rules := []gdb.Record{makeRule(48, 101, 102), makeRule(49, 101, 102)}
	claims := []gdb.Record{{"rule_id": gvar.New(48), "channel_id": gvar.New(101)}}
	filtered := filterCollectRulesByBranchClaims(rules, claims)
	if len(filtered) != 2 {
		t.Fatalf("rule A claim removed rule B: %v", filtered)
	}
	if got := collectRuleTargetChannelIds(filtered[0]); !reflect.DeepEqual(got, []int64{102}) {
		t.Fatalf("rule A remaining channels=%v", got)
	}
	if got := collectRuleTargetChannelIds(filtered[1]); !reflect.DeepEqual(got, []int64{101, 102}) {
		t.Fatalf("rule B must keep both channels: %v", got)
	}
	if got := collectRuleTargetChannelIds(rules[0]); !reflect.DeepEqual(got, []int64{101, 102}) {
		t.Fatal("filter mutated cached rule targets")
	}
	// No in-memory reservation by rule A may remove B before either commits.
	filtered = filterCollectRulesByBranchClaims(rules, nil)
	if len(filtered) != 2 || len(collectRuleTargetChannelIds(filtered[1])) != 2 {
		t.Fatal("independent rules sharing a target must both dispatch")
	}
	// A successful same-branch claim suppresses only that branch/target.
	filtered = filterCollectRulesByBranchClaims([]gdb.Record{makeRule(48, 101), makeRule(49, 101)}, claims)
	if len(filtered) != 1 || filtered[0]["id"].Int64() != 49 {
		t.Fatal("same-branch claim must not suppress another rule")
	}
}

func TestCollectSourceGroupClaimLookupStaysBranchScopedAndBounded(t *testing.T) {
	raw, err := os.ReadFile("collect_channel_relation.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func (s *sSysPublish) filterCollectRulesByClaimedSourceGroup(")
	if start < 0 {
		t.Fatal("branch claim lookup not found")
	}
	end := strings.Index(source[start:], "\n// Claims are already scoped")
	if end < 0 {
		t.Fatal("branch claim lookup not found")
	}
	body := source[start : start+end]
	for _, required := range []string{
		"accountScope := collectEventAccountScope(event)",
		"Where(e.TenantId, tenantID)", "WhereIn(e.AccountId, accountScope)", "Where(e.SourceId, sourceID)",
		"WhereIn(\"d.account_id\", accountScope)", "Where(\"d.source_id\", sourceID)", "WhereIn(\"d.rule_id\", uniqueIds(ruleIDs))",
		"Limit(candidateLimit + 1)", "Limit(claimLimit + 1)", "context.WithTimeout(ctx, 5*time.Second)",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("missing source-group isolation contract: %s", required)
		}
	}
	if strings.Contains(body, "REPLACE(e.source_chat_id") {
		t.Fatal("source-group lookup must use exact Chat ID aliases")
	}
}
