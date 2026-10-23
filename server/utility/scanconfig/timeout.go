// Package scanconfig shares scan execution budgets between ingress and search.
package scanconfig

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// QueryTimeout bounds candidate queries for one search, not each image.
func QueryTimeout(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.mediaSearch.queryTimeoutSeconds", 5).Int()
	if seconds <= 0 || seconds > 600 {
		seconds = 5
	}
	return time.Duration(seconds) * time.Second
}

// SearchTimeout ensures an explicit longer query budget is not cut off at 45s.
func SearchTimeout(ctx context.Context) time.Duration {
	return max(45*time.Second, QueryTimeout(ctx))
}
