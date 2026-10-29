package sys

import (
	"context"
	"encoding/json"
	"strconv"
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

	"hotgo/addons/youban_tg_bot_gateway/service"
)

var gatewayObserveMeter = otel.Meter("hotgo/addons/youban_tg_bot_gateway")

const telegramMediaRequestTimeout = 10 * time.Minute
const telegramMediaSendRequestTimeout = 5 * time.Minute

type botRuntime struct {
	cancel        context.CancelFunc
	signature     string
	menuSignature string
}
type sGateway struct {
	mu                     sync.Mutex
	syncMu                 sync.Mutex
	cancel                 context.CancelFunc
	done                   chan struct{}
	refresh                chan struct{}
	runtimes               map[string]*botRuntime
	bindings               map[string][]service.BotBinding
	clients                map[string]*tgbot.Bot
	mediaClients           map[string]*tgbot.Bot
	webhookAuditAt         time.Time
	queueMu                sync.Mutex
	interactiveQueue       *asynq.Server
	bulkQueue              *asynq.Server
	queueCli               *asynq.Client
	loadBindingsForClient  func(context.Context) (map[string][]service.BotBinding, error)
	runtimeConfigForClient func(context.Context) (*service.RuntimeConfig, error)
	newClientForToken      func(string, string, string, tgbot.HandlerFunc) (*tgbot.Bot, error)
	newMediaClientForToken func(string, string, string, tgbot.HandlerFunc) (*tgbot.Bot, error)
}

func init() { service.RegisterGateway(NewGateway()) }
func NewGateway() *sGateway {
	return &sGateway{
		refresh:      make(chan struct{}, 1),
		runtimes:     map[string]*botRuntime{},
		bindings:     map[string][]service.BotBinding{},
		clients:      map[string]*tgbot.Bot{},
		mediaClients: map[string]*tgbot.Bot{},
	}
}

func (s *sGateway) MediaClient(ctx context.Context, token string) (*tgbot.Bot, error) {
	return s.mediaClient(ctx, token, false)
}

// SendClient uses the Local Bot endpoint only when complete Local Bot mode is enabled.
func (s *sGateway) SendClient(ctx context.Context, token string) (*tgbot.Bot, error) {
	return s.mediaClient(ctx, token, true)
}

func (s *sGateway) mediaClient(ctx context.Context, token string, sending bool) (*tgbot.Bot, error) {
	token = strings.TrimSpace(token)
	key := tokenKey(token)
	cacheKey := key
	if sending {
		cacheKey += ":send"
	}
	s.mu.Lock()
	client := s.mediaClients[cacheKey]
	s.mu.Unlock()
	if client != nil {
		return client, nil
	}
	loadBindings := func(ctx context.Context) (map[string][]service.BotBinding, error) {
		return loadClientBindings(ctx, key, service.Providers())
	}
	if s.loadBindingsForClient != nil {
		loadBindings = s.loadBindingsForClient
	}
	loaded, err := loadBindings(ctx)
	if err != nil {
		return nil, err
	}
	if len(loaded[key]) == 0 {
		return nil, gerror.New("TG Bot Gateway未找到已启用的Bot")
	}
	runtimeConfig := service.RuntimeConfiguration
	if s.runtimeConfigForClient != nil {
		runtimeConfig = s.runtimeConfigForClient
	}
	conf, err := runtimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	timeout := telegramMediaRequestTimeout
	if sending {
		timeout = telegramMediaSendRequestTimeout
	}
	clientFactory := func(token, proxyURL, serverURL string, handler tgbot.HandlerFunc) (*tgbot.Bot, error) {
		return newBotWithTimeout(token, proxyURL, serverURL, handler, timeout)
	}
	if s.newMediaClientForToken != nil {
		clientFactory = s.newMediaClientForToken
	}
	serverURL := conf.MediaServerURL
	operation := "download"
	if sending {
		operation = "send"
		if !conf.LocalBotEnabled {
			// An empty endpoint selects the SDK's official Telegram API URL.
			serverURL = ""
		}
	}
	client, err = clientFactory(token, conf.ProxyURL, serverURL, nil)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if cached := s.mediaClients[cacheKey]; cached != nil {
		client = cached
	} else {
		s.mediaClients[cacheKey] = client
	}
	s.mu.Unlock()
	// Never log the token or endpoint URL (which can contain credentials).
	g.Log().Info(ctx, "TG媒体客户端已创建", g.Map{"botKey": key, "operation": operation, "localBotEnabled": conf.LocalBotEnabled, "timeoutSeconds": int(timeout.Seconds())})
	return client, nil
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

func (s *sGateway) Client(ctx context.Context, token string) (*tgbot.Bot, error) {
	token = strings.TrimSpace(token)
	key := tokenKey(token)
	s.mu.Lock()
	client := s.clients[key]
	s.mu.Unlock()
	if client != nil {
		return client, nil
	}
	loadBindings := func(ctx context.Context) (map[string][]service.BotBinding, error) {
		return loadClientBindings(ctx, key, service.Providers())
	}
	if s.loadBindingsForClient != nil {
		loadBindings = s.loadBindingsForClient
	}
	loaded, err := loadBindings(ctx)
	if err != nil {
		return nil, err
	}
	if len(loaded[key]) == 0 {
		return nil, gerror.New("TG Bot Gateway未找到已启用的Bot")
	}
	runtimeConfig := service.RuntimeConfiguration
	if s.runtimeConfigForClient != nil {
		runtimeConfig = s.runtimeConfigForClient
	}
	conf, err := runtimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	clientFactory := newBot
	if s.newClientForToken != nil {
		clientFactory = s.newClientForToken
	}
	client, err = clientFactory(token, conf.ProxyURL, conf.RuntimeServerURL, nil)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if cached := s.clients[key]; cached != nil {
		client = cached
	} else {
		s.clients[key] = client
	}
	s.mu.Unlock()
	return client, nil
}

func (s *sGateway) Probe(ctx context.Context, token string) (*models.User, error) {
	conf, err := service.RuntimeConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	client, err := newBot(strings.TrimSpace(token), conf.ProxyURL, conf.RuntimeServerURL, nil)
	if err != nil {
		return nil, err
	}
	return client.GetMe(ctx)
}

func (s *sGateway) sync(ctx context.Context) error {
	// Client() may be called concurrently by media workers, auto-delete jobs,
	// and webhook handlers. Serialize the Telegram sync so one bot cannot issue
	// overlapping SetWebhook/DeleteWebhook requests and trigger Telegram 429s.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
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
	s.scheduleLocalWebhookAudit(ctx, conf, bindings)
	mode := runtimeMode(ctx, conf)
	var syncErr error
	for key, items := range bindings {
		if len(items) == 0 {
			continue
		}
		if err = s.ensure(ctx, key, items[0].Token, mode, conf); err != nil {
			if syncErr == nil {
				syncErr = gerror.Wrapf(err, "同步Bot运行时失败 key:%s", key)
			}
			g.Log().Warningf(ctx, "同步Bot运行时失败，继续同步其他Bot key:%s err:%+v", key, err)
			continue
		}
		if err = s.syncMenu(ctx, key, items[0].Token, items); err != nil {
			if syncErr == nil {
				syncErr = gerror.Wrapf(err, "同步Bot菜单失败 key:%s", key)
			}
			g.Log().Warningf(ctx, "同步Bot菜单失败，继续同步其他Bot key:%s err:%+v", key, err)
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
			delete(s.mediaClients, key)
			delete(s.mediaClients, key+":send")
		}
	}
	s.mu.Unlock()
	s.observeBotCounts(ctx, len(bindings))
	return syncErr
}

func (s *sGateway) scheduleLocalWebhookAudit(ctx context.Context, conf *service.RuntimeConfig, bindings map[string][]service.BotBinding) {
	serverURL := strings.TrimRight(strings.TrimSpace(conf.MediaServerURL), "/")
	if serverURL == "" || serverURL == strings.TrimRight(strings.TrimSpace(conf.RuntimeServerURL), "/") {
		return
	}
	s.mu.Lock()
	if !s.webhookAuditAt.IsZero() && time.Since(s.webhookAuditAt) < 5*time.Minute {
		s.mu.Unlock()
		return
	}
	s.webhookAuditAt = time.Now()
	s.mu.Unlock()
	tokens := make([]string, 0, len(bindings))
	for _, items := range bindings {
		if len(items) > 0 && strings.TrimSpace(items[0].Token) != "" {
			tokens = append(tokens, strings.TrimSpace(items[0].Token))
		}
	}
	go s.auditLocalWebhooks(context.WithoutCancel(ctx), conf.ProxyURL, serverURL, tokens)
}

func (s *sGateway) auditLocalWebhooks(ctx context.Context, proxyURL, serverURL string, tokens []string) {
	configured, failures := int64(0), int64(0)
	for _, token := range tokens {
		auditCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		client, err := newBotWithTimeout(token, proxyURL, serverURL, nil, 8*time.Second)
		if err == nil {
			var info *models.WebhookInfo
			info, err = client.GetWebhookInfo(auditCtx)
			if err == nil && info != nil && strings.TrimSpace(info.URL) != "" {
				configured++
			}
		}
		cancel()
		if err != nil {
			failures++
		}
	}
	gauge, _ := gatewayObserveMeter.Int64Gauge("xiaohuiji.tg.local_api_webhooks")
	gauge.Record(ctx, configured, metric.WithAttributes(attribute.String("state", "configured")))
	gauge.Record(ctx, failures, metric.WithAttributes(attribute.String("state", "audit_failed")))
	if configured > 0 || failures > 0 {
		g.Log().Warningf(ctx, "本地TG Bot API Webhook审计异常 configured:%d failed:%d bots:%d", configured, failures, len(tokens))
	}
}

// Client lookup must not depend on unrelated providers being healthy.
func loadClientBindings(ctx context.Context, key string, providers []service.Provider) (map[string][]service.BotBinding, error) {
	var providerErr error
	for _, provider := range providers {
		items, err := provider.ListEnabledBots(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			providerErr = gerror.Wrapf(err, "读取Bot提供方失败 provider:%s", provider.Name())
			continue
		}
		for _, item := range items {
			item.Token = strings.TrimSpace(item.Token)
			if item.Token != "" && tokenKey(item.Token) == key {
				return map[string][]service.BotBinding{key: {item}}, nil
			}
		}
	}
	if providerErr != nil {
		return nil, providerErr
	}
	return nil, gerror.New("TG Bot Gateway未找到已启用的Bot")
}

func (s *sGateway) loadBindings(ctx context.Context) (map[string][]service.BotBinding, error) {
	s.mu.Lock()
	previous := cloneBindings(s.bindings)
	s.mu.Unlock()
	return loadProviderBindings(ctx, service.Providers(), previous)
}

// A provider is an addon boundary. Keep its last known bindings when that
// addon is temporarily unavailable so it cannot take unrelated Bots offline.
func loadProviderBindings(ctx context.Context, providers []service.Provider, previous map[string][]service.BotBinding) (map[string][]service.BotBinding, error) {
	bindings := map[string][]service.BotBinding{}
	succeeded := 0
	var firstErr error
	for _, provider := range providers {
		items, listErr := provider.ListEnabledBots(ctx)
		if listErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if firstErr == nil {
				firstErr = gerror.Wrapf(listErr, "读取Bot提供方失败 provider:%s", provider.Name())
			}
			for key, oldItems := range previous {
				for _, item := range oldItems {
					if item.Owner == provider.Name() {
						bindings[key] = append(bindings[key], item)
					}
				}
			}
			g.Log().Warningf(ctx, "读取Bot提供方失败，保留已有绑定 provider:%s err:%+v", provider.Name(), listErr)
			continue
		}
		succeeded++
		for _, item := range items {
			item.Token = strings.TrimSpace(item.Token)
			if item.Token == "" {
				continue
			}
			key := tokenKey(item.Token)
			bindings[key] = append(bindings[key], item)
		}
	}
	if succeeded == 0 && firstErr != nil {
		return nil, firstErr
	}
	return bindings, nil
}

func cloneBindings(source map[string][]service.BotBinding) map[string][]service.BotBinding {
	cloned := make(map[string][]service.BotBinding, len(source))
	for key, items := range source {
		cloned[key] = append([]service.BotBinding(nil), items...)
	}
	return cloned
}

func (s *sGateway) observeBotCounts(ctx context.Context, configured int) {
	s.mu.Lock()
	running := len(s.runtimes)
	s.mu.Unlock()
	gauge, _ := gatewayObserveMeter.Int64Gauge("xiaohuiji.tg.gateway_bots")
	gauge.Record(ctx, int64(configured), metric.WithAttributes(attribute.String("state", "configured")))
	gauge.Record(ctx, int64(running), metric.WithAttributes(attribute.String("state", "running")))
}

func gatewayRuntimeSignature(mode string, conf *service.RuntimeConfig) string {
	return mode + "\n" + strings.TrimSpace(conf.WebhookBaseURL) + "\n" + strings.TrimSpace(conf.ProxyURL) + "\n" + strings.TrimSpace(conf.RuntimeServerURL) + "\n" + strings.TrimSpace(conf.MediaServerURL) + "\n" + strconv.FormatBool(conf.LocalBotEnabled) + "\n" + strings.Join(allowedUpdates(), ",")
}

func (s *sGateway) ensure(ctx context.Context, key, token, mode string, conf *service.RuntimeConfig) error {
	signature := gatewayRuntimeSignature(mode, conf)
	s.mu.Lock()
	current := s.runtimes[key]
	s.mu.Unlock()
	if current != nil && current.signature == signature {
		return nil
	}
	client, err := newBot(token, conf.ProxyURL, conf.RuntimeServerURL, func(handlerCtx context.Context, bot *tgbot.Bot, update *models.Update) {
		if submitErr := s.submitUpdate(handlerCtx, key, update); submitErr != nil {
			g.Log().Warningf(handlerCtx, "TG Bot Gateway更新提交失败 key:%s err:%+v", key, submitErr)
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
	delete(s.mediaClients, key)
	delete(s.mediaClients, key+":send")
	s.mu.Unlock()
	return nil
}

func (s *sGateway) Webhook(ctx context.Context, key string, body []byte, secret string) error {
	startedAt := time.Now()
	webhooks, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_webhook_updates")
	webhooks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "received")))
	configStartedAt := time.Now()
	conf, err := service.RuntimeConfiguration(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "TG链路 gateway_webhook_config_failed key:%s duration:%s total:%s err:%+v", key, time.Since(configStartedAt), time.Since(startedAt), err)
		return err
	}
	configDuration := time.Since(configStartedAt)
	if conf.WebhookSecret != "" && secret != conf.WebhookSecret {
		return gerror.New("Webhook Secret无效")
	}
	if len(body) == 0 || !json.Valid(body) {
		return gerror.New("Webhook消息格式不正确")
	}
	var update models.Update
	parseStartedAt := time.Now()
	if err = json.Unmarshal(body, &update); err != nil {
		return gerror.Wrap(err, "Webhook消息格式不正确")
	}
	parseDuration := time.Since(parseStartedAt)
	g.Log().Infof(ctx, "TG链路 gateway_webhook_update key:%s updateId:%d message:%t editedMessage:%t channelPost:%t editedChannelPost:%t callback:%t inline:%t reaction:%t", key, update.ID, update.Message != nil, update.EditedMessage != nil, update.ChannelPost != nil, update.EditedChannelPost != nil, update.CallbackQuery != nil, update.InlineQuery != nil, update.MessageReaction != nil)
	if update.InlineQuery != nil {
		g.Log().Infof(ctx, "TG链路 gateway_webhook_inline_received key:%s updateId:%d queryId:%s query:%q", key, update.ID, update.InlineQuery.ID, strings.TrimSpace(update.InlineQuery.Query))
	}
	submitStartedAt := time.Now()
	err = s.submitUpdate(ctx, strings.TrimSpace(key), &update)
	submitDuration := time.Since(submitStartedAt)
	result := "queued"
	if updateNeedsImmediateDispatch(&update) {
		result = "dispatched"
	}
	if err != nil {
		result = "failed"
		g.Log().Warningf(ctx, "TG链路 gateway_webhook_submit_failed key:%s updateId:%d config:%s parse:%s submit:%s total:%s err:%+v", key, update.ID, configDuration, parseDuration, submitDuration, time.Since(startedAt), err)
	} else if total := time.Since(startedAt); total >= time.Second {
		g.Log().Warningf(ctx, "TG链路 gateway_webhook_slow key:%s updateId:%d result:%s config:%s parse:%s submit:%s total:%s", key, update.ID, result, configDuration, parseDuration, submitDuration, total)
	}
	webhooks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	return err
}

func (s *sGateway) submitUpdate(ctx context.Context, key string, update *models.Update) error {
	if update != nil && update.CallbackQuery != nil {
		query := update.CallbackQuery
		g.Log().Infof(ctx, "TG链路 gateway_callback_received updateId:%d callbackId:%s userId:%d data:%s", update.ID, query.ID, query.From.ID, strings.TrimSpace(query.Data))
		s.ackCallbackFast(ctx, key, query)
	}
	if updateNeedsImmediateDispatch(update) {
		return s.dispatch(ctx, key, update)
	}
	return s.enqueueUpdate(ctx, key, update)
}

// ackCallbackFast clears Telegram's callback spinner before the durable
// interactive queue performs database and UI work. It is intentionally best
// effort and only uses an already cached client, so the webhook path never
// blocks on client construction or binding queries.
func (s *sGateway) ackCallbackFast(ctx context.Context, key string, query *models.CallbackQuery) {
	if query == nil || strings.TrimSpace(query.ID) == "" {
		return
	}
	s.mu.Lock()
	client := s.clients[key]
	s.mu.Unlock()
	if client == nil {
		g.Log().Infof(ctx, "TG链路 callback_fast_ack_skipped callbackId:%s reason:client_not_cached", query.ID)
		return
	}
	startedAt := time.Now()
	ackCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	if _, err := client.AnswerCallbackQuery(ackCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
		g.Log().Warningf(ctx, "TG链路 callback_fast_ack_failed callbackId:%s duration:%s err:%+v", query.ID, time.Since(startedAt), err)
		return
	}
	g.Log().Infof(ctx, "TG链路 callback_fast_ack callbackId:%s duration:%s", query.ID, time.Since(startedAt))
}

func updateNeedsImmediateDispatch(update *models.Update) bool {
	// Inline queries must be answered within Telegram's short response window.
	// Callback queries and private messages use the dedicated durable
	// interactive queue so channel traffic cannot starve them.
	return update != nil && update.InlineQuery != nil
}
