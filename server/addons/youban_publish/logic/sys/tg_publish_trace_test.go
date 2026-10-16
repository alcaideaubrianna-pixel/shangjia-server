package sys

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

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
