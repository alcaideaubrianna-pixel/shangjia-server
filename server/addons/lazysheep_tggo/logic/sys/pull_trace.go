package sys

import "context"

type pullTraceKey struct{}

func withPullTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, pullTraceKey{}, traceID)
}

func pullTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(pullTraceKey{}).(string); ok {
		return v
	}
	return ""
}

func pullTraceTag(ctx context.Context) string {
	if id := pullTraceID(ctx); id != "" {
		return "[TGGO_PULL_TRACE " + id + "]"
	}
	return "[TGGO_PULL_TRACE]"
}
