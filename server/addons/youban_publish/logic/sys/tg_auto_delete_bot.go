package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) autoDeleteBot(ctx context.Context, botId int64, channel *autoDeleteChannel, conf *model.AutoDeleteConfig) (*sysin.BotModel, error) {
	if botId > 0 {
		if !autoDeleteBotAllowed(botId, conf.BotIds) {
			return nil, nil
		}
		return s.autoDeleteBotById(ctx, botId, channel.TenantId)
	}
	for _, id := range decodeBotIds(channel.BotIdJson) {
		if autoDeleteBotAllowed(id, conf.BotIds) {
			return s.autoDeleteBotById(ctx, id, channel.TenantId)
		}
	}
	for _, id := range conf.BotIds {
		return s.autoDeleteBotById(ctx, id, channel.TenantId)
	}
	return nil, nil
}

func (s *sSysPublish) autoDeleteBotById(ctx context.Context, id int64, tenantId int64) (*sysin.BotModel, error) {
	key := fmt.Sprintf("%d:%d", tenantId, id)
	now := time.Now()
	autoDeleteRuntimeCache.RLock()
	item, ok := autoDeleteRuntimeCache.bot[key]
	autoDeleteRuntimeCache.RUnlock()
	if ok && now.Before(item.ExpireAt) && item.Bot != nil && item.Bot.Id > 0 {
		return item.Bot, nil
	}
	columns := pdao.YoubanPublishBot.Columns()
	var bot *sysin.BotModel
	err := pdao.YoubanPublishBot.Ctx(ctx).
		Where(columns.Id, id).
		Where(columns.TenantId, tenantId).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		Scan(&bot)
	if err != nil {
		return nil, gerror.Wrap(err, "读取消息自动删除 Bot 失败")
	}
	if bot == nil || bot.Id <= 0 {
		return nil, gerror.New("消息自动删除 Bot 不存在、已停用或无权使用")
	}
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.bot[key] = autoDeleteBotLocalCacheItem{Bot: bot, ExpireAt: now.Add(autoDeleteBotLocalTTL)}
	autoDeleteRuntimeCache.Unlock()
	return bot, nil
}

func clearAutoDeleteBotLocalCache() {
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.bot = make(map[string]autoDeleteBotLocalCacheItem)
	autoDeleteRuntimeCache.Unlock()
}

func autoDeleteBotAllowed(botId int64, allowed []int64) bool {
	if botId <= 0 || len(allowed) == 0 {
		return false
	}
	for _, id := range allowed {
		if id == botId {
			return true
		}
	}
	return false
}

func (s *sSysPublish) deleteMatchedTelegramMessage(ctx context.Context, botToken string, chatId int64, messageId int) error {
	return s.deleteMatchedTelegramMessageByChat(ctx, botToken, normalizeTelegramChannelChatID(strconv.FormatInt(chatId, 10)), messageId)
}

func (s *sSysPublish) deleteMatchedTelegramMessageByChat(ctx context.Context, botToken string, chatId string, messageId int) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	chatId = normalizeTelegramChannelChatID(chatId)
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		_, lastErr = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: messageId})
		if lastErr == nil || isTelegramMessageAlreadyDeletedError(lastErr) || !isTelegramAutoDeleteRetryableError(lastErr) {
			return lastErr
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func isTelegramAutoDeleteRetryableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, part := range []string{"goaway", "connection reset", "connection refused", "connection closed", "server closed idle connection", "unexpected eof", "eof", "timeout", "temporarily unavailable", "too many idle connections"} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) appendAutoDeleteLog(ctx context.Context, channel *autoDeleteChannel, botId int64, msg *models.Message, keyword string, status string, message string) {
	s.appendAutoDeleteLogByValues(ctx, channel, botId, msg.ID, keyword, status, message)
}

func (s *sSysPublish) appendAutoDeleteLogByValues(ctx context.Context, channel *autoDeleteChannel, botId int64, messageId int, keyword string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id": 0, "tenant_id": channel.TenantId, "account_id": 0, "profile_id": 0,
		"bot_id": botId, "action": "auto_delete", "status": status,
		"message":    fmt.Sprintf("%s；频道:%s(%s)；消息:%d；关键词:%s", message, channel.ChannelTitle, channel.TargetChatId, messageId, keyword),
		"created_at": gtime.Now(),
	}).Insert()
}

type autoDeleteChannel struct {
	Id           int64  `json:"id"`
	TenantId     int64  `json:"tenantId"`
	BotIdJson    string `json:"botIdJson"`
	ChannelTitle string `json:"channelTitle"`
	TargetChatId string `json:"targetChatId"`
}

type autoDeleteChannelCacheItem struct {
	Found   bool               `json:"found"`
	Channel *autoDeleteChannel `json:"channel"`
}

type autoDeleteChannelLocalCacheItem struct {
	Value    autoDeleteChannelCacheItem
	ExpireAt time.Time
}

type autoDeleteBotLocalCacheItem struct {
	Bot      *sysin.BotModel
	ExpireAt time.Time
}

func autoDeleteWarn(ctx context.Context, key string, format string, args ...interface{}) {
	now := time.Now()
	autoDeleteRuntimeCache.Lock()
	lastAt, ok := autoDeleteRuntimeCache.warned[key]
	if ok && now.Sub(lastAt) < autoDeleteWarnInterval {
		autoDeleteRuntimeCache.Unlock()
		return
	}
	autoDeleteRuntimeCache.warned[key] = now
	for itemKey, itemAt := range autoDeleteRuntimeCache.warned {
		if now.Sub(itemAt) > autoDeleteWarnInterval*10 {
			delete(autoDeleteRuntimeCache.warned, itemKey)
		}
	}
	autoDeleteRuntimeCache.Unlock()
	g.Log().Warningf(ctx, format, args...)
}
