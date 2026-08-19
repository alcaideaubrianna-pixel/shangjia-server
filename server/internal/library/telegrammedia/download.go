package telegrammedia

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const defaultMaxBytes int64 = 50 * 1024 * 1024

// Download resolves and downloads a Telegram file with bounded retries.
// Storage persistence is deliberately left to the caller.
func Download(ctx context.Context, bot *tgbot.Bot, fileID string) ([]byte, *models.File, error) {
	startedAt := time.Now()
	if bot == nil || strings.TrimSpace(fileID) == "" {
		return nil, nil, gerror.New("Telegram媒体参数不完整")
	}
	downloadCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var file *models.File
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		file, err = bot.GetFile(downloadCtx, &tgbot.GetFileParams{FileID: fileID})
		if err == nil || downloadCtx.Err() != nil || attempt == 3 {
			break
		}
		time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
	}
	if err != nil {
		g.Log().Errorf(ctx, "TG链路 media_get_file_failed fileId:%s duration:%s err:%+v", fileID, time.Since(startedAt), err)
		return nil, nil, gerror.Wrap(err, "读取Telegram媒体文件失败")
	}
	if file == nil || strings.TrimSpace(file.FilePath) == "" {
		return nil, nil, gerror.New("Telegram媒体文件路径为空")
	}
	url := bot.FileDownloadLink(file)
	var response *http.Response
	for attempt := 1; attempt <= 3; attempt++ {
		req, requestErr := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
		if requestErr != nil {
			return nil, nil, requestErr
		}
		response, err = http.DefaultClient.Do(req)
		if err == nil || downloadCtx.Err() != nil || attempt == 3 {
			break
		}
		time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
	}
	if err != nil {
		g.Log().Errorf(ctx, "TG链路 media_download_failed fileId:%s duration:%s err:%+v", fileID, time.Since(startedAt), err)
		return nil, nil, gerror.Wrap(err, "下载Telegram媒体失败")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		g.Log().Errorf(ctx, "TG链路 media_download_http_failed fileId:%s status:%d duration:%s", fileID, response.StatusCode, time.Since(startedAt))
		return nil, nil, gerror.Newf("下载Telegram媒体失败，状态码:%d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, defaultMaxBytes+1))
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取Telegram媒体失败")
	}
	if len(data) == 0 {
		return nil, nil, gerror.New("Telegram媒体内容为空")
	}
	if int64(len(data)) > defaultMaxBytes {
		return nil, nil, gerror.New("Telegram媒体超过50MB限制")
	}
	g.Log().Infof(ctx, "TG链路 media_download_complete fileId:%s bytes:%d duration:%s", fileID, len(data), time.Since(startedAt))
	return data, file, nil
}
