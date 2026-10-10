package sys

import (
	"context"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model"
)

const (
	telegramBotGetFileGlobalConcurrency = 8
	telegramBotGetFileBotConcurrency    = 2
	telegramBotGetFileCircuitFailures   = 3
	telegramBotGetFileCircuitCooldown   = 5 * time.Minute
)

var telegramBotGetFileControl = struct {
	sync.Mutex
	global  chan struct{}
	perBot  map[int64]chan struct{}
	circuit map[int64]telegramBotGetFileCircuitState
}{
	global:  make(chan struct{}, telegramBotGetFileGlobalConcurrency),
	perBot:  make(map[int64]chan struct{}),
	circuit: make(map[int64]telegramBotGetFileCircuitState),
}

type telegramBotGetFileCircuitState struct {
	failures  int
	openUntil time.Time
}

func acquireTelegramBotGetFileSlot(ctx context.Context, botID int64) (func(), error) {
	telegramBotGetFileControl.Lock()
	botSlots := telegramBotGetFileControl.perBot[botID]
	if botSlots == nil {
		botSlots = make(chan struct{}, telegramBotGetFileBotConcurrency)
		telegramBotGetFileControl.perBot[botID] = botSlots
	}
	telegramBotGetFileControl.Unlock()
	select {
	case telegramBotGetFileControl.global <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case botSlots <- struct{}{}:
		return func() {
			<-botSlots
			<-telegramBotGetFileControl.global
		}, nil
	case <-ctx.Done():
		<-telegramBotGetFileControl.global
		return nil, ctx.Err()
	}
}

func telegramBotGetFileCircuitOpen(botID int64, now time.Time) bool {
	telegramBotGetFileControl.Lock()
	defer telegramBotGetFileControl.Unlock()
	return now.Before(telegramBotGetFileControl.circuit[botID].openUntil)
}

func recordTelegramBotGetFileResult(botID int64, err error, now time.Time) {
	telegramBotGetFileControl.Lock()
	defer telegramBotGetFileControl.Unlock()
	state := telegramBotGetFileControl.circuit[botID]
	if err == nil {
		delete(telegramBotGetFileControl.circuit, botID)
		return
	}
	if !isRetryableTelegramBotRequestError(err) {
		return
	}
	state.failures++
	if state.failures >= telegramBotGetFileCircuitFailures {
		state.openUntil = now.Add(telegramBotGetFileCircuitCooldown)
	}
	telegramBotGetFileControl.circuit[botID] = state
}

func telegramOfficialMediaBot(botToken string, conf *model.TelegramConfig) (*tgbot.Bot, error) {
	client, err := telegramHTTPClient(conf.ProxyUrl)
	if err != nil {
		return nil, err
	}
	return tgbot.New(botToken,
		tgbot.WithServerURL("https://api.telegram.org"),
		tgbot.WithHTTPClient(30*time.Second, client),
		tgbot.WithSkipGetMe(),
	)
}

func telegramBotGetFileWithFallback(ctx context.Context, botID int64, botToken string, localBot *tgbot.Bot, conf *model.TelegramConfig, fileID string) (*tgbot.Bot, *models.File, string, int, error) {
	release, err := acquireTelegramBotGetFileSlot(ctx, botID)
	if err != nil {
		return nil, nil, "", 0, err
	}
	defer release()

	localEndpoint := telegramBotAPIEndpoint(conf.BotApiServerUrl)
	if localEndpoint != "local" || !telegramBotGetFileCircuitOpen(botID, time.Now()) {
		file, attempts, localErr := telegramBotGetFileWithBoundedRetry(ctx, func(requestCtx context.Context) (*models.File, error) {
			return localBot.GetFile(requestCtx, &tgbot.GetFileParams{FileID: fileID})
		})
		recordTelegramBotGetFileResult(botID, localErr, time.Now())
		if localErr == nil {
			return localBot, file, localEndpoint, attempts, nil
		}
		if localEndpoint != "local" || !isRetryableTelegramBotRequestError(localErr) {
			return nil, nil, localEndpoint, attempts, localErr
		}
		observeTelegramBotGetFileFallback(ctx, "local_failed", botID)
	} else {
		observeTelegramBotGetFileFallback(ctx, "circuit_open", botID)
	}

	officialBot, err := telegramOfficialMediaBot(botToken, conf)
	if err != nil {
		return nil, nil, "official", 0, gerror.Wrap(err, "创建官方Bot API媒体客户端失败")
	}
	file, attempts, err := telegramBotGetFileWithBoundedRetry(ctx, func(requestCtx context.Context) (*models.File, error) {
		return officialBot.GetFile(requestCtx, &tgbot.GetFileParams{FileID: fileID})
	})
	if err == nil {
		observeTelegramBotGetFileFallback(ctx, "official_success", botID)
	} else {
		observeTelegramBotGetFileFallback(ctx, "official_failed", botID)
	}
	return officialBot, file, "official", attempts, err
}
