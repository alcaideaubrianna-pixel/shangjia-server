package sys

import (
	"context"
	"errors"
	tgbot "github.com/go-telegram/bot"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"hotgo/addons/youban_publish/model"
)

func TestTelegramBotLargeMediaCircuitDoesNotFallback(t *testing.T) {
	const botID int64 = -17261017
	telegramBotGetFileControl.Lock()
	telegramBotGetFileControl.circuit[botID] = telegramBotGetFileCircuitState{openUntil: time.Now().Add(time.Minute)}
	telegramBotGetFileControl.Unlock()
	defer func() {
		telegramBotGetFileControl.Lock()
		delete(telegramBotGetFileControl.circuit, botID)
		telegramBotGetFileControl.Unlock()
	}()
	for _, size := range []int64{0, -1, 21 * 1024 * 1024} {
		_, _, endpoint, attempts, err := telegramBotGetFileWithFallback(context.Background(), botID, "invalid-token", nil, &model.TelegramConfig{BotApiServerUrl: "http://localhost:18081"}, "file", size)
		if endpoint != "local" || attempts != 0 || collectMediaRetryErrorFrom(err) == nil {
			t.Fatalf("size=%d endpoint=%s attempts=%d err=%v", size, endpoint, attempts, err)
		}
	}
}

func TestTelegramBotOfficialFallbackSizeBoundary(t *testing.T) {
	for _, tc := range []struct {
		size    int64
		allowed bool
	}{{-1, false}, {0, false}, {1, true}, {20 * 1024 * 1024, true}, {20*1024*1024 + 1, false}} {
		if telegramBotOfficialFallbackAllowed(tc.size) != tc.allowed {
			t.Fatalf("size=%d", tc.size)
		}
	}
}

func TestTelegramBotLocalRequestHonorsParentDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, attempts, err := telegramBotGetFileWithRetryPolicy(ctx, 1, 2*time.Minute, func(requestCtx context.Context) (*models.File, error) {
		<-requestCtx.Done()
		return nil, requestCtx.Err()
	})
	if attempts != 1 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestTelegramBotLocalRequestHasPreparationTime(t *testing.T) {
	_, _, err := telegramBotGetFileWithRetryPolicy(context.Background(), 1, 2*time.Minute, func(ctx context.Context) (*models.File, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 119*time.Second {
			return nil, errors.New("deadline too short")
		}
		return &models.File{FilePath: "/var/lib/telegram-bot-api/file.mp4"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTelegramBotUnknownSizeLocalFailureRemainsRetryable(t *testing.T) {
	const token = "123456:test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":504,"description":"context deadline exceeded 123456:test-secret"}`))
	}))
	defer server.Close()
	bot, err := tgbot.New(token, tgbot.WithServerURL(server.URL), tgbot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	_, _, endpoint, attempts, err := telegramBotGetFileWithFallback(context.Background(), -17261018, token, bot, &model.TelegramConfig{BotApiServerUrl: server.URL}, "file", 0)
	if endpoint != "local" || attempts != 1 || collectMediaRetryErrorFrom(err) == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("endpoint=%s attempts=%d err=%v", endpoint, attempts, err)
	}
}
