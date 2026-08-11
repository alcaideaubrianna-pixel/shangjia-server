package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
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

type IAccountTasks interface {
	Submit(ctx context.Context, in *sysin.AccountTaskSubmit) (int64, error)
	Get(ctx context.Context, taskID int64) (*sysin.AccountTask, error)
	Claim(ctx context.Context, lease *sysin.AccountLease, limit int, ttl time.Duration) ([]*sysin.AccountTask, error)
	Complete(ctx context.Context, taskID int64, lease *sysin.AccountLease, result json.RawMessage) error
	Fail(ctx context.Context, in *sysin.AccountTaskFailure) error
	RecoverExpired(ctx context.Context, limit int) (int, error)
}

type AccountTaskHandler interface {
	HandleAccountTask(ctx context.Context, client *telegram.Client, task *sysin.AccountTask) (json.RawMessage, error)
}

type AccountRuntimeSession interface {
	UpdateAccountRuntime(binding *sysin.AccountRuntimeBinding) bool
	BindAccountRuntimeHandlers(dispatcher tg.UpdateDispatcher)
	NewAccountRuntimeClient(ctx context.Context, dispatcher tg.UpdateDispatcher) (*telegram.Client, error)
	StartAccountRuntime(ctx context.Context, client *telegram.Client)
	StopAccountRuntime()
	HandleAccountRuntimeError(ctx context.Context, err error)
}

type AccountRuntimeProvider interface {
	ListAccountRuntimes(ctx context.Context) ([]*sysin.AccountRuntimeBinding, error)
	OpenAccountRuntime(ctx context.Context, binding *sysin.AccountRuntimeBinding) (AccountRuntimeSession, error)
}

type AccountOperation func(context.Context, *telegram.Client) error

type IAccountRuntime interface {
	Start(ctx context.Context)
	Stop()
	Refresh()
	Restart(accountID int64)
	Execute(ctx context.Context, accountID int64, timeout time.Duration, operation AccountOperation) (bool, error)
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
var localAccountTasks IAccountTasks
var accountTaskHandlers = map[string]AccountTaskHandler{}
var localAccountRuntime IAccountRuntime
var localAccountRuntimeProvider AccountRuntimeProvider

func Collector() ICollector {
	if localCollector == nil {
		panic("telegram collector service is not registered")
	}
	return localCollector
}

func RegisterCollector(value ICollector) { localCollector = value }

func CollectorDeliveryHandler() DeliveryHandler { return localDeliveryHandler }

func RegisterDeliveryHandler(value DeliveryHandler) { localDeliveryHandler = value }

func AccountTasks() IAccountTasks {
	if localAccountTasks == nil {
		panic("telegram collector account task service is not registered")
	}
	return localAccountTasks
}

func RegisterAccountTasks(value IAccountTasks) { localAccountTasks = value }

func RegisterAccountTaskHandler(taskType string, handler AccountTaskHandler) {
	if taskType == "" || handler == nil {
		return
	}
	accountTaskHandlers[taskType] = handler
}

func AccountTaskHandlerFor(taskType string) AccountTaskHandler { return accountTaskHandlers[taskType] }

func AccountRuntime() IAccountRuntime                        { return localAccountRuntime }
func RegisterAccountRuntime(value IAccountRuntime)           { localAccountRuntime = value }
func AccountRuntimeProviderInstance() AccountRuntimeProvider { return localAccountRuntimeProvider }
func RegisterAccountRuntimeProvider(value AccountRuntimeProvider) {
	localAccountRuntimeProvider = value
}
