package sys

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
)

const (
	autoDeleteChannelCacheVersionKey = "youban_publish:auto_delete:channel:version"
	autoDeleteChannelCacheKeyPrefix  = "youban_publish:auto_delete:channel"
	autoDeleteChannelPositiveTTL     = 10 * time.Minute
	autoDeleteChannelNegativeTTL     = 30 * time.Minute
	autoDeleteChannelLocalTTL        = 2 * time.Minute
	autoDeleteChannelVersionLocalTTL = 10 * time.Second
	autoDeleteBotLocalTTL            = 10 * time.Minute
	autoDeleteWarnInterval           = time.Minute
)

var autoDeleteRuntimeCache = struct {
	sync.RWMutex
	channel         map[string]autoDeleteChannelLocalCacheItem
	bot             map[string]autoDeleteBotLocalCacheItem
	version         string
	versionExpireAt time.Time
	warned          map[string]time.Time
}{
	channel: make(map[string]autoDeleteChannelLocalCacheItem),
	bot:     make(map[string]autoDeleteBotLocalCacheItem),
	warned:  make(map[string]time.Time),
}

func telegramAllowedUpdateNames() []string {
	return []string{
		models.AllowedUpdateMessage,
		models.AllowedUpdateEditedMessage,
		models.AllowedUpdateChannelPost,
		models.AllowedUpdateEditedChannelPost,
	}
}

func telegramAllowedUpdates() tgbot.AllowedUpdates {
	return tgbot.AllowedUpdates(telegramAllowedUpdateNames())
}

func telegramUpdateMessage(update *models.Update) (*models.Message, string) {
	if update == nil {
		return nil, ""
	}
	switch {
	case update.ChannelPost != nil:
		return update.ChannelPost, models.AllowedUpdateChannelPost
	case update.EditedChannelPost != nil:
		return update.EditedChannelPost, models.AllowedUpdateEditedChannelPost
	case update.Message != nil:
		return update.Message, models.AllowedUpdateMessage
	case update.EditedMessage != nil:
		return update.EditedMessage, models.AllowedUpdateEditedMessage
	default:
		return nil, ""
	}
}

func telegramMessageText(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	return text
}

func (s *sSysPublish) handleTelegramAutoDelete(ctx context.Context, botId int64, msg *models.Message, text string) {
	if msg == nil || msg.ID <= 0 || strings.TrimSpace(text) == "" {
		return
	}
	conf, err := service.SysConfig().AutoDeleteConfigView(ctx, &sysin.AutoDeleteConfigViewInp{})
	if err != nil || conf == nil || conf.AutoDeleteConfig == nil {
		g.Log().Warningf(ctx, "读取频道自动删除配置失败：%+v", err)
		return
	}
	if conf.Enabled != 1 {
		return
	}
	keyword := matchedAutoDeleteKeyword(text, conf.Keywords)
	if keyword == "" {
		return
	}
	channel, err := s.autoDeleteChannel(ctx, msg.Chat)
	if err != nil || channel == nil || channel.Id <= 0 {
		if err != nil {
			autoDeleteWarn(ctx, "bot_channel_lookup", "频道自动删除查询频道失败 chat:%d err:%+v", msg.Chat.ID, err)
		}
		return
	}
	botItem, err := s.autoDeleteBot(ctx, botId, channel, conf.AutoDeleteConfig)
	if err != nil || botItem == nil || botItem.Id <= 0 {
		if err != nil {
			autoDeleteWarn(ctx, "bot_lookup", "频道自动删除查询Bot失败 channel:%d bot:%d err:%+v", channel.Id, botId, err)
		}
		return
	}
	if err = s.deleteMatchedTelegramMessage(ctx, botItem.BotToken, msg.Chat.ID, msg.ID); err != nil {
		if isTelegramMessageAlreadyDeletedError(err) {
			s.appendAutoDeleteLog(ctx, channel, botItem.Id, msg, keyword, "skipped", "频道消息命中关键词，但TG消息已不存在")
			return
		}
		s.appendAutoDeleteLog(ctx, channel, botItem.Id, msg, keyword, "failed", err.Error())
		g.Log().Warningf(ctx, "频道自动删除失败 channel:%d bot:%d message:%d err:%+v", channel.Id, botItem.Id, msg.ID, err)
		return
	}
	s.appendAutoDeleteLog(ctx, channel, botItem.Id, msg, keyword, "success", "频道消息命中关键词，已自动删除")
}

func (s *sSysPublish) handleGotdAutoDelete(ctx context.Context, msg *tg.Message) {
	if msg == nil || msg.ID <= 0 || strings.TrimSpace(msg.Message) == "" {
		return
	}
	conf, err := service.SysConfig().AutoDeleteConfigView(ctx, &sysin.AutoDeleteConfigViewInp{})
	if err != nil || conf == nil || conf.AutoDeleteConfig == nil {
		g.Log().Warningf(ctx, "读取频道自动删除配置失败：%+v", err)
		return
	}
	if conf.Enabled != 1 {
		return
	}
	keyword := matchedAutoDeleteKeyword(msg.Message, conf.Keywords)
	if keyword == "" {
		return
	}
	chatIds := gotdMessageChatIds(msg)
	channel, err := s.autoDeleteChannelByChatIds(ctx, chatIds)
	if err != nil || channel == nil || channel.Id <= 0 {
		if err != nil {
			autoDeleteWarn(ctx, "gotd_channel_lookup", "频道自动删除账号监听查询频道失败 chats:%s err:%+v", strings.Join(chatIds, ","), err)
		}
		return
	}
	botItem, err := s.autoDeleteBot(ctx, 0, channel, conf.AutoDeleteConfig)
	if err != nil || botItem == nil || botItem.Id <= 0 {
		if err != nil {
			autoDeleteWarn(ctx, "gotd_bot_lookup", "频道自动删除账号监听查询Bot失败 channel:%d err:%+v", channel.Id, err)
		}
		return
	}
	chatId := normalizeTelegramChannelChatID(channel.TargetChatId)
	if chatId == "" && len(chatIds) > 0 {
		chatId = normalizeTelegramChannelChatID(chatIds[0])
	}
	if err = s.deleteMatchedTelegramMessageByChat(ctx, botItem.BotToken, chatId, msg.ID); err != nil {
		if isTelegramMessageAlreadyDeletedError(err) {
			s.appendAutoDeleteLogByValues(ctx, channel, botItem.Id, msg.ID, keyword, "skipped", "频道消息命中关键词，TG消息已不存在")
			return
		}
		s.appendAutoDeleteLogByValues(ctx, channel, botItem.Id, msg.ID, keyword, "failed", err.Error())
		g.Log().Warningf(ctx, "频道自动删除失败 channel:%d bot:%d message:%d err:%+v", channel.Id, botItem.Id, msg.ID, err)
		return
	}
	s.appendAutoDeleteLogByValues(ctx, channel, botItem.Id, msg.ID, keyword, "success", "账号监听频道消息命中关键词，已自动删除")
}

func matchedAutoDeleteKeyword(text string, keywords []string) string {
	text = normalizeAutoDeleteKeywordText(text)
	if text == "" {
		return ""
	}
	for _, keyword := range keywords {
		rawKeyword := strings.TrimSpace(keyword)
		if rawKeyword == "" {
			continue
		}
		if strings.Contains(text, normalizeAutoDeleteKeywordText(rawKeyword)) {
			return rawKeyword
		}
	}
	return ""
}

func normalizeAutoDeleteKeywordText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	replacer := strings.NewReplacer(
		"敗", "败",
		"錄", "录",
		"複", "复",
		"週", "周",
		"發", "发",
		"現", "现",
		"資", "资",
		"視", "视",
		"頻", "频",
		"號", "号",
		"檢", "检",
		"測", "测",
	)
	return replacer.Replace(text)
}

func (s *sSysPublish) autoDeleteChannel(ctx context.Context, chat models.Chat) (*autoDeleteChannel, error) {
	chatId := strconv.FormatInt(chat.ID, 10)
	positiveChatId := strings.TrimPrefix(chatId, "-100")
	username := strings.TrimPrefix(strings.TrimSpace(chat.Username), "@")
	cacheKey := s.autoDeleteChannelCacheKey(ctx, "bot", uniqueStrings([]string{chatId, positiveChatId, username}))
	if channel, hit := autoDeleteChannelCacheGet(ctx, cacheKey); hit {
		return channel, nil
	}
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("status", 1).
		Where("publish_direction", "up").
		WhereNull("deleted_at")
	if username != "" {
		mod = mod.Where("(target_chat_id IN(?, ?) OR channel_username = ?)", chatId, positiveChatId, username)
	} else {
		mod = mod.Where("target_chat_id IN(?, ?)", chatId, positiveChatId)
	}
	var channel *autoDeleteChannel
	if err := mod.Fields("id,tenant_id,bot_id_json,channel_title,target_chat_id").OrderDesc("id").Scan(&channel); err != nil {
		return nil, err
	}
	autoDeleteChannelCacheSet(ctx, cacheKey, channel)
	return channel, nil
}

func (s *sSysPublish) autoDeleteChannelByChatIds(ctx context.Context, chatIds []string) (*autoDeleteChannel, error) {
	values := make([]string, 0, len(chatIds)*2)
	for _, chatId := range chatIds {
		chatId = strings.TrimSpace(chatId)
		if chatId == "" {
			continue
		}
		values = append(values, chatId)
		if strings.HasPrefix(chatId, "-100") {
			values = append(values, strings.TrimPrefix(chatId, "-100"))
		}
	}
	values = uniqueStrings(values)
	if len(values) == 0 {
		return nil, nil
	}
	cacheKey := s.autoDeleteChannelCacheKey(ctx, "gotd", values)
	if channel, hit := autoDeleteChannelCacheGet(ctx, cacheKey); hit {
		return channel, nil
	}
	var channel *autoDeleteChannel
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("status", 1).
		Where("publish_direction", "up").
		WhereNull("deleted_at").
		WhereIn("target_chat_id", values).
		Fields("id,tenant_id,bot_id_json,channel_title,target_chat_id").
		OrderDesc("id").
		Scan(&channel); err != nil {
		return nil, err
	}
	autoDeleteChannelCacheSet(ctx, cacheKey, channel)
	return channel, nil
}

func (s *sSysPublish) autoDeleteChannelCacheKey(ctx context.Context, source string, values []string) string {
	values = uniqueStrings(values)
	sort.Strings(values)
	version := s.autoDeleteChannelCacheVersion(ctx)
	return fmt.Sprintf("%s:%s:%s:%s", autoDeleteChannelCacheKeyPrefix, version, source, strings.Join(values, "|"))
}

func (s *sSysPublish) autoDeleteChannelCacheVersion(ctx context.Context) string {
	now := time.Now()
	autoDeleteRuntimeCache.RLock()
	if autoDeleteRuntimeCache.version != "" && now.Before(autoDeleteRuntimeCache.versionExpireAt) {
		version := autoDeleteRuntimeCache.version
		autoDeleteRuntimeCache.RUnlock()
		return version
	}
	autoDeleteRuntimeCache.RUnlock()

	cacheVar, err := cache.Instance().Get(ctx, autoDeleteChannelCacheVersionKey)
	if err == nil && !cacheVar.IsNil() {
		version := strings.TrimSpace(cacheVar.String())
		if version != "" {
			autoDeleteChannelLocalVersionSet(version)
			return version
		}
	}
	autoDeleteChannelLocalVersionSet("1")
	return "1"
}

func (s *sSysPublish) refreshAutoDeleteChannelCache(ctx context.Context) {
	version := strconv.FormatInt(gtime.Now().TimestampNano(), 10)
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.channel = make(map[string]autoDeleteChannelLocalCacheItem)
	autoDeleteRuntimeCache.version = version
	autoDeleteRuntimeCache.versionExpireAt = time.Now().Add(autoDeleteChannelVersionLocalTTL)
	autoDeleteRuntimeCache.Unlock()
	_ = cache.Instance().Set(ctx, autoDeleteChannelCacheVersionKey, version, 24*time.Hour)
}

func autoDeleteChannelLocalVersionSet(version string) {
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.version = version
	autoDeleteRuntimeCache.versionExpireAt = time.Now().Add(autoDeleteChannelVersionLocalTTL)
	autoDeleteRuntimeCache.Unlock()
}

func autoDeleteChannelCacheGet(ctx context.Context, key string) (*autoDeleteChannel, bool) {
	if channel, hit := autoDeleteChannelLocalCacheGet(key); hit {
		return channel, true
	}
	cacheVar, err := cache.Instance().Get(ctx, key)
	if err != nil || cacheVar.IsNil() {
		return nil, false
	}
	var item autoDeleteChannelCacheItem
	if err = cacheVar.Scan(&item); err != nil {
		return nil, false
	}
	autoDeleteChannelLocalCacheSet(key, &item, autoDeleteChannelLocalTTL)
	if !item.Found {
		return nil, true
	}
	return item.Channel, true
}

func autoDeleteChannelCacheSet(ctx context.Context, key string, channel *autoDeleteChannel) {
	item := autoDeleteChannelCacheItem{
		Found:   channel != nil && channel.Id > 0,
		Channel: channel,
	}
	ttl := autoDeleteChannelNegativeTTL
	if item.Found {
		ttl = autoDeleteChannelPositiveTTL
	}
	autoDeleteChannelLocalCacheSet(key, &item, autoDeleteChannelLocalTTL)
	_ = cache.Instance().Set(ctx, key, item, ttl)
}

func autoDeleteChannelLocalCacheGet(key string) (*autoDeleteChannel, bool) {
	now := time.Now()
	autoDeleteRuntimeCache.RLock()
	item, ok := autoDeleteRuntimeCache.channel[key]
	autoDeleteRuntimeCache.RUnlock()
	if !ok || now.After(item.ExpireAt) {
		if ok {
			autoDeleteRuntimeCache.Lock()
			if latest, exists := autoDeleteRuntimeCache.channel[key]; exists && now.After(latest.ExpireAt) {
				delete(autoDeleteRuntimeCache.channel, key)
			}
			autoDeleteRuntimeCache.Unlock()
		}
		return nil, false
	}
	if !item.Value.Found {
		return nil, true
	}
	return item.Value.Channel, true
}

func autoDeleteChannelLocalCacheSet(key string, item *autoDeleteChannelCacheItem, ttl time.Duration) {
	if key == "" || item == nil || ttl <= 0 {
		return
	}
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.channel[key] = autoDeleteChannelLocalCacheItem{
		Value:    *item,
		ExpireAt: time.Now().Add(ttl),
	}
	autoDeleteRuntimeCache.Unlock()
}

func (s *sSysPublish) autoDeleteBot(ctx context.Context, botId int64, channel *autoDeleteChannel, conf *model.AutoDeleteConfig) (*sysin.BotModel, error) {
	if botId > 0 {
		if !autoDeleteBotAllowed(botId, conf.BotIds) {
			return nil, nil
		}
		return s.autoDeleteBotById(ctx, botId, channel.TenantId)
	}
	for _, id := range decodeBotIds(channel.BotIdJson) {
		if !autoDeleteBotAllowed(id, conf.BotIds) {
			continue
		}
		return s.autoDeleteBotById(ctx, id, channel.TenantId)
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
	if ok && now.After(item.ExpireAt) {
		autoDeleteRuntimeCache.Lock()
		if latest, exists := autoDeleteRuntimeCache.bot[key]; exists && now.After(latest.ExpireAt) {
			delete(autoDeleteRuntimeCache.bot, key)
		}
		autoDeleteRuntimeCache.Unlock()
	}
	bot, err := s.getBotById(ctx, id, tenantId)
	if err != nil {
		return nil, err
	}
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.bot[key] = autoDeleteBotLocalCacheItem{
		Bot:      bot,
		ExpireAt: time.Now().Add(autoDeleteBotLocalTTL),
	}
	autoDeleteRuntimeCache.Unlock()
	return bot, nil
}

func clearAutoDeleteBotLocalCache() {
	autoDeleteRuntimeCache.Lock()
	autoDeleteRuntimeCache.bot = make(map[string]autoDeleteBotLocalCacheItem)
	autoDeleteRuntimeCache.Unlock()
}

func autoDeleteBotAllowed(botId int64, allowed []int64) bool {
	if botId <= 0 {
		return false
	}
	if len(allowed) == 0 {
		return true
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
		_, lastErr = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
			ChatID:    chatId,
			MessageID: messageId,
		})
		if lastErr == nil || isTelegramMessageAlreadyDeletedError(lastErr) || !isTelegramAutoDeleteRetryableError(lastErr) {
			return lastErr
		}
		delay := time.Duration(attempt+1) * 300 * time.Millisecond
		timer := time.NewTimer(delay)
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
	retryableParts := []string{
		"goaway",
		"connection reset",
		"connection refused",
		"connection closed",
		"server closed idle connection",
		"unexpected eof",
		"eof",
		"timeout",
		"temporarily unavailable",
		"too many idle connections",
	}
	for _, part := range retryableParts {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) appendAutoDeleteLog(ctx context.Context, channel *autoDeleteChannel, botId int64, msg *models.Message, keyword string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     0,
		"task_id":    0,
		"tenant_id":  channel.TenantId,
		"account_id": 0,
		"profile_id": 0,
		"bot_id":     botId,
		"action":     "auto_delete",
		"status":     status,
		"message":    fmt.Sprintf("%s；频道:%s(%s)；消息:%d；关键词:%s", message, channel.ChannelTitle, channel.TargetChatId, msg.ID, keyword),
		"created_at": gtime.Now(),
	}).Insert()
}

func (s *sSysPublish) appendAutoDeleteLogByValues(ctx context.Context, channel *autoDeleteChannel, botId int64, messageId int, keyword string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     0,
		"task_id":    0,
		"tenant_id":  channel.TenantId,
		"account_id": 0,
		"profile_id": 0,
		"bot_id":     botId,
		"action":     "auto_delete",
		"status":     status,
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
