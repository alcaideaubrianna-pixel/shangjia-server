package sys

import (
	"context"

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
}

func NewSysPublish() *sSysPublish {
	return &sSysPublish{
		tgLogins: make(map[string]*telegramLoginRuntime),
	}
}

func init() {
	service.RegisterSysPublish(NewSysPublish())
}
