package sys

import (
	"context"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/service"
)

type sSysPublish struct {
	runtimeCancel    context.CancelFunc
	runtimeDone      chan struct{}
	runtimeMu        publishRuntimeMutex
	telegramBotMu    publishRuntimeMutex
	telegramBots     map[string]*tgbot.Bot
	tgLoginMu        publishRuntimeMutex
	tgLogins         map[string]*telegramLoginRuntime
	tgQueueMu        publishRuntimeMutex
	tgQueueClient    *asynq.Client
	tgQueueServer    *asynq.Server
	mediaQueueServer *asynq.Server

	telegramChannelMu    publishRuntimeMutex
	telegramChannelLocks map[string]*publishRuntimeMutex

	collectGroupMu        publishRuntimeMutex
	collectGroupTimers    map[int64]*time.Timer
	collectMediaMu        publishRuntimeMutex
	collectMediaLocks     map[string]*publishRuntimeMutex
	collectMediaLastTouch map[string]time.Time

	accountRuntimeMu publishRuntimeMutex
	accountRuntimes  map[int64]*accountCollectWorker
}

func NewSysPublish() *sSysPublish {
	return &sSysPublish{
		tgLogins:              make(map[string]*telegramLoginRuntime),
		telegramChannelLocks:  make(map[string]*publishRuntimeMutex),
		collectGroupTimers:    make(map[int64]*time.Timer),
		collectMediaLocks:     make(map[string]*publishRuntimeMutex),
		collectMediaLastTouch: make(map[string]time.Time),
		accountRuntimes:       make(map[int64]*accountCollectWorker),
	}
}

func init() {
	service.RegisterSysPublish(NewSysPublish())
}
