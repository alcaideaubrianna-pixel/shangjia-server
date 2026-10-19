// Package telegrammedia owns Telegram media routing, resolution and transfer.
// Callers retain ownership of authorization, caching, fingerprints and storage.
package telegrammedia

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/utility/sensitive"
)

const OfficialMaxBytes int64 = 20 * 1024 * 1024
const DefaultMaxBytes int64 = 100 * 1024 * 1024

type Request struct {
	FileID    string
	MediaType string
	Size      int64
}

type Resolved struct {
	File     *models.File
	URL      string
	Endpoint string
	Attempts int
}

type ClientFactory func(context.Context) (*tgbot.Bot, error)

type Resolver struct {
	Official     ClientFactory
	Local        ClientFactory
	LocalFileURL string
	Token        string
	Acquire      func(context.Context, string) (func(), error)
}

// Resolve uses the official API except for videos known to exceed its limit.
// Unknown sizes are probed officially; only an explicit size rejection for a
// video can promote that request to the local server. Network errors never do.
func (r Resolver) Resolve(ctx context.Context, in Request) (Resolved, error) {
	out := Resolved{Endpoint: "official"}
	in.FileID = strings.TrimSpace(in.FileID)
	if in.FileID == "" {
		return out, gerror.New("Telegram媒体缺少文件标识")
	}
	video := strings.EqualFold(strings.TrimSpace(in.MediaType), "video")
	local := video && in.Size > OfficialMaxBytes
	if !local && in.Size > OfficialMaxBytes {
		return out, gerror.New("非视频文件超过Telegram官方下载大小限制")
	}
	for {
		factory := r.Official
		timeout := 3 * time.Second
		attempts := 3
		if local {
			out.Endpoint = "local"
			factory = r.Local
			timeout = 2 * time.Minute
			attempts = 1
		}
		if factory == nil {
			return out, gerror.New("Telegram媒体下载客户端未配置")
		}
		bot, err := factory(ctx)
		if err != nil {
			return out, r.safeError(err)
		}
		if bot == nil {
			return out, gerror.New("Telegram媒体下载客户端为空")
		}
		release := func() {}
		if r.Acquire != nil {
			release, err = r.Acquire(ctx, out.Endpoint)
			if err != nil {
				return out, r.safeError(err)
			}
		}
		file, count, err := func() (*models.File, int, error) {
			defer release()
			return GetFileWithRetry(ctx, attempts, timeout, func(callCtx context.Context) (*models.File, error) {
				return bot.GetFile(callCtx, &tgbot.GetFileParams{FileID: in.FileID})
			})
		}()
		out.Attempts += count
		if err != nil {
			if !local && video && strings.Contains(strings.ToLower(err.Error()), "file is too big") {
				local = true
				continue
			}
			return out, r.safeError(err)
		}
		if file == nil || strings.TrimSpace(file.FilePath) == "" {
			return out, gerror.New("Telegram媒体文件路径为空")
		}
		if !local && video && int64(file.FileSize) > OfficialMaxBytes {
			local = true
			continue
		}
		if !local && path.IsAbs(strings.TrimSpace(file.FilePath)) {
			return out, gerror.New("官方Telegram接口返回了异常的本地文件路径")
		}
		out.File = file
		out.URL, err = FileURL(bot, file, r.LocalFileURL)
		if err != nil {
			return out, r.safeError(err)
		}
		g.Log().Info(ctx, "Telegram媒体文件解析完成", g.Map{"stage": "get_file", "endpoint": out.Endpoint, "attempts": out.Attempts, "mediaType": in.MediaType, "size": in.Size})
		return out, nil
	}
}

func (r Resolver) safeError(err error) error {
	if errors.Is(err, context.Canceled) {
		return gerror.Wrap(context.Canceled, "Telegram媒体请求已取消")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return gerror.Wrap(context.DeadlineExceeded, "Telegram媒体请求超时")
	}
	message := err.Error()
	if r.Token != "" {
		message = strings.ReplaceAll(message, r.Token, "[token]")
	}
	return gerror.New(sensitive.Diagnostic(message))
}

// FileURL accepts absolute paths only inside the configured local Bot root.
func FileURL(bot *tgbot.Bot, file *models.File, baseURL string) (string, error) {
	if file == nil || strings.TrimSpace(file.FilePath) == "" {
		return "", gerror.New("Telegram媒体文件路径为空")
	}
	filePath := strings.TrimSpace(file.FilePath)
	if !path.IsAbs(filePath) {
		if bot == nil {
			return "", gerror.New("Telegram媒体下载客户端为空")
		}
		return bot.FileDownloadLink(file), nil
	}
	const root = "/var/lib/telegram-bot-api/"
	if !strings.HasPrefix(filePath, root) || path.Clean(filePath) != filePath {
		return "", gerror.New("TG Bot API返回了不受支持的本地文件路径")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", gerror.New("TG Bot API本地模式缺少有效文件服务地址")
	}
	relative := strings.TrimPrefix(filePath, root)
	parts := strings.Split(relative, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return baseURL + "/" + strings.Join(parts, "/"), nil
}
