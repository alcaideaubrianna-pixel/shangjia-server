package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const telegramChannelDisplayCacheTTL = 10 * time.Minute

type telegramChannelDisplay struct {
	Title    string `json:"title"`
	Username string `json:"username"`
}

func (d telegramChannelDisplay) Empty() bool {
	return strings.TrimSpace(d.Title) == "" && strings.TrimSpace(d.Username) == ""
}

func telegramChannelDisplayCacheKey(tenantId int64, tgAccountId int64, channelId string) string {
	return fmt.Sprintf("youban_publish:tg_channel_display:%d:%d:%s", tenantId, tgAccountId, normalizeTelegramChannelChatID(channelId))
}

func (s *sSysPublish) enrichPublishRecordChannelDisplays(ctx context.Context, tenantId int64, list []*sysin.PublishRecordModel) error {
	type groupKey struct {
		tenantId    int64
		tgAccountId int64
	}
	groups := make(map[groupKey][]string)
	for _, item := range list {
		if item == nil || strings.TrimSpace(item.ChannelTitle) != "" || strings.TrimSpace(item.TargetChatId) == "" || tenantId <= 0 {
			continue
		}
		key := groupKey{tenantId: tenantId, tgAccountId: 0}
		groups[key] = append(groups[key], item.TargetChatId)
	}
	if len(groups) == 0 {
		return nil
	}
	displays := make(map[groupKey]map[string]telegramChannelDisplay, len(groups))
	for key, channelIds := range groups {
		items, err := s.resolveTelegramChannelDisplays(ctx, key.tenantId, key.tgAccountId, channelIds)
		if err != nil {
			return err
		}
		displays[key] = items
	}
	for _, item := range list {
		if item == nil || strings.TrimSpace(item.ChannelTitle) != "" {
			continue
		}
		key := groupKey{tenantId: tenantId, tgAccountId: 0}
		if display, ok := displays[key][normalizeTelegramChannelChatID(item.TargetChatId)]; ok {
			item.ChannelTitle = display.Title
			item.ChannelUsername = display.Username
		}
	}
	return nil
}

func (s *sSysPublish) resolveTelegramChannelDisplays(ctx context.Context, tenantId int64, tgAccountId int64, channelIds []string) (map[string]telegramChannelDisplay, error) {
	res := make(map[string]telegramChannelDisplay)
	pending := make([]string, 0, len(channelIds))
	seen := make(map[string]struct{}, len(channelIds))
	for _, raw := range channelIds {
		channelId := normalizeTelegramChannelChatID(raw)
		if channelId == "" {
			continue
		}
		if _, ok := seen[channelId]; ok {
			continue
		}
		seen[channelId] = struct{}{}
		cacheKey := telegramChannelDisplayCacheKey(tenantId, tgAccountId, channelId)
		cacheVar, err := cache.Instance().Get(ctx, cacheKey)
		if err == nil && !cacheVar.IsNil() {
			var item telegramChannelDisplay
			if scanErr := cacheVar.Scan(&item); scanErr == nil && !item.Empty() {
				res[channelId] = item
				continue
			}
		}
		pending = append(pending, channelId)
	}
	if len(pending) == 0 {
		return res, nil
	}
	rows, err := s.loadTelegramChannelDisplays(ctx, tenantId, tgAccountId, pending)
	if err != nil {
		return nil, err
	}
	for channelId, item := range rows {
		if item.Empty() {
			continue
		}
		res[channelId] = item
		_ = cache.Instance().Set(ctx, telegramChannelDisplayCacheKey(tenantId, tgAccountId, channelId), item, telegramChannelDisplayCacheTTL)
	}
	return res, nil
}

func (s *sSysPublish) loadTelegramChannelDisplays(ctx context.Context, tenantId int64, tgAccountId int64, channelIds []string) (map[string]telegramChannelDisplay, error) {
	lookupIds := make([]string, 0, len(channelIds)*2)
	seen := make(map[string]struct{}, len(channelIds)*2)
	for _, channelId := range channelIds {
		for _, id := range tgChannelCacheLookupIds(channelId) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			lookupIds = append(lookupIds, id)
		}
	}
	if len(lookupIds) == 0 {
		return map[string]telegramChannelDisplay{}, nil
	}
	type row struct {
		ChannelId       string `json:"channelId"`
		ChannelTitle    string `json:"channelTitle"`
		ChannelUsername string `json:"channelUsername"`
	}
	var rows []row
	mod := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Fields("channel_id,channel_title,channel_username").
		Where("tenant_id", tenantId).
		WhereIn("channel_id", lookupIds)
	if tgAccountId > 0 {
		mod = mod.Where("tg_account_id", tgAccountId)
	}
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取TG频道显示信息失败")
	}
	res := make(map[string]telegramChannelDisplay, len(rows))
	for _, item := range rows {
		display := telegramChannelDisplay{Title: strings.TrimSpace(item.ChannelTitle), Username: strings.TrimSpace(item.ChannelUsername)}
		for _, id := range tgChannelCacheLookupIds(item.ChannelId) {
			res[normalizeTelegramChannelChatID(id)] = display
		}
	}
	return res, nil
}
