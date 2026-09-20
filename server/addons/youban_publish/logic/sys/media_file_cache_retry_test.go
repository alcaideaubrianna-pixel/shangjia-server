package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadMediaFileCacheWithRetryRecoversTransientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) < 3 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("media"))
	}))
	defer server.Close()

	err := downloadMediaFileCacheWithRetry(
		context.Background(), server.Client(), server.URL, filepath.Join(t.TempDir(), "media.jpg"), 3, func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("downloadMediaFileCacheWithRetry() error = %v", err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("request count = %d, want 3", got)
	}
}

func TestDownloadMediaFileCacheWithRetrySkipsPermanentHTTPError(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.NotFound(w, nil)
	}))
	defer server.Close()

	err := downloadMediaFileCacheWithRetry(
		context.Background(), server.Client(), server.URL, filepath.Join(t.TempDir(), "media.jpg"), 3, func(time.Duration) {},
	)
	if err == nil {
		t.Fatal("downloadMediaFileCacheWithRetry() error = nil, want HTTP 404 error")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestRewriteMediaFileCacheBaseURL(t *testing.T) {
	got := rewriteMediaFileCacheBaseURL(
		"https://cos.xiao-feiji.cc/hotgo/file/example.jpg?version=2",
		"https://cos.xiao-feiji.cc",
		"https://sto.xiao-feiji.cc",
	)
	want := "https://sto.xiao-feiji.cc/hotgo/file/example.jpg?version=2"
	if got != want {
		t.Fatalf("rewriteMediaFileCacheBaseURL() = %q, want %q", got, want)
	}
}

func TestRewriteMediaFileCacheBaseURLKeepsExternalSource(t *testing.T) {
	source := "https://telegram.org/file/example.jpg"
	got := rewriteMediaFileCacheBaseURL(source, "https://cos.xiao-feiji.cc", "https://sto.xiao-feiji.cc")
	if got != source {
		t.Fatalf("rewriteMediaFileCacheBaseURL() = %q, want unchanged source", got)
	}
}
