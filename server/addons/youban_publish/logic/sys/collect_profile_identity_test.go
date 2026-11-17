package sys

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectPublishClientRequestIDIsolatesEveryBranch(t *testing.T) {
	base := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(6), "source_id": gvar.New(80),
		"source_type": gvar.New(sysin.CollectSourceTypeBot), "bot_id": gvar.New(54),
		"source_chat_id": gvar.New("3974614787"), "source_grouped_id": gvar.New("14288735558729401"),
	}
	rule := gdb.Record{"id": gvar.New(48)}
	want := "collect:v4:5:6:source:80:rule:48:-1003974614787:group:14288735558729401"
	if got := collectPublishClientRequestId(base, rule); got != want {
		t.Fatalf("identity=%q want=%q", got, want)
	}
	for _, field := range []string{"tenant_id", "account_id", "source_id"} {
		other := make(gdb.Record, len(base))
		for key, value := range base {
			other[key] = value
		}
		other[field] = gvar.New(base[field].Int64() + 1)
		if collectPublishClientRequestId(other, rule) == want {
			t.Fatalf("identity merged different %s", field)
		}
	}
	if collectPublishClientRequestId(base, gdb.Record{"id": gvar.New(49)}) == want {
		t.Fatal("two rules using the same Bot/channel must own different profiles")
	}
	base["source_chat_id"] = gvar.New("-1003974614787")
	if collectPublishClientRequestId(base, rule) != want {
		t.Fatal("Chat ID aliases must keep one branch identity")
	}
	if collectPublishClientRequestId(base, nil) != "" {
		t.Fatal("missing rule must not create a shared fallback identity")
	}
}

func TestCollectPublishClientRequestIDMessageAndFallbackStayScoped(t *testing.T) {
	event := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(6), "source_id": gvar.New(80),
		"source_chat_id": gvar.New("3974614787"), "source_message_id": gvar.New(1005),
	}
	rule := gdb.Record{"id": gvar.New(48)}
	if got := collectPublishClientRequestId(event, rule); got != "collect:v4:5:6:source:80:rule:48:-1003974614787:message:1005" {
		t.Fatalf("unexpected message key %q", got)
	}
	delete(event, "source_chat_id")
	delete(event, "source_message_id")
	event["source_unique_key"] = gvar.New("opaque-legacy-identity")
	first := collectPublishClientRequestId(event, rule)
	if first == "" || first == collectPublishClientRequestId(event, gdb.Record{"id": gvar.New(49)}) {
		t.Fatal("fallback identity must retain rule ownership")
	}
	event["source_id"] = gvar.New(81)
	if first == collectPublishClientRequestId(event, rule) {
		t.Fatal("fallback identity must retain source ownership")
	}
	delete(event, "source_unique_key")
	if collectPublishClientRequestId(event, rule) != "" {
		t.Fatal("unidentifiable input must not get a reusable profile key")
	}
}

func TestCollectProfileMaterialShouldNotDowngradeVerification(t *testing.T) {
	existing := gdb.Record{
		"has_verification_video": gvar.New(1), "image_count": gvar.New(4), "video_count": gvar.New(1),
	}
	if collectProfileMaterialShouldReplace(existing, 4, 0, 0) {
		t.Fatal("profile with verification video must not be replaced by incomplete material")
	}
	if !collectProfileMaterialShouldReplace(existing, 5, 2, 1) {
		t.Fatal("more complete verified material should replace existing material")
	}
}

func TestCollectBotZeroAccountBranchKeepsStorageIdentity(t *testing.T) {
	raw := gdb.Record{
		"id": gvar.New(100), "tenant_id": gvar.New(78), "account_id": gvar.New(0),
		"source_id": gvar.New(351), "source_type": gvar.New(sysin.CollectSourceTypeBot),
		"source_chat_id": gvar.New("3974614787"), "source_grouped_id": gvar.New("album"),
		"source_unique_key": gvar.New("bot:351:group:album"),
	}
	source := gdb.Record{"id": gvar.New(351), "tenant_id": gvar.New(78), "account_id": gvar.New(615)}
	branch, err := collectEventForBranch(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	if raw["account_id"].Int64() != 0 || raw[collectEventStorageAccountField] != nil {
		t.Fatal("business owner resolution mutated raw event")
	}
	if branch["account_id"].Int64() != 615 || collectEventStorageAccountID(branch) != 0 {
		t.Fatal("branch must separate business and persisted accounts")
	}
	if got := collectEventAccountScope(branch); !reflect.DeepEqual(got, []int64{615, 0}) {
		t.Fatalf("legacy account scope=%v", got)
	}
	key := collectPublishClientRequestId(branch, gdb.Record{"id": gvar.New(338)})
	if key != "collect:v4:78:615:source:351:rule:338:-1003974614787:group:album" {
		t.Fatalf("branch identity must use real owner: %s", key)
	}
	if collectPublishClientRequestId(raw, gdb.Record{"id": gvar.New(338)}) != "" {
		t.Fatal("raw account zero must not become business profile owner")
	}
	again, err := collectEventForBranch(branch, source)
	if err != nil || collectEventStorageAccountID(again) != 0 {
		t.Fatal("repeated branch derivation lost account zero")
	}
}

func TestCollectBranchOwnerRejectsCrossOwnership(t *testing.T) {
	base := gdb.Record{"id": gvar.New(1), "tenant_id": gvar.New(78), "account_id": gvar.New(615), "source_id": gvar.New(351), "source_type": gvar.New(sysin.CollectSourceTypeBot)}
	for _, source := range []gdb.Record{
		{"id": gvar.New(351), "tenant_id": gvar.New(79), "account_id": gvar.New(615)},
		{"id": gvar.New(352), "tenant_id": gvar.New(78), "account_id": gvar.New(615)},
		{"id": gvar.New(351), "tenant_id": gvar.New(78), "account_id": gvar.New(616)},
		{"id": gvar.New(351), "tenant_id": gvar.New(78), "account_id": gvar.New(0)},
	} {
		if _, err := collectEventForBranch(base, source); err == nil {
			t.Fatal("invalid source owner accepted")
		}
	}
	if !reflect.DeepEqual(collectEventAccountScope(base), []int64{615}) || collectEventStorageAccountID(base) != 615 {
		t.Fatal("normal account must retain one scope")
	}
	base["account_id"] = gvar.New(0)
	base["source_type"] = gvar.New(sysin.CollectSourceTypeAccount)
	if _, err := collectEventForBranch(base, gdb.Record{"id": gvar.New(351), "tenant_id": gvar.New(78), "account_id": gvar.New(615)}); err == nil {
		t.Fatal("zero account exception is limited to legacy Bot ingress")
	}
}

func TestCollectBranchStorageLockAndLegacyOperationContracts(t *testing.T) {
	raw, err := os.ReadFile("collect_event.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"branchEvent, branchErr := collectEventForBranch(event, source)",
		"s.dispatchCollectEventByRule(branchCtx, branchEvent, content, rule)",
		"Where(\"account_id\", collectEventStorageAccountID(event))",
		"findCollectBranchDispatch(ctx, tx.Model(",
		"validateCollectBranchProfileOwner(ctx, dispatch, event)",
	} {
		if !strings.Contains(string(raw), required) {
			t.Errorf("missing raw/branch ownership contract: %s", required)
		}
	}
	raw, err = os.ReadFile("collect_event_helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"WhereIn(\"account_id\", collectEventAccountScope(event))", "collect:%d",
		"states[0][\"account_id\"].Int64() != owner", "拒绝迁移或重发",
	} {
		if !strings.Contains(string(raw), required) {
			t.Errorf("missing accepted operation ownership check: %s", required)
		}
	}
}
