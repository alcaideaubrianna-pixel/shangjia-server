package sys

import (
	"strings"
	"testing"
)

func TestMediaPHashBucketMediaTypeConditionUsesVisualMedia(t *testing.T) {
	condition, args := mediaPHashBucketMediaTypeCondition("b.media_type", "video")
	if condition != "b.media_type IN ('image', 'video')" {
		t.Fatalf("unexpected condition: %s", condition)
	}
	if len(args) != 0 {
		t.Fatalf("visual media condition should not have args: %#v", args)
	}
}

func TestMediaPHashBucketMediaTypeConditionKeepsUnknownTypesExact(t *testing.T) {
	condition, args := mediaPHashBucketMediaTypeCondition("b.media_type", "audio")
	if condition != "b.media_type = ?" {
		t.Fatalf("unexpected condition: %s", condition)
	}
	if len(args) != 1 || args[0] != "audio" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestMediaPHashDeduplicateProfilesKeepsBestMediaMatch(t *testing.T) {
	items := mediaPHashDeduplicateProfiles([]publishProfilePHashDistance{
		{ProfileId: 9, MediaId: 100, Distance: 8},
		{ProfileId: 9, MediaId: 101, Distance: 4},
		{ProfileId: 10, MediaId: 102, Distance: 6},
	})
	if len(items) != 2 {
		t.Fatalf("deduplicated matches = %d, want 2", len(items))
	}
	for _, item := range items {
		if item.ProfileId == 9 && (item.MediaId != 101 || item.Distance != 4) {
			t.Fatalf("profile 9 best match = %#v", item)
		}
	}
}

func TestMediaPHashBucketScopeSQLKeepsTenantScopeWithoutAccounts(t *testing.T) {
	condition, args := mediaPHashBucketScopeSQL("b", []mediaPHashBucketScopePart{{TenantId: 2}})
	if condition != "b.tenant_id = ?" {
		t.Fatalf("unexpected condition: %s", condition)
	}
	if len(args) != 1 || args[0] != int64(2) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestMediaPHashBucketScopeSQLKeepsAccountOnlyScope(t *testing.T) {
	condition, args := mediaPHashBucketScopeSQL("b", []mediaPHashBucketScopePart{{AccountIds: []int64{9}}})
	if condition != "b.account_id IN (?)" {
		t.Fatalf("unexpected condition: %s", condition)
	}
	if len(args) != 1 || args[0] != int64(9) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestMediaPHashBucketScopeSQLDropsEmptyScope(t *testing.T) {
	condition, args := mediaPHashBucketScopeSQL("b", []mediaPHashBucketScopePart{{}})
	if condition != "" || len(args) != 0 {
		t.Fatalf("empty scope must be rejected: condition=%q args=%#v", condition, args)
	}
}

func TestMediaPHashBucketBranchSQLUsesIndexableScope(t *testing.T) {
	query, args := mediaPHashBucketBranchSQL(1, "a", []mediaPHashBucketScopePart{{TenantId: 2, AccountIds: []int64{9, 10}}}, nil, "image", 0)
	for _, fragment := range []string{"b.bucket_pos = ?", "b.bucket_value = ?", "b.tenant_id = ?", "b.account_id IN (?,?)"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing %q: %s", fragment, query)
		}
	}
	if len(args) != 5 {
		t.Fatalf("unexpected args: %#v", args)
	}
}
