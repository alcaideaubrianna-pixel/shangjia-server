package service

import (
	"context"
	"sync"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type RuntimeConfig struct {
	Mode             string
	ProxyURL         string
	RuntimeServerURL string
	MediaServerURL   string
	WebhookBaseURL   string
	WebhookSecret    string
}

type BotBinding struct {
	Owner       string
	ReferenceID int64
	TenantID    int64
	Token       string
}

// OwnedBot describes a Bot managed by a provider without making addons import
// each other's DAO or service packages.
type OwnedBot struct {
	ReferenceID int64
	TenantID    int64
	AccountID   int64
	Name        string
	Username    string
	Token       string
	Status      int
}

type OwnedBotProvider interface {
	ListOwnedBots(ctx context.Context, tenantID, accountID, referenceID int64, lock bool) ([]OwnedBot, error)
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
	Client(ctx context.Context, token string) (*tgbot.Bot, error)
	MediaClient(ctx context.Context, token string) (*tgbot.Bot, error)
	Probe(ctx context.Context, token string) (*models.User, error)
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

func ProviderOwnedBots(ctx context.Context, owner string, tenantID, accountID, referenceID int64, lock bool) ([]OwnedBot, error) {
	providersMu.RLock()
	provider := providers[owner]
	providersMu.RUnlock()
	directory, ok := provider.(OwnedBotProvider)
	if !ok {
		return nil, nil
	}
	return directory.ListOwnedBots(ctx, tenantID, accountID, referenceID, lock)
}

func RegisterConfigProvider(provider ConfigProvider) { configProvider = provider }

func RuntimeConfiguration(ctx context.Context) (*RuntimeConfig, error) {
	if configProvider == nil {
		return &RuntimeConfig{Mode: "pull"}, nil
	}
	return configProvider(ctx)
}
