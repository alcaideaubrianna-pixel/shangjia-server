package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"hotgo/internal/library/collectobserve"
)

// bindCollectEventClock reads observation from the current, already loaded
// event. received_at is a business timestamp and is deliberately never read.
// media_json is the retained event payload; actual media live in event_media.
func bindCollectEventClock(ctx context.Context, event gdb.Record) context.Context {
	ctx = bindCollectEventObservation(ctx, event)
	return collectobserve.RestorePayloadClock(ctx, []byte(event["media_json"].String()), time.Now())
}
