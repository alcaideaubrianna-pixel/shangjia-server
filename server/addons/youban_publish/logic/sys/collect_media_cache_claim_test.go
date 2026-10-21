package sys

import (
	"context"
	"testing"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type completedDuringClaimCollector struct {
	collectorservice.ICollector
	reads    int
	complete bool
}

func (c *completedDuringClaimCollector) Enabled(context.Context) bool { return true }
func (c *completedDuringClaimCollector) MediaCache(context.Context, string) (*collectorin.MediaCacheEntry, bool, error) {
	c.reads++
	if c.reads > 1 && c.complete {
		return &collectorin.MediaCacheEntry{FileURL: "https://example.test/media.jpg", Status: collectorin.MediaStatusReady}, true, nil
	}
	return nil, false, nil
}
func (c *completedDuringClaimCollector) ClaimMediaProcessing(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func TestCollectMediaClaimRaceReusesCompletedResult(t *testing.T) {
	var original collectorservice.ICollector
	func() {
		defer func() { _ = recover() }()
		original = collectorservice.Collector()
	}()
	defer collectorservice.RegisterCollector(original)
	for _, complete := range []bool{true, false} {
		fake := &completedDuringClaimCollector{complete: complete}
		collectorservice.RegisterCollector(fake)
		state, err := acquireTelegramCollectorMediaCache(context.Background(), "photo", "", "", "aabd263ea2631da995dd88cc4e30789f", 98833, "image/jpeg")
		if complete {
			if err != nil || state == nil || state.Entry == nil || state.Claimed {
				t.Fatalf("completed result not reused: state=%+v err=%v", state, err)
			}
		} else if collectMediaRetryErrorFrom(err) == nil {
			t.Fatalf("active processing must remain retryable: %v", err)
		}
		if fake.reads != 2 {
			t.Fatalf("expected post-claim reread, got %d", fake.reads)
		}
	}
}
