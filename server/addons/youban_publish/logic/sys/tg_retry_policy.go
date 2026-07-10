package sys

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
)

const (
	telegramRetryMinDelay = 30 * time.Second
	telegramRetryMaxDelay = 30 * time.Minute
)

type telegramJobRetryPolicy struct {
	Permanent  bool
	RetryDelay time.Duration
	Message    string
}

func telegramJobErrorRetryPolicy(err error, retryCount int) telegramJobRetryPolicy {
	if err == nil {
		return telegramJobRetryPolicy{Permanent: true, Message: "Telegram 发送失败：未知错误"}
	}
	if isTelegramPermanentSendError(err) {
		return telegramJobRetryPolicy{
			Permanent: true,
			Message:   "Telegram 发送遇到不可恢复错误，已停止该任务并释放频道队列：" + err.Error(),
		}
	}
	delay := telegramRecoverableRetryDelay(err, retryCount)
	return telegramJobRetryPolicy{
		RetryDelay: delay,
		Message:    telegramJobFriendlyErrorMessage(err, delay, retryCount),
	}
}

func telegramRecoverableRetryDelay(err error, retryCount int) time.Duration {
	var tooMany *tgbot.TooManyRequestsError
	if errors.As(err, &tooMany) && tooMany.RetryAfter > 0 {
		delay := time.Duration(tooMany.RetryAfter) * time.Second
		if delay > telegramRetryMaxDelay {
			return telegramRetryMaxDelay
		}
		if delay < telegramRetryMinDelay {
			return telegramRetryMinDelay
		}
		return delay
	}
	if retryCount <= 0 {
		retryCount = 1
	}
	exponent := math.Min(float64(retryCount-1), 6)
	delay := time.Duration(math.Pow(2, exponent)) * telegramRetryMinDelay
	if delay > telegramRetryMaxDelay {
		delay = telegramRetryMaxDelay
	}
	jitter := time.Duration(retryCount%5) * 3 * time.Second
	if delay+jitter > telegramRetryMaxDelay {
		return telegramRetryMaxDelay
	}
	return delay + jitter
}

func isTelegramPermanentSendError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	permanentParts := []string{
		"bad request: message to copy not found",
		"bad request: message to forward not found",
		"bad request: there are no messages to forward",
		"bad request: message identifiers must be in a strictly increasing order",
		"bad request: chat not found",
		"bad request: bot was blocked by the user",
		"bad request: not enough rights",
		"bad request: have no rights",
		"账号推送暂不支持远程媒体地址",
		"账号推送媒体文件不存在",
		"forbidden:",
		"unauthorized",
	}
	for _, part := range permanentParts {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func telegramJobFriendlyErrorMessage(err error, retryDelay time.Duration, retryCount int) string {
	var tooMany *tgbot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		seconds := int(retryDelay.Seconds())
		if tooMany.RetryAfter > 0 {
			seconds = tooMany.RetryAfter
		}
		return fmt.Sprintf("Telegram 发送频率过快，已触发限流；系统会等待 %d 秒后自动重试（第 %d 次）。建议为该频道配置多个可用推送 BOT，或降低全量推送/循环推送频率。", seconds, retryCount)
	}
	if isTelegramNetworkRetryError(err) {
		return fmt.Sprintf("Telegram 网络请求临时失败，系统会等待 %s 后自动重试（第 %d 次）：%s", retryDelay, retryCount, err.Error())
	}
	return fmt.Sprintf("Telegram 发送失败，系统会等待 %s 后自动重试（第 %d 次）：%s", retryDelay, retryCount, err.Error())
}

func isTelegramNetworkRetryError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	retryParts := []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"timeout awaiting response headers",
		"timeout",
		"temporary",
		"connection reset",
		"connection refused",
		"connection closed",
		"unexpected eof",
		"eof",
		"tls handshake timeout",
		"no such host",
		"502 bad gateway",
		"503 service unavailable",
		"504 gateway timeout",
		"internal server error",
		"too many requests",
	}
	for _, part := range retryParts {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}
