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

func TestMediaFileCacheRemoteSourcesRecoverObjectPathFromLegacyURL(t *testing.T) {
	ctx := context.Background()
	legacyURL := "https://retired.invalid/hotgo/file/2026-09-14/example.jpg"
	sources, err := mediaFileCacheRemoteSources(ctx, &telegramMediaItem{MediaType: "image", FileUrl: legacyURL})
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 {
		t.Fatalf("source count=%d, want worker, fallback and original", len(sources))
	}
	if sources[0].URL != mediaFileCacheWorkerURL(ctx, "hotgo/file/2026-09-14/example.jpg") {
		t.Fatalf("unexpected worker source: %q", sources[0].URL)
	}
	if sources[1].URL != mediaFileCacheFallbackURL(ctx, "hotgo/file/2026-09-14/example.jpg") {
		t.Fatalf("unexpected fallback source: %q", sources[1].URL)
	}
	if sources[2].URL != legacyURL {
		t.Fatalf("unexpected original source: %q", sources[2].URL)
	}
	if sources[0].Role != "worker" || sources[1].Role != "fallback" || sources[2].Role != "original" {
		t.Fatalf("unexpected source roles: %q, %q, %q", sources[0].Role, sources[1].Role, sources[2].Role)
	}
}

func TestManagedMediaObjectPathRejectsUnmanagedPath(t *testing.T) {
	if got := managedMediaObjectPath("https://example.com/private/file.jpg"); got != "" {
		t.Fatalf("managedMediaObjectPath()=%q, want empty", got)
	}
}

func TestMediaFileCacheErrorClass(t *testing.T) {
	if got := mediaFileCacheErrorClass(&mediaFileCacheHTTPStatusError{statusCode: 404}); got != "404" {
		t.Fatalf("404 class=%q", got)
	}
	if got := mediaFileCacheErrorClass(&mediaFileCacheHTTPStatusError{statusCode: 503}); got != "5xx" {
		t.Fatalf("503 class=%q", got)
	}
}
