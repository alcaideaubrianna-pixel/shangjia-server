package sys

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/internal/library/collectobserve"
)

// Scheduling time is independent of the optional observation carrier. A
// retry retains this timestamp; recovery creates a history task instead.
type freshEventTask struct {
	collectobserve.TaskCarrier
	LiveEnqueuedAtUnixMs int64 `json:"liveEnqueuedAtUnixMs"`
}

func collectorFreshEventPayload(ctx context.Context, eventID int64, now time.Time) ([]byte, error) {
	observationCtx := context.Background()
	if enabled, _ := strconv.ParseBool(os.Getenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED")); enabled {
		observationCtx = ctx
	}
	raw, err := collectobserve.EncodeTask(observationCtx, eventID, now)
	if err != nil {
		return nil, err
	}
	var payload freshEventTask
	if err = json.Unmarshal(raw, &payload.TaskCarrier); err != nil {
		return nil, gerror.Wrap(err, "编码实时采集任务失败")
	}
	payload.LiveEnqueuedAtUnixMs = now.UnixMilli()
	return json.Marshal(payload)
}

func collectorFreshMaxWait() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("YOUBAN_COLLECTOR_REALTIME_MAX_WAIT_SECONDS"))
	if err != nil || seconds < 5 || seconds > 3600 {
		seconds = 120
	}
	return time.Duration(seconds) * time.Second
}

func collectorFreshTaskExpired(raw []byte, now time.Time, maxWait time.Duration) (time.Duration, bool) {
	var payload freshEventTask
	if json.Unmarshal(raw, &payload) != nil || payload.LiveEnqueuedAtUnixMs <= 0 || maxWait <= 0 {
		return 0, false
	}
	age := now.Sub(time.UnixMilli(payload.LiveEnqueuedAtUnixMs))
	return age, age >= maxWait
}
