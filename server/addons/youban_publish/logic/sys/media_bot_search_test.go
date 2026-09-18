package sys

import "testing"

func TestBotMediaSearchResultLimitIsBounded(t *testing.T) {
	if botMediaSearchMaxResults <= 0 || botMediaSearchMaxResults > 20 {
		t.Fatalf("unexpected bot media search result limit: %d", botMediaSearchMaxResults)
	}
}

func TestBotMediaFingerprintCacheKeyUsesStableTelegramId(t *testing.T) {
	if got := botMediaFingerprintCacheKey("  unique-id  "); got != "youban_publish:bot_media_fingerprint:v1:unique-id" {
		t.Fatalf("unexpected cache key: %q", got)
	}
}
