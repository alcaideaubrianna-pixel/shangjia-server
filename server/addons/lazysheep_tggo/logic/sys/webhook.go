// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model"
	isc "hotgo/internal/service"
)

const webhookSecretSize = 32

type webhookInfoResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		URL              string `json:"url"`
		LastErrorMessage string `json:"last_error_message"`
	} `json:"result"`
	Description string `json:"description"`
}

func (s *sLazySheepTGGo) normalizeBotConfig(key string, item *model.BotConfig) {
	if item == nil {
		return
	}
	item.Key = key
	if item.Role == "finance" {
		item.Role = "official"
	}
	if item.Role == "" {
		item.Role = "user"
	}
	if item.WebhookPath == "" {
		item.WebhookPath = defaultWebhookPath(key)
	}
	if item.WebhookSecret == "" {
		item.WebhookSecret = randomWebhookSecret()
	}
}

func (s *sLazySheepTGGo) syncWebhooksAfterSave(ctx context.Context, state *model.State) error {
	baseURL, err := s.webhookBaseURL(ctx)
	if err != nil || baseURL == "" {
		if err != nil {
			g.Log().Warningf(ctx, "跳过 Telegram webhook 自动注册：%+v", err)
		}
		for key, cfg := range state.Bots {
			if cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.Token) == "" {
				if err = s.SyncBot(ctx, key); err != nil {
					return err
				}
				continue
			}
			if err = s.startPollingBot(ctx, key); err != nil {
				return gerror.Wrapf(err, "启动机器人[%s] Polling失败", key)
			}
		}
		return nil
	}
	for key, cfg := range state.Bots {
		if cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.Token) == "" {
			continue
		}
		webhookURL := baseURL + cfg.WebhookPath
		if err = s.SetWebhook(ctx, key, webhookURL); err != nil {
			return gerror.Wrapf(err, "注册机器人[%s] Webhook失败", key)
		}
		if err = s.VerifyWebhook(ctx, key, webhookURL); err != nil {
			return gerror.Wrapf(err, "验证机器人[%s] Webhook失败", key)
		}
	}
	return s.SyncAllBots(ctx)
}

func (s *sLazySheepTGGo) bootBots(ctx context.Context) error {
	state, err := s.GetState(ctx)
	if err != nil {
		return err
	}
	baseURL, err := s.webhookBaseURL(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "跳过 Telegram webhook 自动注册：%+v", err)
	}
	for key, cfg := range state.Bots {
		if cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.Token) == "" {
			continue
		}
		if baseURL != "" {
			webhookURL := baseURL + cfg.WebhookPath
			if err = s.SetWebhook(ctx, key, webhookURL); err != nil {
				g.Log().Warningf(ctx, "注册 Telegram webhook 失败 bot:%s err:%+v", key, err)
			} else if err = s.VerifyWebhook(ctx, key, webhookURL); err != nil {
				g.Log().Warningf(ctx, "验证 Telegram webhook 失败 bot:%s err:%+v", key, err)
			}
			if err = s.SyncBot(ctx, key); err != nil {
				g.Log().Warningf(ctx, "初始化 Telegram bot runtime 失败 bot:%s err:%+v", key, err)
			}
			continue
		}
		if err = s.startPollingBot(ctx, key); err != nil {
			g.Log().Warningf(ctx, "启动 Telegram bot polling 失败 bot:%s err:%+v", key, err)
		}
	}
	return nil
}

func (s *sLazySheepTGGo) webhookBaseURL(ctx context.Context) (string, error) {
	basic, err := isc.SysConfig().GetBasic(ctx)
	if err != nil {
		return "", err
	}
	if basic == nil || strings.TrimSpace(basic.Domain) == "" {
		return "", nil
	}
	raw := strings.TrimRight(strings.TrimSpace(basic.Domain), "/")
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", gerror.Wrap(err, "基础域名格式不正确")
	}
	if parsed.Scheme != "https" {
		return "", nil
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "", nil
	}
	return raw, nil
}

func (s *sLazySheepTGGo) VerifyWebhook(ctx context.Context, botKey, expectedURL string) error {
	state, err := s.GetState(ctx)
	if err != nil {
		return err
	}
	cfg, ok := state.Bots[botKey]
	if !ok || cfg == nil {
		return fmt.Errorf("bot not found: %s", botKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.telegram.org/bot%s/getWebhookInfo", cfg.Token), nil)
	if err != nil {
		return err
	}
	httpClient, err := s.telegramHTTPClient(ctx)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("get webhook info failed: %s", string(out))
	}
	var data webhookInfoResponse
	if err = json.Unmarshal(out, &data); err != nil {
		return err
	}
	if !data.Ok {
		return fmt.Errorf("get webhook info failed: %s", data.Description)
	}
	if data.Result.URL != expectedURL {
		return fmt.Errorf("webhook url mismatch, current: %s, expected: %s", data.Result.URL, expectedURL)
	}
	if data.Result.LastErrorMessage != "" {
		g.Log().Warningf(ctx, "Telegram webhook 最近一次投递失败 bot:%s err:%s", botKey, data.Result.LastErrorMessage)
	}
	return nil
}

func defaultWebhookPath(botKey string) string {
	return "/api/lazysheep_tggo/webhook/" + url.PathEscape(botKey)
}

func randomWebhookSecret() string {
	buf := make([]byte, webhookSecretSize)
	if _, err := rand.Read(buf); err != nil {
		return shortHash(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	return hex.EncodeToString(buf)
}

func (s *sLazySheepTGGo) syncBotAfterSave(ctx context.Context, botKey string) error {
	state, err := s.GetState(ctx)
	if err != nil {
		return err
	}
	cfg, ok := state.Bots[botKey]
	if !ok || cfg == nil {
		return nil
	}
	if !cfg.Enabled || strings.TrimSpace(cfg.Token) == "" {
		return s.SyncBot(ctx, botKey)
	}
	baseURL, err := s.webhookBaseURL(ctx)
	if err != nil || baseURL == "" {
		if err != nil {
			g.Log().Warningf(ctx, "跳过 Telegram webhook 自动注册：%+v", err)
		}
		return s.startPollingBot(ctx, botKey)
	}
	webhookURL := baseURL + cfg.WebhookPath
	if err = s.SetWebhook(ctx, botKey, webhookURL); err != nil {
		return err
	}
	if err = s.VerifyWebhook(ctx, botKey, webhookURL); err != nil {
		return err
	}
	return s.SyncBot(ctx, botKey)
}

func shortHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:12]
}
