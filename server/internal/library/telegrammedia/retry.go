package telegrammedia

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/utility/sensitive"
)

func Retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var rate *tgbot.TooManyRequestsError
	if errors.As(err, &rate) {
		return true
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, part := range []string{"no such host", "server misbehaving", "temporary failure", "connection reset", "connection refused", "connection closed", "connection timed out", "broken pipe", "unexpected eof", "i/o timeout", "context deadline exceeded", "tls handshake timeout", "internal server error", "bad gateway", "gateway timeout", "service unavailable"} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func waitRetry(ctx context.Context, attempt int, err error) error {
	delay := time.Duration(attempt*attempt) * 250 * time.Millisecond
	var rate *tgbot.TooManyRequestsError
	if errors.As(err, &rate) && rate.RetryAfter > 0 {
		delay = time.Duration(rate.RetryAfter) * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// GetFileWithRetry is shared by all Telegram getFile callers. Parent deadlines
// always win; permanent API errors are never retried.
func GetFileWithRetry(ctx context.Context, attempts int, timeout time.Duration, request func(context.Context) (*models.File, error)) (*models.File, int, error) {
	if attempts < 1 || timeout <= 0 || request == nil {
		return nil, 0, gerror.New("Telegram文件请求重试参数无效")
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, attempt - 1, err
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		file, err := request(callCtx)
		cancel()
		if err == nil || !Retryable(err) || attempt == attempts || ctx.Err() != nil {
			return file, attempt, err
		}
		g.Log().Warning(ctx, "Telegram媒体临时故障，准备重试", g.Map{"stage": "get_file", "attempt": attempt, "err": sensitive.Diagnostic(err.Error())})
		if waitErr := waitRetry(ctx, attempt, err); waitErr != nil {
			return nil, attempt, waitErr
		}
	}
	return nil, attempts, gerror.New("Telegram文件请求失败")
}
