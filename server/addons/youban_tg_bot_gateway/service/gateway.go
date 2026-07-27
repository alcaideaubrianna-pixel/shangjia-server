package service

import (
	"context"
	"sync"

	"github.com/go-telegram/bot/models"
)

type RuntimeConfig struct {
	Mode           string
	ProxyURL       string
	WebhookBaseURL string
	WebhookSecret  string
}

type BotBinding struct {
	Owner       string
	ReferenceID int64
	TenantID    int64
	Token       string
}

type Provider interface {
	Name() string
	ListEnabledBots(ctx context.Context) ([]BotBinding, error)
	HandleUpdate(ctx context.Context, binding BotBinding, update *models.Update) error
}

type ConfigProvider func(ctx context.Context) (*RuntimeConfig, error)

type IGateway interface {
	StartRuntime(ctx context.Context)
	StopRuntime()
	Refresh(ctx context.Context) error
	Webhook(ctx context.Context, key string, body []byte, secret string) error
}

var (
	localGateway   IGateway
	providersMu    sync.RWMutex
	providers      = map[string]Provider{}
	configProvider ConfigProvider
)

func Gateway() IGateway {
	if localGateway == nil {
		panic("implement not found for interface IGateway, forgot register?")
	}
	return localGateway
}

func RegisterGateway(i IGateway) { localGateway = i }

func RegisterProvider(provider Provider) {
	if provider == nil || provider.Name() == "" {
		return
	}
	providersMu.Lock()
	providers[provider.Name()] = provider
	providersMu.Unlock()
}

func Providers() []Provider {
	providersMu.RLock()
	defer providersMu.RUnlock()
	items := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		items = append(items, provider)
	}
	return items
}

func RegisterConfigProvider(provider ConfigProvider) { configProvider = provider }

func RuntimeConfiguration(ctx context.Context) (*RuntimeConfig, error) {
	if configProvider == nil {
		return &RuntimeConfig{Mode: "pull"}, nil
	}
	return configProvider(ctx)
}
