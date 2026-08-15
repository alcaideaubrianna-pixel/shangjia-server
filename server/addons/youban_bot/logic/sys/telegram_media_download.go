package sys

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
)

func downloadTelegramAsset(ctx context.Context, sourceURL string) ([]byte, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return nil, gerror.New("Telegram媒体下载地址为空")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	client, err := telegramHTTPClient(telegramProxyUrl(ctx))
	if err != nil {
		return nil, err
	}
	client.Timeout = 2 * time.Minute
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, gerror.Newf("下载Telegram媒体失败，状态码:%d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, gerror.New("Telegram媒体内容为空")
	}
	return data, nil
}
