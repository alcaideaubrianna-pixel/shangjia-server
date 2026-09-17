package sys

import "testing"

func TestBotMediaSearchConcurrencyIsBounded(t *testing.T) {
	if botMediaSearchConcurrency < 2 || botMediaSearchConcurrency > 4 {
		t.Fatalf("unexpected bot media search concurrency: %d", botMediaSearchConcurrency)
	}
}

func TestBotMediaFingerprintCacheKeyUsesStableTelegramId(t *testing.T) {
	if got := botMediaFingerprintCacheKey("  unique-id  "); got != "youban_publish:bot_media_fingerprint:v1:unique-id" {
		t.Fatalf("unexpected cache key: %q", got)
	}
}
