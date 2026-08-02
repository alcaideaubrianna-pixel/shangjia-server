package sys

import (
	"reflect"
	"testing"

	publishmodel "hotgo/addons/youban_publish/model"
)

func TestMediaSearchScopeFromPartitionsNormalizesOrderAndDuplicates(t *testing.T) {
	scope := mediaSearchScopeFromPartitions([]publishmodel.MediaSearchScopePartition{
		{TenantId: 3, AccountIds: []int64{9, 8, 9}},
		{TenantId: 2, AccountIds: []int64{5}},
		{TenantId: 3, AccountIds: []int64{7}},
		{TenantId: 0, AccountIds: []int64{1}},
	})
	wantAccounts := []int64{5, 7, 8, 9}
	if !reflect.DeepEqual(scope.AccountIds, wantAccounts) {
		t.Fatalf("account ids = %#v, want %#v", scope.AccountIds, wantAccounts)
	}
	wantPartitions := []publishmodel.MediaSearchScopePartition{
		{TenantId: 2, AccountIds: []int64{5}},
		{TenantId: 3, AccountIds: []int64{7, 8, 9}},
	}
	if !reflect.DeepEqual(scope.Partitions, wantPartitions) {
		t.Fatalf("partitions = %#v, want %#v", scope.Partitions, wantPartitions)
	}
}

func TestMediaSearchScopeCacheKeyIsStable(t *testing.T) {
	left := mediaSearchScopeFromPartitions([]publishmodel.MediaSearchScopePartition{
		{TenantId: 3, AccountIds: []int64{9, 8}},
		{TenantId: 2, AccountIds: []int64{5}},
	})
	right := mediaSearchScopeFromPartitions([]publishmodel.MediaSearchScopePartition{
		{TenantId: 2, AccountIds: []int64{5, 5}},
		{TenantId: 3, AccountIds: []int64{8, 9}},
	})
	if mediaSearchScopeCacheKey(left) != mediaSearchScopeCacheKey(right) {
		t.Fatalf("equivalent scopes must share the same cache key")
	}
}

func TestMediaSearchScopeTenantIdRequiresSinglePartition(t *testing.T) {
	if got := mediaSearchScopeTenantId(mediaSearchScopeForTenant(2, []int64{5})); got != 2 {
		t.Fatalf("single tenant id = %d, want 2", got)
	}
	if got := mediaSearchScopeTenantId(mediaSearchScopeFromPartitions([]publishmodel.MediaSearchScopePartition{
		{TenantId: 2, AccountIds: []int64{5}},
		{TenantId: 3, AccountIds: []int64{8}},
	})); got != 0 {
		t.Fatalf("multi-tenant scope must not collapse to tenant %d", got)
	}
}
