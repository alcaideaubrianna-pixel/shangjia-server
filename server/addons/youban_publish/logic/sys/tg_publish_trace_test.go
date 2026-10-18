package sys

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramPublishTimingConcurrentAndIsolated(t *testing.T) {
	ctx := telegramPublishTraceContext(context.Background(), 42)
	timing := ctx.Value(telegramPublishTimingContextKey{}).(*telegramPublishTiming)
	mediaCtx := telegramPublishMediaTraceContext(ctx, &telegramMediaItem{Id: 1})
	if mediaCtx.Value(telegramPublishTimingContextKey{}) != timing {
		t.Fatal("nested media lost execution timing")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); timing.record("api.sendVideo", 2, nil); _ = timing.snapshot() }()
	}
	wg.Wait()
	timing.record("job.deferred", 0, []g.Map{{"reason": "channel_lease_busy"}})
	snapshot := timing.snapshot()
	if snapshot["stageDurationMs"].(map[string]int64)["api.sendVideo"] != 64 || snapshot["stageCounts"].(map[string]int)["api.sendVideo"] != 32 || snapshot["deferredReason"] != "channel_lease_busy" {
		t.Fatalf("lost timings: %+v", snapshot)
	}
	snapshot["stageDurationMs"].(map[string]int64)["api.sendVideo"] = -1
	if timing.snapshot()["stageDurationMs"].(map[string]int64)["api.sendVideo"] != 64 {
		t.Fatal("snapshot mutated shared state")
	}
	other := telegramPublishTraceContext(context.Background(), 43)
	if other.Value(telegramPublishTimingContextKey{}) == timing {
		t.Fatal("different executions share timing")
	}
}

func TestTelegramPublishTimingEndOnlyOnce(t *testing.T) {
	ctx := telegramPublishTraceContext(context.Background(), 42)
	step := beginTelegramPublishStage(ctx, "api.sendVideo")
	step.End(nil)
	step.End(nil)
	counts := ctx.Value(telegramPublishTimingContextKey{}).(*telegramPublishTiming).snapshot()["stageCounts"].(map[string]int)
	if counts["api.sendVideo"] != 1 {
		t.Fatalf("duplicate completion: %+v", counts)
	}
	if telegramPublishTimestampAge(nil) != 0 || telegramPublishTimestampAge(gtime.New(time.Now().Add(time.Hour))) != 0 {
		t.Fatal("missing/future timestamp should not produce negative age")
	}
}

func TestTelegramPublishMediaTraceIsolation(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := g.Map{"jobId": int64(42), "traceId": "trace-test", "executionId": "execution-test"}
	parent = context.WithValue(parent, telegramPublishTraceContextKey{}, base)
	first := telegramPublishMediaTraceContext(parent, &telegramMediaItem{Id: 1, Purpose: "verify", MediaType: "video", Size: 100})
	second := telegramPublishMediaTraceContext(parent, &telegramMediaItem{Id: 2, Purpose: "display", MediaType: "image"})
	for _, tc := range []struct {
		ctx context.Context
		id  int64
	}{{first, 1}, {second, 2}} {
		fields := tc.ctx.Value(telegramPublishTraceContextKey{}).(g.Map)
		if fields["jobId"] != int64(42) || fields["mediaId"] != tc.id || fields["traceId"] != "trace-test" {
			t.Fatalf("nested stage lost correlation: %+v", fields)
		}
		parentDeadline, _ := parent.Deadline()
		deadline, _ := tc.ctx.Deadline()
		if !deadline.Equal(parentDeadline) {
			t.Fatal("diagnostics changed the business deadline")
		}
	}
	if _, exists := base["mediaId"]; exists {
		t.Fatal("media context mutated parent shared by concurrent workers")
	}
	cancel()
	if first.Err() != context.Canceled || second.Err() != context.Canceled {
		t.Fatal("diagnostics detached cancellation")
	}
}

func TestTelegramPublishMediaFieldsExcludeSecrets(t *testing.T) {
	fields := telegramPublishMediaLogFields(&telegramMediaItem{
		FileUrl: "https://cdn.example/video.mp4?secret=private-signature", TgFileId: "private-file-id",
		PosterUrl: "https://cdn.example/poster.jpg?secret=private-signature", Size: 123,
	})
	if fields["sourceHost"] != "cdn.example" || fields["sizeBytes"] != int64(123) || fields["hasPoster"] != true {
		t.Fatalf("missing safe media diagnostics: %+v", fields)
	}
	for _, value := range fields {
		if str, ok := value.(string); ok && (strings.Contains(str, "private-file-id") || strings.Contains(str, "private-signature")) {
			t.Fatal("diagnostics exposed media credentials")
		}
	}
}
