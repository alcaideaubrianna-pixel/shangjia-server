package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/net/proxy"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

var gatewayObserveMeter = otel.Meter("hotgo/addons/youban_tg_bot_gateway")

type botRuntime struct {
	cancel        context.CancelFunc
	signature     string
	menuSignature string
}
type sGateway struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	refresh  chan struct{}
	runtimes map[string]*botRuntime
	bindings map[string][]service.BotBinding
	clients  map[string]*tgbot.Bot
	queueMu  sync.Mutex
	queue    *asynq.Server
	queueCli *asynq.Client
}

func init() { service.RegisterGateway(NewGateway()) }
func NewGateway() *sGateway {
	return &sGateway{
		refresh:  make(chan struct{}, 1),
		runtimes: map[string]*botRuntime{},
		bindings: map[string][]service.BotBinding{},
		clients:  map[string]*tgbot.Bot{},
	}
}

func (s *sGateway) StartRuntime(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	s.startUpdateQueue(ctx)
	go func() { defer close(s.done); s.run(runCtx) }()
}

func (s *sGateway) StopRuntime() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	for _, runtime := range s.runtimes {
		if runtime.cancel != nil {
			runtime.cancel()
		}
	}
	s.runtimes = map[string]*botRuntime{}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	s.stopUpdateQueue()
}

func (s *sGateway) run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	syncGateway := func() {
		if err := s.sync(ctx); err != nil {
			g.Log().Warningf(ctx, "同步TG Bot Gateway失败：%+v", err)
		}
	}
	syncGateway()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncGateway()
		case <-s.refresh:
			for {
				select {
				case <-s.refresh:
					continue
				default:
					syncGateway()
				}
				break
			}
		}
	}
}

func (s *sGateway) Refresh(_ context.Context) error {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
	return nil
}

func (s *sGateway) sync(ctx context.Context) error {
	conf, err := service.RuntimeConfiguration(ctx)
	if err != nil {
		return err
	}
	bindings, err := s.loadBindings(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.bindings = bindings
	s.mu.Unlock()
	mode := runtimeMode(ctx, conf)
	for key, items := range bindings {
		if len(items) == 0 {
			continue
		}
		if err = s.ensure(ctx, key, items[0].Token, mode, conf); err != nil {
			return err
		}
		if err = s.syncMenu(ctx, key, items[0].Token, items); err != nil {
			return err
		}
	}
	s.mu.Lock()
	for key, runtime := range s.runtimes {
		if _, ok := bindings[key]; !ok {
			if runtime.cancel != nil {
				runtime.cancel()
			}
			delete(s.runtimes, key)
			delete(s.clients, key)
		}
	}
	s.mu.Unlock()
	s.observeBotCounts(ctx, len(bindings))
	return nil
}

func (s *sGateway) loadBindings(ctx context.Context) (map[string][]service.BotBinding, error) {
	bindings := map[string][]service.BotBinding{}
	for _, provider := range service.Providers() {
		items, listErr := provider.ListEnabledBots(ctx)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range items {
			item.Token = strings.TrimSpace(item.Token)
			if item.Token == "" {
				continue
			}
			key := tokenKey(item.Token)
			bindings[key] = append(bindings[key], item)
		}
	}
	return bindings, nil
}

func (s *sGateway) observeBotCounts(ctx context.Context, configured int) {
	s.mu.Lock()
	running := len(s.runtimes)
	s.mu.Unlock()
	gauge, _ := gatewayObserveMeter.Int64Gauge("xiaohuiji.tg.gateway_bots")
	gauge.Record(ctx, int64(configured), metric.WithAttributes(attribute.String("state", "configured")))
	gauge.Record(ctx, int64(running), metric.WithAttributes(attribute.String("state", "running")))
}

func (s *sGateway) ensure(ctx context.Context, key, token, mode string, conf *service.RuntimeConfig) error {
	signature := mode + "\n" + strings.TrimSpace(conf.WebhookBaseURL) + "\n" + strings.TrimSpace(conf.ProxyURL)
	s.mu.Lock()
	current := s.runtimes[key]
	s.mu.Unlock()
	if current != nil && current.signature == signature {
		return nil
	}
	client, err := newBot(token, conf.ProxyURL, func(handlerCtx context.Context, bot *tgbot.Bot, update *models.Update) {
		if enqueueErr := s.enqueueUpdate(handlerCtx, key, update); enqueueErr != nil {
			g.Log().Warningf(handlerCtx, "TG Bot Gateway更新入队失败 key:%s err:%+v", key, enqueueErr)
		}
	})
	if err != nil {
		return err
	}
	if current != nil && current.cancel != nil {
		current.cancel()
	}
	runtime := &botRuntime{signature: signature}
	if mode == "webhook" {
		webhookURL := strings.TrimRight(conf.WebhookBaseURL, "/") + "/api/youban_tg_bot_gateway/telegram/webhook?key=" + key
		params := &tgbot.SetWebhookParams{URL: webhookURL, AllowedUpdates: allowedUpdates()}
		if conf.WebhookSecret != "" {
			params.SecretToken = conf.WebhookSecret
		}
		if _, err = client.SetWebhook(ctx, params); err != nil {
			return gerror.Wrap(err, "设置TG Bot Gateway Webhook失败")
		}
	} else {
		_, _ = client.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{DropPendingUpdates: false})
		runCtx, cancel := context.WithCancel(context.Background())
		runtime.cancel = cancel
		go client.Start(runCtx)
	}
	s.mu.Lock()
	s.runtimes[key], s.clients[key] = runtime, client
	s.mu.Unlock()
	return nil
}

func (s *sGateway) Webhook(ctx context.Context, key string, body []byte, secret string) error {
	webhooks, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_webhook_updates")
	webhooks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "received")))
	conf, err := service.RuntimeConfiguration(ctx)
	if err != nil {
		return err
	}
	if conf.WebhookSecret != "" && secret != conf.WebhookSecret {
		return gerror.New("Webhook Secret无效")
	}
	if len(body) == 0 || !json.Valid(body) {
		return gerror.New("Webhook消息格式不正确")
	}
	err = s.enqueueUpdateBody(ctx, strings.TrimSpace(key), body)
	result := "queued"
	if err != nil {
		result = "failed"
	}
	webhooks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	return err
}

func (s *sGateway) dispatch(ctx context.Context, key string, update *models.Update) error {
	s.mu.Lock()
	bindings := append([]service.BotBinding(nil), s.bindings[key]...)
	s.mu.Unlock()
	if len(bindings) == 0 {
		loaded, err := s.loadBindings(ctx)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.bindings = loaded
		bindings = append(bindings, s.bindings[key]...)
		s.mu.Unlock()
	}
	botCtx := service.BotContext{Key: key, Bindings: bindings}
	if len(bindings) > 0 {
		botCtx.Token = bindings[0].Token
	}
	var featureErrors []error
	for _, feature := range service.Features() {
		handled, featureErr := feature.HandleUpdate(ctx, botCtx, update)
		if featureErr != nil {
			featureErrors = append(featureErrors, gerror.Wrapf(featureErr, "TG Bot附加功能处理失败 feature:%s", feature.Key()))
			continue
		}
		if handled {
			counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_updates_dispatched")
			counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "handled")))
			return nil
		}
	}
	providers := map[string]service.Provider{}
	for _, provider := range service.Providers() {
		providers[provider.Name()] = provider
	}
	for _, binding := range bindings {
		provider := providers[binding.Owner]
		if provider == nil {
			continue
		}
		if err := provider.HandleUpdate(ctx, binding, update); err != nil {
			return err
		}
	}
	if len(featureErrors) > 0 {
		return errors.Join(featureErrors...)
	}
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_updates_dispatched")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "provider_dispatch")))
	return nil
}

func (s *sGateway) syncMenu(ctx context.Context, key, token string, bindings []service.BotBinding) error {
	items := make([]service.MenuItem, 0)
	managed := false
	botCtx := service.BotContext{Key: key, Token: token, Bindings: bindings}
	for _, feature := range service.Features() {
		menus, err := feature.Menus(ctx, botCtx)
		if err != nil {
			return gerror.Wrapf(err, "读取Bot功能菜单失败 feature:%s", feature.Key())
		}
		managed = managed || menus.Managed
		items = append(items, menus.Items...)
	}
	if !managed {
		return nil
	}
	commands, signature, err := normalizeMenuItems(items)
	if err != nil {
		return err
	}
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
	return []string{models.AllowedUpdateMessage, models.AllowedUpdateEditedMessage, models.AllowedUpdateChannelPost, models.AllowedUpdateEditedChannelPost, models.AllowedUpdateCallbackQuery}
}

func newBot(token, proxyURL string, handler tgbot.HandlerFunc) (*tgbot.Bot, error) {
	client, err := httpClient(proxyURL)
	if err != nil {
		return nil, err
	}
	return tgbot.New(token, tgbot.WithHTTPClient(35*time.Second, client), tgbot.WithSkipGetMe(), tgbot.WithAllowedUpdates(tgbot.AllowedUpdates(allowedUpdates())), tgbot.WithDefaultHandler(handler))
}
func httpClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{}
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
