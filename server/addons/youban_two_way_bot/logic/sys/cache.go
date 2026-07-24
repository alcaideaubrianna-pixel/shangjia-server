package sys

import (
	"context"
	"fmt"
	"time"

	"hotgo/internal/library/cache"
)

const twoWayTopicCacheTTL = 24 * time.Hour
const twoWayAdminCacheTTL = 5 * time.Minute
const twoWayStateCacheTTL = 30 * 24 * time.Hour
const twoWayMediaGroupCacheTTL = 30 * time.Second

func topicUserCacheKey(botId int64, userId string) string {
	return fmt.Sprintf("youban_two_way_bot:topic:user:%d:%s", botId, userId)
}

func topicThreadCacheKey(botId int64, threadId int64) string {
	return fmt.Sprintf("youban_two_way_bot:topic:thread:%d:%d", botId, threadId)
}

func adminUserCacheKey(botId int64, userId int64) string {
	return fmt.Sprintf("youban_two_way_bot:admin:%d:%d", botId, userId)
}

func bannedUserCacheKey(botId int64, userId string) string {
	return fmt.Sprintf("youban_two_way_bot:banned:%d:%s", botId, userId)
}

func trustedUserCacheKey(botId int64, userId string) string {
	return fmt.Sprintf("youban_two_way_bot:trusted:%d:%s", botId, userId)
}

func mediaGroupFlushCacheKey(botId int64, direction string, groupId string) string {
	return fmt.Sprintf("youban_two_way_bot:media_group_flush:%d:%s:%s", botId, direction, groupId)
}

func mediaGroupMessageCacheKey(botId int64, direction string, groupId string, sourceChatId string, sourceMessageId int) string {
	return fmt.Sprintf("youban_two_way_bot:media_group_message:%d:%s:%s:%s:%d", botId, direction, groupId, sourceChatId, sourceMessageId)
}

func cacheUserTopic(ctx context.Context, botId int64, userId string, threadId int64) {
	if botId <= 0 || userId == "" || threadId <= 0 {
		return
	}
	_ = cache.Instance().Set(ctx, topicUserCacheKey(botId, userId), threadId, twoWayTopicCacheTTL)
	_ = cache.Instance().Set(ctx, topicThreadCacheKey(botId, threadId), userId, twoWayTopicCacheTTL)
}

func cachedUserThread(ctx context.Context, botId int64, userId string) int64 {
	if botId <= 0 || userId == "" {
		return 0
	}
	value, err := cache.Instance().Get(ctx, topicUserCacheKey(botId, userId))
	if err != nil || value == nil {
		return 0
	}
	return value.Int64()
}

func cachedThreadUser(ctx context.Context, botId int64, threadId int64) string {
	if botId <= 0 || threadId <= 0 {
		return ""
	}
	value, err := cache.Instance().Get(ctx, topicThreadCacheKey(botId, threadId))
	if err != nil || value == nil {
		return ""
	}
	return value.String()
}

func removeCachedUserTopic(ctx context.Context, botId int64, userId string, threadId int64) {
	keys := make([]interface{}, 0, 2)
	if userId != "" {
		keys = append(keys, topicUserCacheKey(botId, userId))
	}
	if threadId > 0 {
		keys = append(keys, topicThreadCacheKey(botId, threadId))
	}
	if len(keys) > 0 {
		_, _ = cache.Instance().Remove(ctx, keys...)
	}
}
