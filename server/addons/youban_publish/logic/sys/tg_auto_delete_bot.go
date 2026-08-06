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
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) deleteMatchedTelegramMessageWithChannelBots(ctx context.Context, incomingBotId int64, channel *autoDeleteChannel, chatId string, messageId int) (*sysin.BotModel, error) {
	if channel == nil {
		return nil, gerror.New("消息自动删除频道不能为空")
	}
	botIds := channelAutoDeleteBotIds(channel.BotIdJson, incomingBotId)
	if len(botIds) == 0 {
		return nil, gerror.New("频道未配置上架 Bot，无法自动删除消息")
	}
	chatId = normalizeTelegramChannelChatID(chatId)
	var failures []string
	for _, botId := range botIds {
		botItem, err := s.autoDeleteBotById(ctx, botId, channel.TenantId)
		if err != nil {
			failures = append(failures, fmt.Sprintf("bot:%d %v", botId, err))
			continue
		}
		if err = s.ensureTelegramBotCanDeleteMessages(ctx, channel, botItem, chatId); err != nil {
			failures = append(failures, fmt.Sprintf("bot:%d %v", botId, err))
			autoDeleteWarn(ctx, fmt.Sprintf("permission:%d:%s", botId, chatId), "频道自动删除 Bot 无删除权限 channel:%d bot:%d chat:%s err:%+v", channel.Id, botId, chatId, err)
			continue
		}
		err = s.deleteMatchedTelegramMessageByChat(ctx, botItem.BotToken, chatId, messageId)
		if err == nil || isTelegramMessageAlreadyDeletedError(err) {
			return botItem, err
		}
		failures = append(failures, fmt.Sprintf("bot:%d %v", botId, err))
		autoDeleteWarn(ctx, fmt.Sprintf("delete:%d:%s", botId, chatId), "频道自动删除 Bot 删除消息失败，尝试下一个 Bot channel:%d bot:%d chat:%s message:%d err:%+v", channel.Id, botId, chatId, messageId, err)
	}
	if len(failures) == 0 {
		return nil, gerror.New("频道没有可用的上架 Bot")
	}
	return nil, gerror.Newf("频道自动删除失败，已尝试频道配置中的全部 Bot：%s", strings.Join(failures, "；"))
}

func channelAutoDeleteBotIds(raw string, incomingBotId int64) []int64 {
	configured := decodeBotIds(raw)
	if incomingBotId <= 0 || !autoDeleteBotAllowed(incomingBotId, configured) {
		return configured
	}
	result := make([]int64, 0, len(configured))
	result = append(result, incomingBotId)
	for _, id := range configured {
		if id != incomingBotId {
			result = append(result, id)
		}
	}
	return result
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

func (s *sSysPublish) ensureTelegramBotCanDeleteMessages(ctx context.Context, channel *autoDeleteChannel, botItem *sysin.BotModel, chatId string) error {
	if channel == nil || botItem == nil || botItem.Id <= 0 {
		return gerror.New("频道 Bot 权限状态不存在，请先检测频道")
	}
	state := channelBotPermissionStateForBot(channel.BotPermissionStatusJson, botItem.Id)
	if state == nil {
		canSend, canDelete, inChannel, message := s.checkBotChannelMember(ctx, botItem, chatId)
		result := &sysin.ChannelCheckBotModel{
			BotId:             botItem.Id,
			BotName:           botItem.BotName,
			BotUsername:       strings.TrimPrefix(botItem.BotUsername, "@"),
			CanSendMessage:    boolToInt(canSend),
			CanDeleteMessages: boolToInt(canDelete),
			InChannel:         boolToInt(inChannel),
			Status:            "success",
			Message:           message,
		}
		if !canSend || !canDelete {
			result.Status = "warning"
		}
		channel.BotPermissionStatusJson = mergeChannelBotPermissionState(channel.BotPermissionStatusJson, result)
		if err := s.persistChannelBotPermissionState(ctx, channel.TenantId, channel.Id, []*sysin.ChannelCheckBotModel{result}); err != nil {
			autoDeleteWarn(ctx, fmt.Sprintf("permission_cache:%d:%s", botItem.Id, chatId), "回填频道 Bot 权限检测结果失败 channel:%d bot:%d chat:%s err:%+v", channel.Id, botItem.Id, chatId, err)
		}
		if !canSend || !canDelete {
			return gerror.New(message)
		}
		return nil
	}
	return ensureStoredTelegramBotPermissionState(state)
}

func ensureStoredTelegramBotPermissionState(state *channelBotPermissionState) error {
	if state == nil {
		return gerror.New("频道 Bot 权限状态不存在，请先检测频道")
	}
	if state.CanSendMessages != 1 || state.CanDeleteMessages != 1 {
		if strings.TrimSpace(state.Message) != "" {
			return gerror.New(state.Message)
		}
		return gerror.New("频道 Bot 没有发送或删除消息权限，请先点击频道检测")
	}
	return nil
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
	Id                      int64  `json:"id"`
	TenantId                int64  `json:"tenantId"`
	BotIdJson               string `json:"botIdJson"`
	BotPermissionStatusJson string `json:"botPermissionStatusJson"`
	ChannelTitle            string `json:"channelTitle"`
	TargetChatId            string `json:"targetChatId"`
	AutoDeleteEnabled       int    `json:"autoDeleteEnabled"`
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
