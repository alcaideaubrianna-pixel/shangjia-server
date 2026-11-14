package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramChannelQueueKey(t *testing.T) {
	if got, want := telegramChannelQueueKey(254), "youban:tg:channel:queue:254"; got != want {
		t.Fatalf("queue key=%q, want %q", got, want)
	}
}

func TestTelegramChannelQueueRejectsInvalidIdentity(t *testing.T) {
	ctx := context.Background()
	if err := enqueueTelegramChannelJob(ctx, 0, 1, 1); err == nil {
		t.Fatal("expected invalid channel error")
	}
	if err := removeTelegramChannelJob(ctx, 1, 0); err == nil {
		t.Fatal("expected invalid job error")
	}
	if _, err := dequeueTelegramChannelHead(ctx, 0); err == nil {
		t.Fatal("expected invalid channel error")
	}
}

// This test is opt-in because it requires a disposable Redis instance. It
// verifies ZADD idempotency, score ordering, atomic head removal, and cleanup.
func TestTelegramChannelQueueRedisIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_CHANNEL_QUEUE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_CHANNEL_QUEUE_INTEGRATION=1 with disposable Redis")
	}
	ctx := context.Background()
	channelID := int64(990001)
	_, _ = g.Redis().Do(ctx, "DEL", telegramChannelQueueKey(channelID))
	t.Cleanup(func() { _, _ = g.Redis().Do(ctx, "DEL", telegramChannelQueueKey(channelID)) })
	if err := enqueueTelegramChannelJob(ctx, channelID, 2, 20); err != nil {
		t.Fatal(err)
	}
	if err := enqueueTelegramChannelJob(ctx, channelID, 1, 10); err != nil {
		t.Fatal(err)
	}
	if err := enqueueTelegramChannelJob(ctx, channelID, 1, 10); err != nil {
		t.Fatal(err)
	}
	if got, err := dequeueTelegramChannelHead(ctx, channelID); err != nil || got != 1 {
		t.Fatalf("first head=(%d,%v), want (1,nil)", got, err)
	}
	if got, err := dequeueTelegramChannelHead(ctx, channelID); err != nil || got != 2 {
		t.Fatalf("second head=(%d,%v), want (2,nil)", got, err)
	}
	if got, err := dequeueTelegramChannelHead(ctx, channelID); err != nil || got != 0 {
		t.Fatalf("empty head=(%d,%v), want (0,nil)", got, err)
	}
}
