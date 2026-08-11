package service

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

type ICollector interface {
	Enabled(ctx context.Context) bool
	StartRuntime(ctx context.Context)
	StartDeliveryRuntime(ctx context.Context)
	StopRuntime()
	IngestBotUpdate(ctx context.Context, bot BotContext, update *models.Update) error
	IngestAccountMessage(ctx context.Context, event *sysin.AccountMessageEvent) error
	EventExists(ctx context.Context, tenantID int64, eventKey string) (bool, error)
	MediaCache(ctx context.Context, fingerprint string) (*sysin.MediaCacheEntry, bool, error)
	ClaimMediaProcessing(ctx context.Context, fingerprint string, ttl time.Duration) (bool, error)
	SaveMediaReady(ctx context.Context, entry *sysin.MediaCacheEntry, ttl time.Duration) error
	ReleaseMediaProcessing(ctx context.Context, fingerprint string, cause error) error
}

type DeliveryHandler interface {
	HandleCollectorDelivery(ctx context.Context, delivery *sysin.CollectorDelivery) error
}

type BotContext struct {
	Key     string
	Token   string
	Binding BotBinding
}

type BotBinding struct {
	TenantID  int64
	SourceID  int64
	Reference string
}

var localCollector ICollector
var localDeliveryHandler DeliveryHandler

func Collector() ICollector {
	if localCollector == nil {
		panic("telegram collector service is not registered")
	}
	return localCollector
}

func RegisterCollector(value ICollector) { localCollector = value }

func CollectorDeliveryHandler() DeliveryHandler { return localDeliveryHandler }

func RegisterDeliveryHandler(value DeliveryHandler) { localDeliveryHandler = value }
