package telegrammedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadTelegramAssetRetriesTemporaryFailure(t *testing.T) {
	for _, status := range []int{503, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(status)
					return
				}
				_, _ = w.Write([]byte("image"))
			}))
			defer server.Close()
			data, err := DownloadURL(context.Background(), server.Client(), server.URL, DefaultMaxBytes)
			if status == 503 {
				if err != nil || string(data) != "image" || calls.Load() != 2 {
					t.Fatalf("retry failed: calls=%d err=%v", calls.Load(), err)
				}
			} else if err == nil || calls.Load() != 1 {
				t.Fatal("permanent error retried")
			}
		})
	}
}

func TestDownloadTelegramAssetRedactsAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DownloadURL(ctx, http.DefaultClient, "https://example.invalid/123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ_fake_token/photo.jpg", DefaultMaxBytes)
	if err == nil || strings.Contains(err.Error(), "fake_token") || strings.Contains(err.Error(), "https://") {
		t.Fatal("unsafe download error")
	}
}

func TestDownloadTelegramAssetRetriesReset(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		_, _ = w.Write([]byte("image"))
	}))
	defer server.Close()
	data, err := DownloadURL(context.Background(), server.Client(), server.URL, DefaultMaxBytes)
	if err != nil || string(data) != "image" || calls.Load() != 2 {
		t.Fatalf("reset recovery failed: calls=%d err=%v", calls.Load(), err)
	}
}

func TestDownloadFileRemovesPartialBeforeRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("partial"))
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	defer server.Close()
	dir := t.TempDir()
	destination := filepath.Join(dir, "result.jpg")
	if err := DownloadFile(context.Background(), server.Client(), server.URL, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "complete" || calls.Load() != 2 {
		t.Fatal("partial content retained")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("partial file leaked")
	}
}

func TestDownloadLimitsAndEmptyBody(t *testing.T) {
	for _, body := range []string{"", "oversized"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := DownloadURL(context.Background(), server.Client(), server.URL, 3)
		server.Close()
		if err == nil {
			t.Fatal("accepted empty or oversized media")
		}
	}
}
