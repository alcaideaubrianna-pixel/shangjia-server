package sys

import (
	"context"
	"strings"
	"testing"
)

func TestMediaFileCacheCDNURLsAreSeparated(t *testing.T) {
	ctx := context.Background()
	path := "hotgo/file/2026-09-14/example.mp4"
	workerURL := mediaFileCacheWorkerURL(ctx, path)
	fallbackURL := mediaFileCacheFallbackURL(ctx, path)
	if !strings.HasPrefix(workerURL, "https://sto.xiao-feiji.cc/") {
		t.Fatalf("unexpected worker URL: %q", workerURL)
	}
	if !strings.HasPrefix(fallbackURL, "https://cos.xiao-feiji.cc/") {
		t.Fatalf("unexpected fallback URL: %q", fallbackURL)
	}
}

func TestMediaFileCacheCDNURLAcceptsAbsoluteObjectURL(t *testing.T) {
	actual := mediaFileCacheWorkerURL(context.Background(), "https://origin.example/hotgo/file/example.jpg?token=secret")
	want := "https://sto.xiao-feiji.cc/hotgo/file/example.jpg"
	if actual != want {
		t.Fatalf("unexpected worker URL: got %q want %q", actual, want)
	}
}
