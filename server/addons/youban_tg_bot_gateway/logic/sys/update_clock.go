package sys

import (
	"context"
	"time"

	"hotgo/internal/library/collectobserve"
)

// gatewayUpdateClock restores the existing epoch: new webhook tasks sample
// at Webhook entry, while polling and legacy tasks sample at gateway enqueue.
// The conservative basis avoids labeling polling/legacy tasks as HTTP receipt.
func gatewayUpdateClock(ctx context.Context, enqueuedAt int64, timeBasis string, consumedAt time.Time) context.Context {
	ctx = collectobserve.RestorePayloadClock(ctx, nil, consumedAt)
	if enqueuedAt <= 0 || consumedAt.IsZero() || enqueuedAt > consumedAt.UnixNano() {
		return ctx
	}
	return collectobserve.With(ctx, collectobserve.Metadata{
		ReceivedAt: time.Unix(0, enqueuedAt), TimeBasis: timeBasis, Mode: "realtime",
	})
}
