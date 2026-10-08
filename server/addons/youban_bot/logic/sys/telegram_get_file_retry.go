package sys

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

const telegramBotGetFileMaxAttempts = 4

var telegramBotGetFileRetryDelays = []time.Duration{
	250 * time.Millisecond,
	750 * time.Millisecond,
	1500 * time.Millisecond,
}

func telegramBotGetFileWithRetry(
	logCtx context.Context,
	requestCtx context.Context,
	fileID string,
	request func(context.Context) (*models.File, error),
) (*models.File, int, error) {
	var file *models.File
	var err error
	for attempt := 1; attempt <= telegramBotGetFileMaxAttempts; attempt++ {
		file, err = request(requestCtx)
		if err == nil || !isRetryableTelegramBotGetFileError(err) || attempt == telegramBotGetFileMaxAttempts || requestCtx.Err() != nil {
			return file, attempt, err
		}
		delay := telegramBotGetFileRetryDelays[attempt-1]
		g.Log().Warning(logCtx, "Bot媒体解析临时网络故障，准备重试", g.Map{
			"fileId": fileID, "attempt": attempt, "nextAttempt": attempt + 1,
			"delayMs": delay.Milliseconds(), "err": err,
		})
		timer := time.NewTimer(delay)
		select {
		case <-requestCtx.Done():
			timer.Stop()
			return nil, attempt, requestCtx.Err()
		case <-timer.C:
		}
	}
	return file, telegramBotGetFileMaxAttempts, err
}

func isRetryableTelegramBotGetFileError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"no such host",
		"server misbehaving",
		"temporary failure",
		"connection reset",
		"connection refused",
		"connection closed",
		"broken pipe",
		"unexpected eof",
		"i/o timeout",
		"tls handshake timeout",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
