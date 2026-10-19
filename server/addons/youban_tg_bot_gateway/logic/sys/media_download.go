package sys

import (
	"context"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/telegrammedia"
)

// Shared by interactive and collector callers; local large-video preparation
// cannot consume all getFile capacity or repeatedly hammer a failing server.
var mediaDownloads = struct {
	sync.Mutex
	official chan struct{}
	local    chan struct{}
	bots     map[string]chan struct{}
	circuits map[string]mediaDownloadCircuit
}{official: make(chan struct{}, 8), local: make(chan struct{}, 4), bots: map[string]chan struct{}{}, circuits: map[string]mediaDownloadCircuit{}}

type mediaDownloadCircuit struct {
	failures int
	until    time.Time
}

func acquireMediaDownload(ctx context.Context, key string, endpoint string) (func(), error) {
	key += ":" + endpoint
	mediaDownloads.Lock()
	slots := mediaDownloads.bots[key]
	if slots == nil {
		slots = make(chan struct{}, 2)
		mediaDownloads.bots[key] = slots
	}
	mediaDownloads.Unlock()
	// Acquire per-bot capacity first so one busy bot cannot occupy global slots
	// while it is merely waiting for its own in-flight calls.
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	global := mediaDownloads.official
	if endpoint == "local" {
		global = mediaDownloads.local
	}
	select {
	case global <- struct{}{}:
		return func() { <-global; <-slots }, nil
	case <-ctx.Done():
		<-slots
		return nil, ctx.Err()
	}
}

func (s *sGateway) mediaDownloadConfig(ctx context.Context) (*service.RuntimeConfig, error) {
	provider := service.RuntimeConfiguration
	if s.runtimeConfigForClient != nil {
		provider = s.runtimeConfigForClient
	}
	return provider(ctx)
}

func (s *sGateway) ResolveMedia(ctx context.Context, token string, in telegrammedia.Request) (telegrammedia.Resolved, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return telegrammedia.Resolved{}, gerror.New("Telegram Bot Token未配置")
	}
	conf, err := s.mediaDownloadConfig(ctx)
	if err != nil {
		return telegrammedia.Resolved{}, err
	}
	key := tokenKey(token)
	resolver := telegrammedia.Resolver{
		Token: token, LocalFileURL: conf.MediaFileURL,
		Acquire: func(ctx context.Context, endpoint string) (func(), error) {
			return acquireMediaDownload(ctx, key, endpoint)
		},
		Official: func(ctx context.Context) (*tgbot.Bot, error) { return s.SendClient(ctx, token) },
		Local: func(ctx context.Context) (*tgbot.Bot, error) {
			if strings.TrimSpace(conf.MediaServerURL) == "" || strings.Contains(strings.ToLower(conf.MediaServerURL), "api.telegram.org") {
				return nil, gerror.New("大视频下载需要配置自建Telegram Bot API服务")
			}
			mediaDownloads.Lock()
			circuit := mediaDownloads.circuits[key]
			mediaDownloads.Unlock()
			if time.Now().Before(circuit.until) {
				return nil, gerror.New("自建Telegram媒体服务暂处于熔断等待，请稍后重试")
			}
			return s.MediaClient(ctx, token)
		},
	}
	result, err := resolver.Resolve(ctx, in)
	if result.Endpoint == "local" && result.Attempts > 0 {
		mediaDownloads.Lock()
		if err == nil {
			delete(mediaDownloads.circuits, key)
		} else if telegrammedia.Retryable(err) {
			state := mediaDownloads.circuits[key]
			state.failures++
			if state.failures >= 3 {
				state.until = time.Now().Add(5 * time.Minute)
			}
			mediaDownloads.circuits[key] = state
		}
		mediaDownloads.Unlock()
	}
	return result, err
}

func (s *sGateway) DownloadMedia(ctx context.Context, token string, in telegrammedia.Request, maxBytes int64) ([]byte, telegrammedia.Resolved, error) {
	result, err := s.ResolveMedia(ctx, token, in)
	if err != nil {
		return nil, result, err
	}
	conf, err := s.mediaDownloadConfig(ctx)
	if err != nil {
		return nil, result, err
	}
	client, err := httpClient(conf.ProxyURL)
	if err != nil {
		return nil, result, gerror.New("创建Telegram媒体下载网络客户端失败")
	}
	client.Timeout = 2 * time.Minute
	defer client.CloseIdleConnections()
	if maxBytes <= 0 {
		maxBytes = telegrammedia.DefaultMaxBytes
	}
	data, err := telegrammedia.DownloadURL(ctx, client, result.URL, maxBytes)
	return data, result, err
}
