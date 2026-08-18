package opencontext

import "context"

type appKey struct{}

func WithAppId(ctx context.Context, appId string) context.Context {
	return context.WithValue(ctx, appKey{}, appId)
}

func AppId(ctx context.Context) string {
	value, _ := ctx.Value(appKey{}).(string)
	return value
}
