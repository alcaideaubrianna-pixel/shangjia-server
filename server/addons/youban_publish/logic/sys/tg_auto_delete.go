package sys

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
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

func (s *sSysPublish) handleTelegramAutoDelete(ctx context.Context, botId int64, tenantId int64, msg *models.Message, text string) {
	if msg == nil || msg.ID <= 0 || strings.TrimSpace(text) == "" {
		return
	}
	channel, err := s.autoDeleteChannel(ctx, msg.Chat, tenantId)
	if err != nil || channel == nil || channel.Id <= 0 {
		if err != nil {
			autoDeleteWarn(ctx, "bot_channel_lookup", "频道自动删除查询频道失败 chat:%d err:%+v", msg.Chat.ID, err)
		}
		return
	}
	if channel.AutoDeleteEnabled != 1 {
		return
	}
	conf, err := service.SysConfig().AutoDeleteConfigForTenant(ctx, channel.TenantId)
	if err != nil || conf == nil || conf.AutoDeleteConfig == nil {
		g.Log().Warningf(ctx, "读取租户频道自动删除规则失败 tenant:%d err:%+v", channel.TenantId, err)
		return
	}
	keyword := matchedAutoDeleteKeyword(text, conf.Keywords)
	if keyword == "" {
		keyword = matchedAutoDeleteRule(text, conf.Rules)
	}
	if keyword == "" {
		return
	}
	messageLock := lock.Mutex(fmt.Sprintf("youban_publish:auto_delete:%d:%d", channel.Id, msg.ID))
	if err = messageLock.TryLock(ctx); err != nil {
		return
	}
	defer func() { _ = messageLock.Unlock(context.Background()) }()
	botItem, err := s.deleteMatchedTelegramMessageWithChannelBots(ctx, botId, channel, strconv.FormatInt(msg.Chat.ID, 10), msg.ID)
	if botItem == nil || botItem.Id <= 0 || (err != nil && !isTelegramMessageAlreadyDeletedError(err)) {
		if err != nil {
			autoDeleteWarn(ctx, "bot_lookup", "频道自动删除查询Bot失败 channel:%d bot:%d err:%+v", channel.Id, botId, err)
		}
		return
	}
	if err != nil {
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

func matchedAutoDeleteRule(text string, rules []string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := splitAutoDeleteRuleLines(text)
	joined := strings.Join(lines, "\n")
	for _, rule := range rules {
		mode, pattern := parseAutoDeleteRulePattern(rule)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		switch mode {
		case "single":
			if len(lines) == 1 && re.MatchString(lines[0]) {
				return strings.TrimSpace(rule)
			}
		case "text":
			if re.MatchString(joined) {
				return strings.TrimSpace(rule)
			}
		default:
			if re.MatchString(joined) {
				return strings.TrimSpace(rule)
			}
		}
	}
	return ""
}

func splitAutoDeleteRuleLines(text string) []string {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func parseAutoDeleteRulePattern(rule string) (mode string, pattern string) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return "", ""
	}
	if idx := strings.Index(rule, ":"); idx > 0 {
		prefix := strings.ToLower(strings.TrimSpace(rule[:idx]))
		if prefix == "single" || prefix == "text" {
			mode = prefix
			pattern = strings.TrimSpace(rule[idx+1:])
			return mode, pattern
		}
	}
	return "", rule
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

func (s *sSysPublish) autoDeleteChannel(ctx context.Context, chat models.Chat, tenantId int64) (*autoDeleteChannel, error) {
	chatId := strconv.FormatInt(chat.ID, 10)
	positiveChatId := strings.TrimPrefix(chatId, "-100")
	username := strings.TrimPrefix(strings.TrimSpace(chat.Username), "@")
	cacheKey := s.autoDeleteChannelCacheKey(ctx, fmt.Sprintf("bot:%d", tenantId), uniqueStrings([]string{chatId, positiveChatId, username}))
	if channel, hit := autoDeleteChannelCacheGet(ctx, cacheKey); hit {
		return channel, nil
	}
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("status", 1).
		Where("publish_direction", "up").
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if username != "" {
		mod = mod.Where("(target_chat_id IN(?, ?) OR channel_username = ?)", chatId, positiveChatId, username)
	} else {
		mod = mod.Where("target_chat_id IN(?, ?)", chatId, positiveChatId)
	}
	var channel *autoDeleteChannel
	if err := mod.Fields("id,tenant_id,bot_id_json,bot_permission_status_json,channel_title,target_chat_id,auto_delete_enabled").OrderDesc("id").Scan(&channel); err != nil {
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
