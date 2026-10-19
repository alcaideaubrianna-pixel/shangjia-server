package telegrammedia

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func testBot(t *testing.T, endpoint string) *tgbot.Bot {
	t.Helper()
	bot, err := tgbot.New("123456:resolver-test-secret", tgbot.WithServerURL(endpoint), tgbot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	return bot
}

func TestResolverRoutesByVideoSize(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		size       int64
		local      bool
		rejected   bool
	}{
		{"photo", "image", 1000, false, false},
		{"video thumbnail", "image", 1000, false, false},
		{"small video", "video", 1000, false, false},
		{"at cloud limit", "video", OfficialMaxBytes, false, false},
		{"large video", "video", OfficialMaxBytes + 1, true, false},
		{"unknown video", "video", 0, false, false},
		{"missing photo size", "image", 0, false, false},
		{"oversized document", "file", OfficialMaxBytes + 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var official, local atomic.Int32
			cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				official.Add(1)
				fmt.Fprint(w, `{"ok":true,"result":{"file_id":"x","file_path":"photos/file.jpg"}}`)
			}))
			defer cloud.Close()
			large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				local.Add(1)
				fmt.Fprint(w, `{"ok":true,"result":{"file_id":"x","file_path":"/var/lib/telegram-bot-api/123456:resolver-test-secret/videos/file.mp4"}}`)
			}))
			defer large.Close()
			resolver := Resolver{Official: func(context.Context) (*tgbot.Bot, error) { return testBot(t, cloud.URL), nil }, Local: func(context.Context) (*tgbot.Bot, error) { return testBot(t, large.URL), nil }, LocalFileURL: large.URL, Token: "123456:resolver-test-secret"}
			got, err := resolver.Resolve(context.Background(), Request{FileID: "x", MediaType: tc.kind, Size: tc.size})
			if tc.rejected {
				if err == nil || official.Load()+local.Load() != 0 {
					t.Fatal("oversized nonvideo reached API")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.local {
				if got.Endpoint != "local" || official.Load() != 0 || local.Load() != 1 || !strings.HasPrefix(got.URL, large.URL+"/") {
					t.Fatalf("large video misrouted: %+v", got)
				}
			} else if got.Endpoint != "official" || official.Load() != 1 || local.Load() != 0 || !strings.HasPrefix(got.URL, cloud.URL+"/file/bot") {
				t.Fatalf("ordinary media misrouted: %+v", got)
			}
		})
	}
}

func TestResolverUnknownVideoSizeUsesLocalOnlyOnSizeRejection(t *testing.T) {
	for _, kind := range []string{"video", "image"} {
		t.Run(kind, func(t *testing.T) {
			var local atomic.Int32
			cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: file is too big"}`)
			}))
			defer cloud.Close()
			large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				local.Add(1)
				fmt.Fprint(w, `{"ok":true,"result":{"file_id":"x","file_path":"/var/lib/telegram-bot-api/123/video.mp4"}}`)
			}))
			defer large.Close()
			got, err := (Resolver{Official: func(context.Context) (*tgbot.Bot, error) { return testBot(t, cloud.URL), nil }, Local: func(context.Context) (*tgbot.Bot, error) { return testBot(t, large.URL), nil }, LocalFileURL: large.URL}).Resolve(context.Background(), Request{FileID: "x", MediaType: kind})
			if kind == "video" {
				if err != nil || local.Load() != 1 || got.Attempts != 2 {
					t.Fatalf("large video fallback failed: %v", err)
				}
			} else if err == nil || local.Load() != 0 {
				t.Fatal("image fell back to local")
			}
		})
	}
}

func TestResolverNetworkFailureNeverSwitchesToLocal(t *testing.T) {
	var local atomic.Int32
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(504)
		fmt.Fprint(w, `{"ok":false,"error_code":504,"description":"gateway timeout"}`)
	}))
	defer cloud.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := (Resolver{Official: func(context.Context) (*tgbot.Bot, error) { return testBot(t, cloud.URL), nil }, Local: func(context.Context) (*tgbot.Bot, error) { local.Add(1); return nil, errors.New("must not use local") }}).Resolve(ctx, Request{FileID: "x", MediaType: "video"})
	if err == nil || local.Load() != 0 {
		t.Fatal("network failure switched to local")
	}
}

func TestFileURLValidatesLocalPaths(t *testing.T) {
	for _, filePath := range []string{"/tmp/file.mp4", "/var/lib/telegram-bot-api/../secret", "/var/lib/telegram-bot-api/123/../../secret", "/var/lib/telegram-bot-api/123//file.mp4"} {
		if _, err := FileURL(nil, &models.File{FilePath: filePath}, "http://files.internal"); err == nil {
			t.Fatalf("accepted invalid path %q", filePath)
		}
	}
	got, err := FileURL(nil, &models.File{FilePath: "/var/lib/telegram-bot-api/123/video/file 1.mp4"}, "http://files.internal/")
	if err != nil || got != "http://files.internal/123/video/file%201.mp4" {
		t.Fatalf("bad local source %q err=%v", got, err)
	}
	if _, err = FileURL(nil, &models.File{FilePath: "/var/lib/telegram-bot-api/123/video.mp4"}, ""); err == nil {
		t.Fatal("accepted missing file service")
	}
}

func TestGetFileRetryAndParentDeadline(t *testing.T) {
	count := 0
	file, attempts, err := GetFileWithRetry(context.Background(), 3, time.Second, func(context.Context) (*models.File, error) {
		count++
		if count < 3 {
			return nil, errors.New("lookup service: no such host")
		}
		return &models.File{FilePath: "photos/file.jpg"}, nil
	})
	if err != nil || attempts != 3 || file == nil {
		t.Fatalf("temporary failure not retried: %v", err)
	}
	_, attempts, err = GetFileWithRetry(context.Background(), 3, time.Second, func(context.Context) (*models.File, error) {
		return nil, errors.New("Bad Request: wrong file identifier")
	})
	if err == nil || attempts != 1 {
		t.Fatal("permanent failure retried")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, attempts, err = GetFileWithRetry(ctx, 1, 2*time.Minute, func(ctx context.Context) (*models.File, error) { <-ctx.Done(); return nil, ctx.Err() })
	if attempts != 1 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("parent deadline ignored")
	}
	_, _, err = GetFileWithRetry(context.Background(), 1, 2*time.Minute, func(ctx context.Context) (*models.File, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 119*time.Second {
			return nil, errors.New("local budget truncated")
		}
		return &models.File{FilePath: "video.mp4"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
