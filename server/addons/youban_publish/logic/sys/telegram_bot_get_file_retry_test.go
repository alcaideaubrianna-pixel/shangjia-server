package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramBotGetFileWithBoundedRetryRecovers(t *testing.T) {
	attempts := 0
	file, gotAttempts, err := telegramBotGetFileWithBoundedRetry(context.Background(), func(context.Context) (*models.File, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("dial tcp: connection timed out")
		}
		return &models.File{FilePath: "photos/file.jpg"}, nil
	})
	if err != nil || gotAttempts != 3 || file == nil {
		t.Fatalf("file=%+v attempts=%d err=%v", file, gotAttempts, err)
	}
}

func TestTelegramBotGetFileWithBoundedRetryStopsOnPermanentError(t *testing.T) {
	_, attempts, err := telegramBotGetFileWithBoundedRetry(context.Background(), func(context.Context) (*models.File, error) {
		return nil, errors.New("Bad Request: wrong file identifier")
	})
	if err == nil || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}
