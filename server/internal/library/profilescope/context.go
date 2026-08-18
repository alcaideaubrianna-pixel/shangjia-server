package profilescope

import "context"

type tenantScopeKey struct{}

type TenantScope struct {
	Applied   bool
	Trusted   bool
	TenantIds []int64
}

func WithTenantIds(ctx context.Context, tenantIds []int64) context.Context {
	copyIds := append([]int64(nil), tenantIds...)
	return context.WithValue(ctx, tenantScopeKey{}, TenantScope{Applied: true, TenantIds: copyIds})
}

func WithTrustedTenantIds(ctx context.Context, tenantIds []int64) context.Context {
	copyIds := append([]int64(nil), tenantIds...)
	return context.WithValue(ctx, tenantScopeKey{}, TenantScope{
		Applied:   true,
		Trusted:   true,
		TenantIds: copyIds,
	})
}

func FromContext(ctx context.Context) TenantScope {
	scope, _ := ctx.Value(tenantScopeKey{}).(TenantScope)
	return scope
}
