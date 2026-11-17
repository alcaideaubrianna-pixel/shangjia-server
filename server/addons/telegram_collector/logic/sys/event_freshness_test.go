package sys

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"
)

func TestFreshEventSchedulingIndependentOfTelemetry(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv("YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED", enabled)
			raw, err := collectorFreshEventPayload(context.Background(), 351, now)
			if err != nil {
				t.Fatal(err)
			}
			_, id, err := collectobserve.DecodeTask(context.Background(), raw, now)
			if err != nil || id != 351 {
				t.Fatalf("decode: id=%d err=%v", id, err)
			}
			if _, expired := collectorFreshTaskExpired(raw, now.Add(119*time.Second), 2*time.Minute); expired {
				t.Fatal("fresh task expired")
			}
			if age, expired := collectorFreshTaskExpired(raw, now.Add(2*time.Minute), 2*time.Minute); !expired || age != 2*time.Minute {
				t.Fatalf("expired=%v age=%s", expired, age)
			}
			if _, expired := collectorFreshTaskExpired(raw, now.Add(-time.Second), 2*time.Minute); expired {
				t.Fatal("future timestamp expired")
			}
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if len(fields["liveEnqueuedAtUnixMs"]) == 0 {
				t.Fatal("missing scheduling epoch")
			}
		})
	}
}

func TestFreshEventLegacyAndInvalidTiming(t *testing.T) {
	for _, raw := range []string{"351", `{}`, `{"liveEnqueuedAtUnixMs":0}`, `invalid`} {
		if _, expired := collectorFreshTaskExpired([]byte(raw), time.Now(), time.Minute); expired {
			t.Fatalf("legacy payload expired: %s", raw)
		}
	}
	for _, value := range []string{"", "invalid", "0", "-1", "3601"} {
		t.Setenv("YOUBAN_COLLECTOR_REALTIME_MAX_WAIT_SECONDS", value)
		if got := collectorFreshMaxWait(); got != 2*time.Minute {
			t.Fatalf("config %q: %s", value, got)
		}
	}
	t.Setenv("YOUBAN_COLLECTOR_REALTIME_MAX_WAIT_SECONDS", "30")
	if got := collectorFreshMaxWait(); got != 30*time.Second {
		t.Fatal(got)
	}
}
