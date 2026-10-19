package sys

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/utility/sensitive"
)

func downloadTelegramAsset(ctx context.Context, sourceURL string) ([]byte, error) {
	client, err := telegramHTTPClient(telegramProxyUrl(ctx))
	if err != nil {
		return nil, gerror.New(sensitive.Diagnostic(err.Error()))
	}
	client.Timeout = 2 * time.Minute
	defer client.CloseIdleConnections()
	return downloadTelegramAssetWithClient(ctx, client, sourceURL)
}

func downloadTelegramAssetWithClient(ctx context.Context, client *http.Client, sourceURL string) ([]byte, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return nil, gerror.New("Telegram媒体下载地址为空")
	}
	// All retries share the previous two-minute total download budget.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		data, retry, err := readTelegramAsset(ctx, client, sourceURL)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retry || attempt == 3 || ctx.Err() != nil {
			break
		}
		g.Log().Warning(ctx, "Telegram媒体下载临时失败，准备重试", g.Map{"attempt": attempt, "err": sensitive.Diagnostic(err.Error())})
		timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, gerror.New("Telegram媒体下载已超时或取消，请稍后重试")
		case <-timer.C:
		}
	}
	// Do not allow the credential-bearing URL to escape into callers or logs.
	return nil, gerror.Newf("下载Telegram媒体失败：%s", sensitive.Diagnostic(lastErr.Error()))
}

func readTelegramAsset(ctx context.Context, client *http.Client, sourceURL string) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, isRetryableTelegramBotGetFileError(err) || errors.Is(err, io.EOF), err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
			gerror.Newf("下载Telegram媒体失败，状态码:%d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, isRetryableTelegramBotGetFileError(err) || errors.Is(err, io.EOF), err
	}
	if len(data) == 0 {
		return nil, false, gerror.New("Telegram媒体内容为空")
	}
	return data, false, nil
}
