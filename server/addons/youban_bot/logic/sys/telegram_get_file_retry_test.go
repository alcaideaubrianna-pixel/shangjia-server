package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramBotGetFileRetriesDockerDNSFailure(t *testing.T) {
	attempts := 0
	file, gotAttempts, err := telegramBotGetFileWithRetry(context.Background(), context.Background(), "file-1", func(context.Context) (*models.File, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("dial tcp: lookup telegram-bot-api on 127.0.0.11:53: no such host")
		}
		return &models.File{FilePath: "photos/file.jpg"}, nil
	})
	if err != nil {
		t.Fatalf("telegramBotGetFileWithRetry() error = %v", err)
	}
	if gotAttempts != 3 || file == nil || file.FilePath != "photos/file.jpg" {
		t.Fatalf("attempts=%d file=%+v", gotAttempts, file)
	}
}

func TestTelegramBotGetFileDoesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	_, gotAttempts, err := telegramBotGetFileWithRetry(context.Background(), context.Background(), "file-2", func(context.Context) (*models.File, error) {
		attempts++
		return nil, errors.New("Bad Request: wrong file identifier")
	})
	if err == nil || gotAttempts != 1 || attempts != 1 {
		t.Fatalf("attempts=%d callbackAttempts=%d err=%v", gotAttempts, attempts, err)
	}
}

func TestRetryableTelegramBotGetFileError(t *testing.T) {
	if !isRetryableTelegramBotGetFileError(errors.New("lookup service on 127.0.0.11:53: no such host")) {
		t.Fatal("Docker DNS failure must be retryable")
	}
	if isRetryableTelegramBotGetFileError(errors.New("Bad Request: wrong file identifier")) {
		t.Fatal("Telegram API validation error must not be retryable")
	}
}

func TestScanTelegramErrorType(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, "none"},
		{context.DeadlineExceeded, "timeout"},
		{errors.New("dial tcp: lookup service: no such host"), "dns"},
		{errors.New("Bad Request: wrong file identifier"), "bad_request"},
		{errors.New("unknown failure"), "other"},
	}
	for _, item := range tests {
		if got := scanTelegramErrorType(item.err); got != item.want {
			t.Fatalf("scanTelegramErrorType(%v)=%q want=%q", item.err, got, item.want)
		}
	}
}
