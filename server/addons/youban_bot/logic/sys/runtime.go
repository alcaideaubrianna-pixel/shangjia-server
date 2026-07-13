package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
)

func (s *sSysBot) startPolling(ctx context.Context) {
	s.stopPolling()
	runtimeCtx, cancel := context.WithCancel(ctx)
	s.runtimeMu.Lock()
	s.runtimeCancel = cancel
	s.runtimeMu.Unlock()
	go s.pollingLoop(runtimeCtx)
}

func (s *sSysBot) restartRuntime(ctx context.Context) {
	_ = s.syncAllTelegramBotMenus(ctx)
	s.startPolling(ctx)
}

func (s *sSysBot) stopPolling() {
	s.runtimeMu.Lock()
	cancel := s.runtimeCancel
	s.runtimeCancel = nil
	s.runtimeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *sSysBot) pollingLoop(ctx context.Context) {
	bots, err := s.enabledBots(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取Bot配置失败，跳过本地Polling err:%+v", err)
		return
	}
	for _, item := range bots {
		if item == nil || strings.TrimSpace(item.BotToken) == "" {
			continue
		}
		row := item
		go s.startBotRuntime(ctx, row)
	}
}

func (s *sSysBot) startBotRuntime(ctx context.Context, row *sysin.BotModel) {
	bot, err := s.telegramBotWithHandler(ctx, row)
	if err != nil {
		g.Log().Warningf(ctx, "启动Telegram Bot失败 botId:%d err:%+v", row.Id, err)
		_ = s.markBotOffline(ctx, row.Id, err)
		return
	}
	mode := s.botRuntimeMode(ctx, row)
	webhookUrl := s.botWebhookURL(ctx, row)
	if mode == "webhook" || (mode == "auto" && s.shouldUseWebhookInAuto(ctx) && webhookUrl != "") {
		_, err = bot.SetWebhook(ctx, &tgbot.SetWebhookParams{URL: webhookUrl, AllowedUpdates: botAllowedUpdates()})
		if err != nil {
			g.Log().Warningf(ctx, "设置Telegram Webhook失败 botId:%d url:%s err:%+v", row.Id, webhookUrl, err)
			if shouldMarkBotOffline(err) {
				_ = s.markBotOffline(ctx, row.Id, err)
			}
			return
		}
		g.Log().Infof(ctx, "Telegram Bot Webhook已启用 botId:%d username:%s url:%s", row.Id, row.BotUsername, webhookUrl)
		return
	}
	if !g.Cfg().MustGet(ctx, "youbanBot.telegram.pollingEnabled", true).Bool() {
		g.Log().Infof(ctx, "Telegram Bot Polling未启用 botId:%d username:%s", row.Id, row.BotUsername)
		return
	}
	if _, err = bot.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{DropPendingUpdates: false}); err != nil {
		if isIgnorableTelegramError(err) {
			g.Log().Infof(ctx, "清理Telegram Webhook返回异常，已忽略并继续Polling botId:%d err:%+v", row.Id, err)
		} else {
			g.Log().Warningf(ctx, "清理Telegram Webhook失败，继续启动Polling botId:%d err:%+v", row.Id, err)
		}
	}
	g.Log().Infof(ctx, "Telegram Bot Polling已启动 botId:%d username:%s", row.Id, row.BotUsername)
	bot.Start(ctx)
}

func (s *sSysBot) telegramBotWithHandler(ctx context.Context, row *sysin.BotModel) (*tgbot.Bot, error) {
	if row == nil {
		return nil, gerror.New("Bot配置不能为空")
	}
	client := telegramHTTPClientFromConfig(ctx)
	return tgbot.New(strings.TrimSpace(row.BotToken),
		tgbot.WithHTTPClient(21*time.Second, client),
		tgbot.WithSkipGetMe(),
		tgbot.WithAllowedUpdates(botAllowedUpdates()),
		tgbot.WithDefaultHandler(func(updateCtx context.Context, _ *tgbot.Bot, update *models.Update) {
			if err := s.handleUpdate(updateCtx, row.Id, update); err != nil {
				g.Log().Warningf(updateCtx, "处理Telegram消息失败 botId:%d err:%+v", row.Id, err)
			}
		}),
		tgbot.WithErrorsHandler(func(err error) { logTelegramSDKError(ctx, err) }),
	)
}

func telegramHTTPClientFromConfig(ctx context.Context) tgbot.HttpClient {
	client := &http.Client{Timeout: 35 * time.Second}
	if proxyUrl := g.Cfg().MustGet(ctx, "youbanBot.telegram.proxyUrl").String(); strings.TrimSpace(proxyUrl) != "" {
		proxyClient, err := telegramHTTPClient(proxyUrl)
		if err == nil {
			return proxyClient
		}
		g.Log().Warningf(ctx, "Telegram代理配置无效 err:%+v", err)
	}
	return client
}

func (s *sSysBot) handleUpdate(ctx context.Context, botId int64, update *models.Update) error {
	if update == nil {
		return nil
	}
	msg := botMessageFromUpdate(update)
	if msg == nil {
		return nil
	}
	userId := ""
	if msg.From != nil {
		userId = fmt.Sprintf("%d", msg.From.ID)
		ctx = context.WithValue(ctx, telegramUserIdCtxKey{}, userId)
	}
	g.Log().Infof(ctx, "收到Telegram Update botId:%d chatId:%d userId:%s text:%s", botId, msg.Chat.ID, userId, strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption)))
	if err := s.storeTelegramMessage(ctx, botId, msg); err != nil {
		g.Log().Warningf(ctx, "保存Telegram消息日志失败 botId:%d err:%+v", botId, err)
	}
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	if text == "" {
		return nil
	}
	_, err := s.dispatchBotMessage(ctx, &botMessageEvent{BotId: botId, Msg: msg, Text: text})
	return err
}

func (s *sSysBot) storeTelegramMessage(ctx context.Context, botId int64, msg *models.Message) error {
	if msg == nil {
		return nil
	}
	now := gtime.Now()
	telegramUserId := ""
	telegramUsername := ""
	telegramFirstName := ""
	telegramLastName := ""
	if msg.From != nil {
		telegramUserId = fmt.Sprintf("%d", msg.From.ID)
		telegramUsername = strings.TrimPrefix(strings.TrimSpace(msg.From.Username), "@")
		telegramFirstName = msg.From.FirstName
		telegramLastName = msg.From.LastName
	}
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	messageType := telegramMessageType(msg)
	rawBytes, _ := json.Marshal(msg)
	lastAt := now
	if msg.Date > 0 {
		lastAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	if telegramUserId != "" {
		userData := g.Map{
			"bot_id":              botId,
			"telegram_user_id":    telegramUserId,
			"telegram_username":   telegramUsername,
			"telegram_first_name": telegramFirstName,
			"telegram_last_name":  telegramLastName,
			"chat_id":             chatId,
			"chat_type":           string(msg.Chat.Type),
			"chat_title":          firstNonEmpty(msg.Chat.Title, strings.TrimSpace(msg.Chat.FirstName+" "+msg.Chat.LastName), msg.Chat.Username),
			"last_message_text":   text,
			"last_message_at":     lastAt,
			"status":              1,
			"updated_at":          now,
		}
		var exists struct {
			Id int64 `json:"id"`
		}
		_ = g.DB().Model(userTable).Safe().Ctx(ctx).Fields("id").Where("bot_id", botId).Where("telegram_user_id", telegramUserId).Scan(&exists)
		if exists.Id > 0 {
			if _, err := g.DB().Model(userTable).Safe().Ctx(ctx).Where("id", exists.Id).Data(userData).Increment("message_count", 1); err != nil {
				return gerror.Wrap(err, "更新Telegram用户失败")
			}
		} else {
			userData["message_count"] = 1
			userData["created_at"] = now
			if _, err := g.DB().Model(userTable).Safe().Ctx(ctx).Data(userData).Insert(); err != nil {
				return gerror.Wrap(err, "写入Telegram用户失败")
			}
		}
		g.Log().Infof(ctx, "已记录Telegram用户 botId:%d telegramUserId:%s chatId:%s", botId, telegramUserId, chatId)
	}
	_, err := g.DB().Model(messageTable).Safe().Ctx(ctx).Data(g.Map{
		"bot_id":            botId,
		"telegram_user_id":  telegramUserId,
		"telegram_username": telegramUsername,
		"chat_id":           chatId,
		"chat_type":         string(msg.Chat.Type),
		"message_id":        msg.ID,
		"message_type":      messageType,
		"text":              text,
		"raw_json":          string(rawBytes),
		"created_at":        lastAt,
	}).Insert()
	return err
}

func telegramMessageType(msg *models.Message) string {
	switch {
	case msg.Text != "":
		return "text"
	case msg.Caption != "":
		return "caption"
	case len(msg.Photo) > 0:
		return "photo"
	case msg.Video != nil:
		return "video"
	case msg.Document != nil:
		return "document"
	case msg.Voice != nil:
		return "voice"
	case msg.Sticker != nil:
		return "sticker"
	default:
		return "other"
	}
}
