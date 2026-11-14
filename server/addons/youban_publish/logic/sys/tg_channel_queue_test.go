package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramChannelQueueKey(t *testing.T) {
	if got, want := telegramChannelQueueKey(254), "youban:tg:channel:queue:254"; got != want {
		t.Fatalf("queue key=%q, want %q", got, want)
	}
}

func TestTelegramChannelQueueScorePriorityAndFIFO(t *testing.T) {
	due := time.UnixMilli(1_700_000_000_000)
	manual := telegramChannelQueueScore(tgJobPriorityInteractive, due, 101)
	oldBulk := telegramChannelQueueScore(tgJobPriorityBulk, due.Add(-time.Hour), 1)
	if manual >= oldBulk {
		t.Fatalf("manual score=%d must precede older bulk score=%d", manual, oldBulk)
	}
	first := telegramChannelQueueScore(tgJobPriorityInteractive, due, 101)
	second := telegramChannelQueueScore(tgJobPriorityInteractive, due, 102)
	if first >= second {
		t.Fatalf("same-priority FIFO scores: first=%d second=%d", first, second)
	}
	future := telegramChannelQueueScore(tgJobPriorityInteractive, due.Add(time.Second), 103)
	if future <= second {
		t.Fatalf("future retry score=%d must remain after due score=%d", future, second)
	}
}

func TestTelegramChannelCursorNext(t *testing.T) {
	channels := []telegramChannelQueueChannel{
		{ChannelID: 1672},
		{ChannelID: 1673},
		{ChannelID: 3201},
	}
	if got := telegramChannelCursorNext(1500, channels); got != 3201 {
		t.Fatalf("cursor=%d, want 3201", got)
	}
	if got := telegramChannelCursorNext(4000, channels); got != 4000 {
		t.Fatalf("cursor=%d, want current cursor 4000 when no larger channel exists", got)
	}
	if got := telegramChannelCursorNext(0, nil); got != 0 {
		t.Fatalf("empty cursor=%d, want 0", got)
	}
}

func TestTelegramChannelQueueRejectsInvalidIdentity(t *testing.T) {
	ctx := context.Background()
	if err := enqueueTelegramChannelJob(ctx, 0, 1, tgJobPriorityInteractive, time.Now()); err == nil {
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
	if err := enqueueTelegramChannelJob(ctx, channelID, 2, tgJobPriorityBulk, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := enqueueTelegramChannelJob(ctx, channelID, 1, tgJobPriorityInteractive, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := enqueueTelegramChannelJob(ctx, channelID, 1, tgJobPriorityInteractive, time.Now()); err != nil {
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

func TestTelegramChannelQueueDifferentChannelsAreIndependent(t *testing.T) {
	if os.Getenv("YOUBAN_CHANNEL_QUEUE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_CHANNEL_QUEUE_INTEGRATION=1 with disposable Redis")
	}
	ctx := context.Background()
	channelA, channelB := int64(990002), int64(990003)
	for _, channelID := range []int64{channelA, channelB} {
		_, _ = g.Redis().Do(ctx, "DEL", telegramChannelQueueKey(channelID))
		t.Cleanup(func() { _, _ = g.Redis().Do(ctx, "DEL", telegramChannelQueueKey(channelID)) })
	}
	if err := enqueueTelegramChannelJob(ctx, channelA, 11, tgJobPriorityInteractive, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := enqueueTelegramChannelJob(ctx, channelB, 21, tgJobPriorityInteractive, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := dequeueTelegramChannelHead(ctx, channelA); err != nil || got != 11 {
		t.Fatalf("channel A head=(%d,%v), want (11,nil)", got, err)
	}
	if got, err := dequeueTelegramChannelHead(ctx, channelB); err != nil || got != 21 {
		t.Fatalf("channel B head=(%d,%v), want (21,nil)", got, err)
	}
}

func TestTelegramChannelQueueWorkerDisabledByDefault(t *testing.T) {
	t.Setenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED", "false")
	if telegramChannelQueueEnabled() {
		t.Fatal("channel queue worker must remain disabled unless explicitly enabled")
	}
}
