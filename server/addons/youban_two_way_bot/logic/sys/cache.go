package sys

import (
	"context"
	"fmt"
	"time"

	"hotgo/internal/library/cache"
)

const twoWayTopicCacheTTL = 24 * time.Hour

func topicUserCacheKey(botId int64, userId string) string {
	return fmt.Sprintf("youban_two_way_bot:topic:user:%d:%s", botId, userId)
}

func topicThreadCacheKey(botId int64, threadId int64) string {
	return fmt.Sprintf("youban_two_way_bot:topic:thread:%d:%d", botId, threadId)
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
