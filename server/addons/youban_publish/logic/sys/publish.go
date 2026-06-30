package sys

import (
	"context"

	tgbot "github.com/go-telegram/bot"

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
}

func NewSysPublish() *sSysPublish {
	return &sSysPublish{
		tgLogins: make(map[string]*telegramLoginRuntime),
	}
}

func init() {
	service.RegisterSysPublish(NewSysPublish())
}
