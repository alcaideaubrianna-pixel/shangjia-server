package profilescope

import (
	"context"
	"testing"
)

func TestWithTrustedTenantIdsCopiesAndMarksScope(t *testing.T) {
	ids := []int64{3, 7}
	ctx := WithTrustedTenantIds(context.Background(), ids)
	ids[0] = 99

	scope := FromContext(ctx)
	if !scope.Applied || !scope.Trusted {
		t.Fatalf("expected trusted applied scope, got %+v", scope)
	}
	if len(scope.TenantIds) != 2 || scope.TenantIds[0] != 3 || scope.TenantIds[1] != 7 {
		t.Fatalf("tenant ids were not copied: %+v", scope.TenantIds)
	}
}

func TestWithTenantIdsDoesNotGrantTrustedAccess(t *testing.T) {
	scope := FromContext(WithTenantIds(context.Background(), []int64{5}))
	if !scope.Applied || scope.Trusted {
		t.Fatalf("expected scoped non-trusted context, got %+v", scope)
	}
}
