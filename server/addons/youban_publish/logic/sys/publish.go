package sys

import (
	"context"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/service"
)

type sSysPublish struct {
	runtimeCancel context.CancelFunc
	runtimeDone   chan struct{}
	runtimeMu     publishRuntimeMutex
	telegramBotMu publishRuntimeMutex
	telegramBots  map[string]*tgbot.Bot
	tgLoginMu     publishRuntimeMutex
	tgLogins      map[string]*telegramLoginRuntime
	tgQueueMu     publishRuntimeMutex
	tgQueueClient *asynq.Client
	tgQueueServer *asynq.Server

	collectGroupMu     publishRuntimeMutex
	collectGroupTimers map[int64]*time.Timer
}

func NewSysPublish() *sSysPublish {
	return &sSysPublish{
		tgLogins:           make(map[string]*telegramLoginRuntime),
		collectGroupTimers: make(map[int64]*time.Timer),
	}
}

func init() {
	service.RegisterSysPublish(NewSysPublish())
}
