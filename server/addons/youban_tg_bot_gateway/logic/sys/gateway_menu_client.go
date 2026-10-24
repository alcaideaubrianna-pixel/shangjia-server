package sys

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/net/proxy"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

func (s *sGateway) syncMenu(ctx context.Context, key, token string, bindings []service.BotBinding) error {
	commands, signature, managed, err := gatewayMenuCommands(ctx, service.BotContext{Key: key, Token: token, Bindings: bindings}, service.Features())
	if err != nil || !managed {
		return err
	}
	return s.applyMenu(ctx, key, commands, signature)
}

func gatewayMenuCommands(ctx context.Context, botCtx service.BotContext, features []service.Feature) ([]models.BotCommand, string, bool, error) {
	items := make([]service.MenuItem, 0)
	managed := false
	for _, feature := range features {
		if err := ctx.Err(); err != nil {
			return nil, "", false, err
		}
		menus, err := feature.Menus(ctx, botCtx)
		if err != nil {
			return nil, "", false, gerror.Wrapf(err, "读取Bot功能菜单失败 feature:%s", feature.Key())
		}
		managed = managed || menus.Managed
		if menus.Exclusive {
			items = nil
		}
		items = append(items, menus.Items...)
		if menus.Exclusive {
			break
		}
	}
	if !managed {
		return nil, "", false, nil
	}
	commands, signature, err := normalizeMenuItems(items)
	return commands, signature, managed, err
}

func (s *sGateway) applyMenu(ctx context.Context, key string, commands []models.BotCommand, signature string) error {
	var err error
	s.mu.Lock()
	runtime := s.runtimes[key]
	client := s.clients[key]
	if runtime != nil && runtime.menuSignature == signature {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if client == nil {
		return gerror.New("TG Bot Gateway客户端不存在")
	}
	if len(commands) == 0 {
		if _, err = client.DeleteMyCommands(ctx, &tgbot.DeleteMyCommandsParams{Scope: &models.BotCommandScopeDefault{}}); err != nil {
			return gerror.Wrap(err, "清理TG Bot菜单失败")
		}
	} else if _, err = client.SetMyCommands(ctx, &tgbot.SetMyCommandsParams{Commands: commands, Scope: &models.BotCommandScopeDefault{}}); err != nil {
		return gerror.Wrap(err, "设置TG Bot菜单失败")
	}
	if _, err = client.SetChatMenuButton(ctx, &tgbot.SetChatMenuButtonParams{MenuButton: &models.MenuButtonCommands{Type: models.MenuButtonTypeCommands}}); err != nil {
		return gerror.Wrap(err, "设置TG Bot底部菜单失败")
	}
	s.mu.Lock()
	if current := s.runtimes[key]; current != nil {
		current.menuSignature = signature
	}
	s.mu.Unlock()
	return nil
}

func normalizeMenuItems(items []service.MenuItem) ([]models.BotCommand, string, error) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Order == items[j].Order {
			return items[i].Command < items[j].Command
		}
		return items[i].Order < items[j].Order
	})
	seen := make(map[string]string, len(items))
	commands := make([]models.BotCommand, 0, len(items))
	signatureParts := make([]string, 0, len(items))
	for _, item := range items {
		command := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(item.Command), "/"))
		description := strings.TrimSpace(item.Description)
		if command == "" || description == "" {
			continue
		}
		if existing, ok := seen[command]; ok {
			if existing != description {
				return nil, "", gerror.Newf("TG Bot菜单命令冲突：/%s", command)
			}
			continue
		}
		seen[command] = description
		commands = append(commands, models.BotCommand{Command: command, Description: description})
		signatureParts = append(signatureParts, command+"="+description)
	}
	return commands, strings.Join(signatureParts, "\n"), nil
}

func runtimeMode(ctx context.Context, conf *service.RuntimeConfig) string {
	systemMode := strings.ToLower(strings.TrimSpace(g.Cfg().MustGet(ctx, "system.mode", "develop").String()))
	return runtimeModeForSystem(systemMode, conf)
}

func runtimeModeForSystem(systemMode string, conf *service.RuntimeConfig) string {
	if conf == nil {
		return "pull"
	}
	mode := strings.ToLower(strings.TrimSpace(conf.Mode))
	if mode == "polling" {
		mode = "pull"
	}
	if mode == "" || mode == "auto" {
		switch strings.ToLower(strings.TrimSpace(systemMode)) {
		case "", "develop", "testing", "not-set":
			return "pull"
		}
		if publicWebhook(conf.WebhookBaseURL) {
			return "webhook"
		}
		return "pull"
	}
	if mode == "webhook" && !publicWebhook(conf.WebhookBaseURL) {
		return "pull"
	}
	return mode
}
func publicWebhook(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "https://") && !strings.Contains(value, "localhost") && !strings.Contains(value, "127.0.0.1") && !strings.Contains(value, "0.0.0.0")
}
func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:12])
}
func allowedUpdates() []string {
	return []string{
		models.AllowedUpdateMessage,
		models.AllowedUpdateEditedMessage,
		models.AllowedUpdateChannelPost,
		models.AllowedUpdateEditedChannelPost,
		models.AllowedUpdateCallbackQuery,
		models.AllowedUpdateInlineQuery,
		models.AllowedUpdateMessageReaction,
		models.AllowedUpdateMyChatMember,
	}
}

func newBot(token, proxyURL, serverURL string, handler tgbot.HandlerFunc) (*tgbot.Bot, error) {
	return newBotWithTimeout(token, proxyURL, serverURL, handler, 35*time.Second)
}

func newBotWithTimeout(token, proxyURL, serverURL string, handler tgbot.HandlerFunc, timeout time.Duration) (*tgbot.Bot, error) {
	client, err := httpClient(proxyURL)
	if err != nil {
		return nil, err
	}
	client.Timeout = timeout
	options := []tgbot.Option{
		tgbot.WithHTTPClient(timeout, client),
		tgbot.WithSkipGetMe(),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates(allowedUpdates())),
		tgbot.WithDefaultHandler(handler),
	}
	if serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/"); serverURL != "" {
		parsed, parseErr := url.Parse(serverURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, gerror.New("TG Bot API Server URL配置无效")
		}
		options = append(options, tgbot.WithServerURL(serverURL))
	}
	return tgbot.New(token, options...)
}
func httpClient(proxyURL string) (*http.Client, error) {
	// Telegram occasionally sends HTTP/2 GOAWAY while a request body is in
	// flight. HTTP/1.1 avoids reusing that drained connection and lets the
	// existing queue retry policy handle transient network failures cleanly.
	transport := &http.Transport{
		ForceAttemptHTTP2: false,
		TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return &http.Client{Timeout: 35 * time.Second, Transport: transport}, nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	case "socks5", "socks5h":
		dialer, dialErr := proxy.FromURL(parsed, proxy.Direct)
		if dialErr != nil {
			return nil, dialErr
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}
	default:
		return nil, errors.New("仅支持 http/https/socks5 代理")
	}
	return &http.Client{Timeout: 35 * time.Second, Transport: transport}, nil
}
