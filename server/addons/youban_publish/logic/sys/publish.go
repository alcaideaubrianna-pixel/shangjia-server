package sys

import (
	"context"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/payment"
)

type sSysPublish struct {
	runtimeCancel         context.CancelFunc
	runtimeDone           chan struct{}
	runtimeMu             publishRuntimeMutex
	telegramBotMu         publishRuntimeMutex
	telegramBots          map[string]*tgbot.Bot
	tgLoginMu             publishRuntimeMutex
	tgLogins              map[string]*telegramLoginRuntime
	tgQueueMu             publishRuntimeMutex
	tgQueueClient         *asynq.Client
	tgQueueServer         *asynq.Server
	tgBulkQueueServer     *asynq.Server
	mediaQueueServer      *asynq.Server
	mediaBulkQueueServer  *asynq.Server
	backgroundQueueServer *asynq.Server
	historyQueueServer    *asynq.Server

	telegramChannelMu    publishRuntimeMutex
	telegramChannelLocks map[string]*publishRuntimeMutex

	collectGroupMu        publishRuntimeMutex
	collectGroupTimers    map[int64]*time.Timer
	collectMediaMu        publishRuntimeMutex
	collectMediaLastTouch map[string]time.Time
	collectMediaSlots     chan struct{}
	collectMediaAccounts  map[string]chan struct{}

	accountCircuitMu publishRuntimeMutex
	accountCircuits  map[int64]accountCollectCircuit
}

func NewSysPublish() *sSysPublish {
	return &sSysPublish{
		tgLogins:              make(map[string]*telegramLoginRuntime),
		telegramChannelLocks:  make(map[string]*publishRuntimeMutex),
		collectGroupTimers:    make(map[int64]*time.Timer),
		collectMediaLastTouch: make(map[string]time.Time),
		collectMediaAccounts:  make(map[string]chan struct{}),
		accountCircuits:       make(map[int64]accountCollectCircuit),
	}
}

func init() {
	publish := NewSysPublish()
	service.RegisterSysPublish(publish)
	collectorservice.RegisterDeliveryHandler(&publishCollectorDeliveryHandler{publish: publish})
	collectorservice.RegisterAccountTaskHandler(collectorin.AccountTaskTypeHistoryPage, &publishCollectorAccountTaskHandler{publish: publish})
	collectorservice.RegisterAccountTaskHandler(collectorin.AccountTaskTypeMediaDownload, &publishCollectorAccountTaskHandler{publish: publish})
	collectorservice.RegisterAccountRuntimeProvider(&publishAccountRuntimeProvider{publish: publish})
	gatewayservice.RegisterProvider(&publishBotGatewayProvider{publish: publish})
	gatewayservice.RegisterConfigProvider(func(ctx context.Context) (*gatewayservice.RuntimeConfig, error) {
		conf, err := NewSysConfig().GetTelegram(ctx)
		if err != nil {
			return nil, err
		}
		return &gatewayservice.RuntimeConfig{
			Mode:           conf.BotRuntimeMode,
			ProxyURL:       conf.ProxyUrl,
			WebhookBaseURL: conf.WebhookBaseUrl,
			WebhookSecret:  conf.WebhookSecret,
		}, nil
	})
	payment.RegisterNotifyCall(tenantVipOrderGroup, publish.TenantVipPayNotify)
}

type publishBotGatewayProvider struct{ publish *sSysPublish }

func (p *publishBotGatewayProvider) Name() string { return "youban_publish" }

func (p *publishBotGatewayProvider) ListEnabledBots(ctx context.Context) ([]gatewayservice.BotBinding, error) {
	var rows []*struct {
		Id       int64  `json:"id"`
		TenantId int64  `json:"tenantId"`
		BotToken string `json:"botToken"`
	}
	if err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Fields("id", "tenant_id", "bot_token").
		Where("status", 1).
		WhereNull("deleted_at").
		Scan(&rows); err != nil {
		return nil, err
	}
	items := make([]gatewayservice.BotBinding, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.BotToken == "" {
			continue
		}
		items = append(items, gatewayservice.BotBinding{Owner: p.Name(), ReferenceID: row.Id, TenantID: row.TenantId, Token: row.BotToken})
	}
	return items, nil
}

func (p *publishBotGatewayProvider) HandleUpdate(ctx context.Context, binding gatewayservice.BotBinding, update *models.Update) error {
	p.publish.handleTelegramUpdate(ctx, binding.ReferenceID, binding.TenantID, update)
	return nil
}
