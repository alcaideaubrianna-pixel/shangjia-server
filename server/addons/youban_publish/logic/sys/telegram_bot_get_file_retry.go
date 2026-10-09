package sys

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
)

const (
	telegramBotGetFileAttempts       = 3
	telegramBotGetFileAttemptTimeout = 4 * time.Second
)

var telegramBotGetFileDelays = []time.Duration{250 * time.Millisecond, 750 * time.Millisecond}

func telegramBotGetFileWithBoundedRetry(ctx context.Context, request func(context.Context) (*models.File, error)) (*models.File, int, error) {
	var file *models.File
	var err error
	for attempt := 1; attempt <= telegramBotGetFileAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, telegramBotGetFileAttemptTimeout)
		file, err = request(attemptCtx)
		cancel()
		if err == nil || !isRetryableTelegramBotRequestError(err) || attempt == telegramBotGetFileAttempts || ctx.Err() != nil {
			return file, attempt, err
		}
		timer := time.NewTimer(telegramBotGetFileDelays[attempt-1])
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, attempt, ctx.Err()
		case <-timer.C:
		}
	}
	return file, telegramBotGetFileAttempts, err
}

func isRetryableTelegramBotRequestError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"context deadline exceeded", "i/o timeout", "connection timed out", "connection refused",
		"connection reset", "connection closed", "no such host", "server misbehaving",
		"temporary failure", "unexpected eof", "broken pipe",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
