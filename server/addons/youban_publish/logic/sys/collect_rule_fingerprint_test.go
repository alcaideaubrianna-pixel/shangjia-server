package sys

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectProfileFingerprintScopeSeparatesBranches(t *testing.T) {
	raw := buildProfileFingerprints([]int64{9}, "same material", []collectMediaItem{{Type: "image", FileMd5: "same-md5", FilePhash: "0123456789abcdef"}})
	before := append([]profileFingerprint(nil), raw...)
	first := scopeCollectProfileFingerprints("collect:v4:5:6:source:80:rule:48:-10042:group:100", raw)
	if len(first) == 0 || !reflect.DeepEqual(raw, before) {
		t.Fatal("scoping must preserve raw input for legacy lookup")
	}
	for _, key := range []string{
		"collect:v4:7:6:source:80:rule:48:-10042:group:100",
		"collect:v4:5:7:source:80:rule:48:-10042:group:100",
		"collect:v4:5:6:source:81:rule:48:-10042:group:100",
		"collect:v4:5:6:source:80:rule:49:-10042:group:100",
	} {
		other := scopeCollectProfileFingerprints(key, raw)
		for index := range first {
			if first[index].Signature == other[index].Signature {
				t.Fatalf("fingerprint merged distinct branch %s", key)
			}
		}
	}
	for index, item := range first {
		if len(item.Signature) != 64 || item.Signature == raw[index].Signature {
			t.Fatal("scoped exact signature must fit varchar(64) and differ from raw")
		}
		item.Signature = raw[index].Signature
		if item != raw[index] {
			t.Fatal("scoping must not change layer/channel/counts or pHash media")
		}
	}
	first[0].Signature = "changed"
	if !reflect.DeepEqual(raw, before) {
		t.Fatal("scoped result aliases the raw input")
	}
}

func TestCollectProfileFingerprintScopeIgnoresMessageIdentity(t *testing.T) {
	raw := []profileFingerprint{{ChannelID: 9, Layer: "text_hash", Signature: "same-text", ItemTotal: 1, SignatureCount: 1}}
	first := scopeCollectProfileFingerprints("collect:v4:5:6:source:80:rule:48:-10042:group:100", raw)
	for _, key := range []string{
		"collect:v4:5:6:source:80:rule:48:-10042:group:200",
		"collect:v4:5:6:source:80:rule:48:-10042:message:300",
		"collect:v4:5:6:source:80:rule:48:key:opaque",
	} {
		if !reflect.DeepEqual(first, scopeCollectProfileFingerprints(key, raw)) {
			t.Fatalf("same branch must dedupe across groups/messages: %s", key)
		}
	}
}

func TestCollectProfileFingerprintScopeLeavesOrdinaryProfilesUnchanged(t *testing.T) {
	raw := []profileFingerprint{{ChannelID: 9, Layer: "phash_exact", Signature: "0123456789abcdef", ItemTotal: 1, SignatureCount: 1}}
	for _, key := range []string{"", "manual:42", "collect:v2:5:6:-10042:group:100", "collect:v3:5:6:bot:54:-10042:group:100"} {
		got := scopeCollectProfileFingerprints(key, raw)
		if !reflect.DeepEqual(got, raw) {
			t.Fatalf("ordinary/legacy profile changed: %s", key)
		}
		got[0].Signature = "modified copy"
		if raw[0].Signature != "0123456789abcdef" {
			t.Fatal("ordinary result must also be an independent copy")
		}
	}
	if scopeCollectProfileFingerprints("", nil) != nil {
		t.Fatal("nil input must remain nil")
	}
}

func TestCollectProfileFingerprintNamespaceRejectsIncompleteScope(t *testing.T) {
	for _, key := range []string{
		"collect:v4:5:6:source:80:rule:48",
		"collect:v4:5:6:source:0:rule:48:group:1",
		"collect:v4:5:6:source:80:rule:0:group:1",
		"collect:v4:5:6:bot:80:rule:48:group:1",
	} {
		if collectProfileFingerprintNamespace(key) != "" {
			t.Fatalf("accepted incomplete namespace %q", key)
		}
	}
}

func TestLegacyCollectRuleFingerprintLookupRejectsUnscopedBeforeDatabase(t *testing.T) {
	s := &sSysPublish{}
	items := []profileFingerprint{{ChannelID: 9, Layer: "text_hash", Signature: "raw"}}
	if _, err := s.findLegacyCollectRuleDuplicate(context.Background(), nil, nil, items); err == nil {
		t.Fatal("unscoped lookup must fail before touching database")
	}
	rawBot := gdb.Record{
		"tenant_id": gvar.New(78), "account_id": gvar.New(0),
		"source_id": gvar.New(351), "source_type": gvar.New("bot"),
	}
	if _, err := s.findLegacyCollectRuleDuplicate(context.Background(), rawBot, gdb.Record{"id": gvar.New(338)}, items); err == nil {
		t.Fatal("raw Bot account 0 must not be treated as a resolved business profile owner")
	}
	event := gdb.Record{"tenant_id": gvar.New(5), "account_id": gvar.New(6), "source_id": gvar.New(80)}
	rule := gdb.Record{"id": gvar.New(48), collectRuleSourceIDField: gvar.New(81)}
	if _, err := s.findLegacyCollectRuleDuplicate(context.Background(), event, rule, items); err == nil {
		t.Fatal("lookup must not substitute another source")
	}
	items = make([]profileFingerprint, 257)
	rule = gdb.Record{"id": gvar.New(48)}
	if _, err := s.findLegacyCollectRuleDuplicate(context.Background(), event, rule, items); err == nil {
		t.Fatal("oversized raw lookup must fail before touching database")
	}
}

func TestCollectProfileFingerprintProjectionUsesPersistedScope(t *testing.T) {
	raw, err := os.ReadFile("profile_fingerprint.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Fields(profileColumns.PlainText, profileColumns.SourceKey)",
		"p.id,p.plain_text,p.source_key,ps.tenant_id,ps.account_id",
		"scopeCollectProfileFingerprints(profile[\"source_key\"].String(), buildProfileFingerprints(",
		"scopeCollectProfileFingerprints(row[\"source_key\"].String(), buildProfileFingerprints(",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("generic refresh/backfill lost branch scope: %s", required)
		}
	}
	raw, err = os.ReadFile("collect_material_commit.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "scopeCollectProfileFingerprints(sourceKey, buildProfileFingerprints(") {
		t.Fatal("material commit must attach the same scoped signatures as lookup")
	}
}

func TestLegacyCollectRuleFingerprintLookupUsesExactBranchEvidence(t *testing.T) {
	raw, err := os.ReadFile("collect_rule_fingerprint.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func (s *sSysPublish) findLegacyCollectRuleDuplicate(")
	if start < 0 {
		t.Fatal("legacy lookup not found")
	}
	body := source[start:]
	for _, required := range []string{
		"f.channel_id=? AND f.layer=? AND f.signature=? AND f.item_total=? AND f.signature_count=?",
		"accountScope := collectEventAccountScope(event)",
		"Where(\"f.tenant_id\", tenantID)", "WhereIn(\"f.account_id\", accountScope)",
		"ps.profile_id=p.id AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)\", tenantID, accountID",
		"d.profile_id=f.profile_id AND d.tenant_id=? AND d.account_id IN (?) AND d.source_id=? AND d.rule_id=?",
		"other.account_id NOT IN (?)", "cp.account_id NOT IN (?)",
		"p.source_key NOT LIKE ?", "collect:v4:%", "Limit(1)",
		"context.WithTimeout(ctx, 3*time.Second)", "len(items) > fingerprintLimit",
		"NOT EXISTS (SELECT 1 FROM ",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("missing legacy fingerprint safety contract: %s", required)
		}
	}
	for _, forbidden := range []string{".Update(", ".Save(", ".Insert(", "backfillProfileFingerprints(", "cache.Instance()"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("legacy lookup must remain read-only: %s", forbidden)
		}
	}
}
