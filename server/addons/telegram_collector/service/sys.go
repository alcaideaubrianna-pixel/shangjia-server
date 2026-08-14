package service

import (
	"context"
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
	SubmitAndWait(ctx context.Context, in *sysin.AccountTaskSubmit, pollInterval time.Duration) (*sysin.AccountTask, error)
	Get(ctx context.Context, taskID int64) (*sysin.AccountTask, error)
	WaitTerminal(ctx context.Context, taskID int64, pollInterval time.Duration) (*sysin.AccountTask, error)
	Claim(ctx context.Context, lease *sysin.AccountLease, limit int, ttl time.Duration) ([]*sysin.AccountTask, error)
	Complete(ctx context.Context, taskID int64, lease *sysin.AccountLease, result *sysin.AccountMediaDownloadResult) error
	Fail(ctx context.Context, in *sysin.AccountTaskFailure) error
	RecoverExpired(ctx context.Context, limit int) (int, error)
	ActiveStatusStats(ctx context.Context) ([]sysin.AccountTaskStatusStat, error)
}

type AccountTaskHandler interface {
	HandleAccountTask(ctx context.Context, client *telegram.Client, task *sysin.AccountTask) (*sysin.AccountMediaDownloadResult, error)
}

type AccountMediaProvider interface {
	ResolvePeer(ctx context.Context, tenantID, accountID int64, chatID string, client *telegram.Client) (tg.InputPeerClass, error)
	StoreMedia(ctx context.Context, task *sysin.AccountTask, localPath string) (*sysin.AccountMediaDownloadResult, error)
}

type IAccountHistory interface {
	FetchPage(ctx context.Context, client *telegram.Client, request *sysin.AccountHistoryPageRequest) ([]*tg.Message, error)
	RetryDelay(err error) (time.Duration, bool)
}

type AccountRuntimeSession interface {
	UpdateAccountRuntime(binding *sysin.AccountRuntimeBinding) bool
	NewAccountRuntimeClient(ctx context.Context, dispatcher tg.UpdateDispatcher) (*telegram.Client, error)
	StartAccountRuntime(ctx context.Context, client *telegram.Client)
	StopAccountRuntime()
	HandleAccountRuntimeError(ctx context.Context, err error)
}

type AccountRuntimeMessageObserver interface {
	HandleAccountRuntimeMessage(ctx context.Context, entities tg.Entities, message *tg.Message, chatIDs []string)
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
var localAccountMediaProvider AccountMediaProvider
var localAccountHistory IAccountHistory

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

func AccountMedia() AccountMediaProvider              { return localAccountMediaProvider }
func RegisterAccountMedia(value AccountMediaProvider) { localAccountMediaProvider = value }

func AccountHistory() IAccountHistory              { return localAccountHistory }
func RegisterAccountHistory(value IAccountHistory) { localAccountHistory = value }

func AccountRuntime() IAccountRuntime                        { return localAccountRuntime }
func RegisterAccountRuntime(value IAccountRuntime)           { localAccountRuntime = value }
func AccountRuntimeProviderInstance() AccountRuntimeProvider { return localAccountRuntimeProvider }
func RegisterAccountRuntimeProvider(value AccountRuntimeProvider) {
	localAccountRuntimeProvider = value
}
