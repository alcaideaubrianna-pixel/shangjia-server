package sys

import "testing"

func TestBotMediaSearchConcurrencyIsBounded(t *testing.T) {
	if botMediaSearchConcurrency < 2 || botMediaSearchConcurrency > 4 {
		t.Fatalf("unexpected bot media search concurrency: %d", botMediaSearchConcurrency)
	}
}
