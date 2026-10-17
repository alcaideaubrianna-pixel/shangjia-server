package sys

import (
	"context"
	"strings"
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

func telegramBotGetFileWithFallback(ctx context.Context, botID int64, botToken string, localBot *tgbot.Bot, conf *model.TelegramConfig, fileID string, sourceSize int64) (*tgbot.Bot, *models.File, string, int, error) {
	release, err := acquireTelegramBotGetFileSlot(ctx, botID)
	if err != nil {
		return nil, nil, "", 0, err
	}
	defer release()

	localEndpoint := telegramBotAPIEndpoint(conf.BotApiServerUrl)
	if localEndpoint != "local" {
		officialBot, createErr := telegramOfficialMediaBot(botToken, conf)
		if createErr != nil {
			return nil, nil, "official", 0, gerror.Wrap(createErr, "创建官方Bot API媒体客户端失败")
		}
		file, attempts, getErr := telegramBotGetFileWithBoundedRetry(ctx, func(requestCtx context.Context) (*models.File, error) {
			return officialBot.GetFile(requestCtx, &tgbot.GetFileParams{FileID: fileID})
		})
		return officialBot, file, "official", attempts, getErr
	}
	localAttempts := 0
	localFailure := "自建Bot API暂处于熔断等待"
	circuitOpen := telegramBotGetFileCircuitOpen(botID, time.Now())
	if !circuitOpen {
		file, attempts, localErr := telegramBotGetFileWithRetryPolicy(ctx, 1, 2*time.Minute, func(requestCtx context.Context) (*models.File, error) {
			return localBot.GetFile(requestCtx, &tgbot.GetFileParams{FileID: fileID})
		})
		localAttempts = attempts
		recordTelegramBotGetFileResult(botID, localErr, time.Now())
		if localErr == nil {
			return localBot, file, localEndpoint, attempts, nil
		}
		localFailure = strings.ReplaceAll(localErr.Error(), botToken, "[REDACTED]")
		if !isRetryableTelegramBotRequestError(localErr) {
			return nil, nil, localEndpoint, attempts, gerror.New(localFailure)
		}
		observeTelegramBotGetFileFallback(ctx, "local_failed", botID)
	} else {
		observeTelegramBotGetFileFallback(ctx, "circuit_open", botID)
	}

	if !telegramBotOfficialFallbackAllowed(sourceSize) {
		if circuitOpen {
			return nil, nil, localEndpoint, 0, newCollectMediaFairnessRetryError(localFailure, telegramBotGetFileCircuitCooldown)
		}
		return nil, nil, localEndpoint, localAttempts, newCollectMediaRetryError(localFailure+"；大文件或未知大小媒体等待自建接口重试", 30*time.Second)
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
	if err != nil {
		message := localFailure + "；官方回退失败: " + strings.ReplaceAll(err.Error(), botToken, "[REDACTED]")
		return nil, nil, "official", localAttempts + attempts, newCollectMediaRetryError(message, 30*time.Second)
	}
	return officialBot, file, "official", localAttempts + attempts, nil
}

// 未知大小不冒险回退，官方 getFile 的下载限制为 20 MB。
func telegramBotOfficialFallbackAllowed(sourceSize int64) bool {
	return sourceSize > 0 && sourceSize <= 20*1024*1024
}
