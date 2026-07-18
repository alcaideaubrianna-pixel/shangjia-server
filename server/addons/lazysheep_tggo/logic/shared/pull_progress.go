package shared

import (
	"context"
	"strings"
)

type pullProgressKey struct{}

type PullProgressReporter func(string)

func WithPullProgressReporter(ctx context.Context, reporter PullProgressReporter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if reporter == nil {
		return ctx
	}
	return context.WithValue(ctx, pullProgressKey{}, reporter)
}

func ReportPullProgress(ctx context.Context, text string) {
	if ctx == nil {
		return
	}
	if reporter, ok := ctx.Value(pullProgressKey{}).(PullProgressReporter); ok && reporter != nil {
		reporter(strings.TrimSpace(text))
	}
}
