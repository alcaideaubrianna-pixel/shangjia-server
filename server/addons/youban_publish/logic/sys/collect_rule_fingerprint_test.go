package sys

import (
	"os"
	"reflect"
	"strings"
	"testing"
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
