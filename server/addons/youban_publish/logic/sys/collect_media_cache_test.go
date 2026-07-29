package sys

import "testing"

func TestCollectMediaErrorIsRateLimited(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "localized message", message: "TG媒体下载触发限流", want: true},
		{name: "too many requests", message: "rpc error: TOO MANY REQUESTS", want: true},
		{name: "flood wait", message: "FLOOD_WAIT_30", want: true},
		{name: "network timeout", message: "context deadline exceeded", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectMediaErrorIsRateLimited(test.message); got != test.want {
				t.Fatalf("collectMediaErrorIsRateLimited(%q) = %v, want %v", test.message, got, test.want)
			}
		})
	}
}

func TestCollectTelegramMediaCacheSource(t *testing.T) {
	item := collectMediaItem{FileId: "gotd:-100123:456"}
	if got := collectTelegramMediaCacheSource(item, 1); got != "gotd:global:gotd:-100123:456" {
		t.Fatalf("collectTelegramMediaCacheSource() = %q", got)
	}
	item.FileId = "bot-file-id"
	if got := collectTelegramMediaCacheSource(item, 7); got != "gotd:account:7:bot-file-id" {
		t.Fatalf("collectTelegramMediaCacheSource() fallback = %q", got)
	}
}
