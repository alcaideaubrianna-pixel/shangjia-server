package profilescope

import "context"

type tenantScopeKey struct{}

type TenantScope struct {
	Applied               bool
	Trusted               bool
	RequireEnabledAccount bool
	TenantIds             []int64
}

func WithTenantIds(ctx context.Context, tenantIds []int64) context.Context {
	copyIds := append([]int64(nil), tenantIds...)
	return context.WithValue(ctx, tenantScopeKey{}, TenantScope{Applied: true, TenantIds: copyIds})
}

func WithTrustedTenantIds(ctx context.Context, tenantIds []int64) context.Context {
	return withTrustedTenantIds(ctx, tenantIds, false)
}

func WithTrustedEnabledAccountTenantIds(ctx context.Context, tenantIds []int64) context.Context {
	return withTrustedTenantIds(ctx, tenantIds, true)
}

func withTrustedTenantIds(ctx context.Context, tenantIds []int64, requireEnabledAccount bool) context.Context {
	copyIds := append([]int64(nil), tenantIds...)
	return context.WithValue(ctx, tenantScopeKey{}, TenantScope{
		Applied:               true,
		Trusted:               true,
		RequireEnabledAccount: requireEnabledAccount,
		TenantIds:             copyIds,
	})
}

func FromContext(ctx context.Context) TenantScope {
	scope, _ := ctx.Value(tenantScopeKey{}).(TenantScope)
	return scope
}
