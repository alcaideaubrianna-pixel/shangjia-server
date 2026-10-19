package sys

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
)

func TestBotErrorMessageHidesWrappedCredentials(t *testing.T) {
	secret := "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ_fake_token"
	inner := errors.New(`Get "https://example.invalid/files/` + secret + `/photo.jpg": read tcp: connection reset by peer`)
	if got := botErrorMessage(gerror.Wrap(inner, "展示资料保存失败，请重新发送")); got != "展示资料保存失败，请重新发送" {
		t.Fatalf("unexpected message: %s", got)
	}
	if got := botErrorMessage(inner); strings.Contains(got, secret) || strings.Contains(got, "https://") {
		t.Fatal("credential exposed")
	}
}

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
			data, err := downloadTelegramAssetWithClient(context.Background(), server.Client(), server.URL)
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
	_, err := downloadTelegramAssetWithClient(ctx, http.DefaultClient, "https://example.invalid/123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ_fake_token/photo.jpg")
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
	data, err := downloadTelegramAssetWithClient(context.Background(), server.Client(), server.URL)
	if err != nil || string(data) != "image" || calls.Load() != 2 {
		t.Fatalf("reset recovery failed: calls=%d err=%v", calls.Load(), err)
	}
}
