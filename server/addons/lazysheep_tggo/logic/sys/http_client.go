// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"golang.org/x/net/proxy"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
)

const (
	telegramHTTPTimeout                      = 30 * time.Second
	telegramMediaUploadHTTPTimeout           = 2 * time.Minute
	telegramMediaUploadResponseHeaderTimeout = 90 * time.Second
)

var telegramBotTokenPattern = regexp.MustCompile(`([0-9]{5,}):[A-Za-z0-9_-]{20,}`)

func (s *sLazySheepTGGo) telegramHTTPClient(ctx context.Context) (*http.Client, error) {
	state, err := s.GetState(ctx)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if state.Global != nil {
		proxyURL = strings.TrimSpace(state.Global.TelegramProxy)
	}
	return buildTelegramHTTPClient(proxyURL)
}

func (s *sLazySheepTGGo) telegramMediaUploadHTTPClient(ctx context.Context) (*http.Client, error) {
	client, err := s.telegramHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	return configureTelegramMediaUploadHTTPClient(client), nil
}

func configureTelegramMediaUploadHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		return nil
	}
	client.Timeout = telegramMediaUploadHTTPTimeout
	if transport, ok := client.Transport.(*http.Transport); ok {
		cloned := transport.Clone()
		cloned.ResponseHeaderTimeout = telegramMediaUploadResponseHeaderTimeout
		client.Transport = cloned
	}
	return client
}

func sanitizeTelegramBotError(err error) error {
	if err == nil {
		return nil
	}
	return gerror.New(telegramBotTokenPattern.ReplaceAllString(err.Error(), "$1:***"))
}

func telegramBotIDFromToken(token string) string {
	token = strings.TrimSpace(token)
	if index := strings.IndexByte(token, ':'); index > 0 {
		return token[:index]
	}
	return "unknown"
}

func buildTelegramHTTPClient(proxyRaw string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxyRaw != "" {
		parsed, err := url.Parse(proxyRaw)
		if err != nil {
			return nil, gerror.Wrap(err, "Telegram 代理地址格式不正确")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			transport.Proxy = http.ProxyURL(parsed)
		case "socks5", "socks5h":
			dialer, err := proxy.FromURL(parsed, proxy.Direct)
			if err != nil {
				return nil, gerror.Wrap(err, "Telegram Socks5 代理初始化失败")
			}
			transport.Proxy = nil
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		default:
			return nil, gerror.New("Telegram 代理仅支持 http、https、socks5")
		}
	}
	return &http.Client{
		Timeout:   telegramHTTPTimeout,
		Transport: transport,
	}, nil
}

func (s *sLazySheepTGGo) TestTelegramProxy(ctx context.Context, in *lsysin.TelegramProxyTestInp) (*lsysin.TelegramProxyTestModel, error) {
	proxyURL := ""
	if in != nil {
		proxyURL = strings.TrimSpace(in.TelegramProxy)
	}
	client, err := buildTelegramHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, gerror.Wrap(err, "Telegram 代理不可用")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return nil, fmt.Errorf("Telegram 代理检测失败，HTTP状态码：%d", resp.StatusCode)
	}
	return &lsysin.TelegramProxyTestModel{Ok: true}, nil
}
